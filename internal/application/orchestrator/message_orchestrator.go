package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ProcessMessageRequest はメッセージ処理リクエスト
type ProcessMessageRequest struct {
	SessionID   string
	Channel     string
	ChatID      string
	UserMessage string
	Attachments []attachment.Attachment
}

// ProcessMessageResponse はメッセージ処理レスポンス
type ProcessMessageResponse struct {
	Response   string
	Route      routing.Route
	Confidence float64
	JobID      string
}

// Orchestrator は MessageOrchestrator と DistributedOrchestrator の共通インターフェース。
// 各アダプター（LINE / Slack / Telegram / Discord）はこのインターフェースに依存する。
type Orchestrator interface {
	ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error)
}

// SessionRepository はセッション永続化のインターフェース
type SessionRepository interface {
	Save(ctx context.Context, sess *session.Session) error
	Load(ctx context.Context, id string) (*session.Session, error)
	Exists(ctx context.Context, id string) (bool, error)
	Delete(ctx context.Context, id string) error
}

// MioAgent はルーティング・会話を担当
type MioAgent interface {
	DecideAction(ctx context.Context, t task.Task) (routing.Decision, error)
	Chat(ctx context.Context, t task.Task) (string, error)
	HandleChatCommand(ctx context.Context, sessionID string, message string) (agent.ChatCommandResult, error)
}

// ShiroAgent は実行を担当
type ShiroAgent interface {
	Execute(ctx context.Context, t task.Task) (string, error)
}

// CoderAgent はコード生成を担当
type CoderAgent interface {
	Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error)
}

// WildAgent は創作Wildを担当
type WildAgent interface {
	Generate(ctx context.Context, t task.Task) (string, error)
}

// HeavyAgent は深い分析・診断を担当
type HeavyAgent interface {
	Generate(ctx context.Context, t task.Task) (string, error)
}

// CoderAgentWithProposal はProposal生成機能を持つCoderAgent
type CoderAgentWithProposal interface {
	CoderAgent
	GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error)
}

// MessageOrchestrator はメッセージ処理を統括
type MessageOrchestrator struct {
	sessionRepo     SessionRepository
	mio             MioAgent
	shiro           ShiroAgent
	coder1          CoderAgent // Slot 1
	coder2          CoderAgent // Slot 2
	coder3          CoderAgent // Slot 3
	coder4          CoderAgent // Slot 4 (v4.1)
	wild            WildAgent
	heavy           HeavyAgent
	workerExecution service.WorkerExecutionService
	coderStatus     *CoderStatus
	codeExecutor    CodeExecutor // Phase 1リファクタリング: コード実行を委譲
	listener        EventListener
	reporter        ReportStore
	idleNotifier    IdleNotifier
	ttsBridge       TTSBridge
	vtuberBridge    VTuberBridge
	maxRepair       int // 0以下は1とみなす

	sessions             *messageSessionLifecycle
	responses            messageResponseAssembler
	preRoutingCommands   *preRoutingCommandHandler
	routeDecisions       *routeDecisionCoordinator
	idleBusyGuards       *idleBusyGuardFactory
	autonomousExecutions *autonomousExecutionCoordinator
	routeDispatcher      *messageRouteDispatcher
}

// SetMaxRepair は自律実行のリペア上限を設定する（デフォルト: 1）
func (o *MessageOrchestrator) SetMaxRepair(n int) {
	if n > 0 {
		o.maxRepair = n
	}
}

func (o *MessageOrchestrator) maxRepairOrDefault() int {
	if o.maxRepair > 0 {
		return o.maxRepair
	}
	return 1
}

// NewMessageOrchestrator は新しいMessageOrchestratorを作成
func NewMessageOrchestrator(
	sessionRepo SessionRepository,
	mio MioAgent,
	shiro ShiroAgent,
	coder1 CoderAgent,
	coder2 CoderAgent,
	coder3 CoderAgent,
	coder4 CoderAgent,
	workerExecution service.WorkerExecutionService,
) *MessageOrchestrator {
	coderStatus := NewCoderStatus()

	// CodeExecutorを初期化（イベント発火は後でSetEventListenerで設定）
	codeExecutor := NewDefaultCodeExecutor(
		coder1,
		coder2,
		coder3,
		coder4,
		workerExecution,
		coderStatus,
		nil, // eventEmitterは後でSetEventListenerで設定
	)

	orch := &MessageOrchestrator{
		sessionRepo:     sessionRepo,
		mio:             mio,
		shiro:           shiro,
		coder1:          coder1,
		coder2:          coder2,
		coder3:          coder3,
		coder4:          coder4,
		workerExecution: workerExecution,
		coderStatus:     coderStatus,
		codeExecutor:    codeExecutor,
	}
	orch.responses = messageResponseAssembler{}
	orch.sessions = newMessageSessionLifecycle(sessionRepo)
	orch.preRoutingCommands = newPreRoutingCommandHandler(mio, orch.emit, orch.responses)
	orch.routeDecisions = newRouteDecisionCoordinator(mio, orch.emit)
	orch.idleBusyGuards = newIdleBusyGuardFactory(nil)
	orch.routeDispatcher = newMessageRouteDispatcher(
		mio,
		shiro,
		codeExecutor,
		orch.emit,
		orch.withStreamHooks,
		orch.pushTTS,
	)
	orch.autonomousExecutions = newAutonomousExecutionCoordinator(nil, orch.maxRepairOrDefault, orch.emit, orch.routeDispatcher.ExecuteDirect)
	orch.routeDispatcher.SetAutonomousExecutor(orch.autonomousExecutions.Execute)
	return orch
}

// SetEventListener sets an optional listener for monitoring events.
func (o *MessageOrchestrator) SetEventListener(l EventListener) {
	o.listener = l
	// CodeExecutorにもイベント発火関数を設定
	if executor, ok := o.codeExecutor.(*DefaultCodeExecutor); ok {
		executor.SetEventEmitter(o.emit)
	}
}

// SetCoderCapabilities は動的コーダー選択に使う能力情報を注入する（Phase 3）
func (o *MessageOrchestrator) SetCoderCapabilities(caps []capability.CoderCapability) {
	if executor, ok := o.codeExecutor.(*DefaultCodeExecutor); ok {
		executor.WithCapabilities(caps)
	}
}

func (o *MessageOrchestrator) SetWildAgent(wild WildAgent) {
	o.wild = wild
	if o.routeDispatcher != nil {
		o.routeDispatcher.SetWildAgent(wild)
	}
}

func (o *MessageOrchestrator) SetHeavyAgent(heavy HeavyAgent) {
	o.heavy = heavy
	if o.routeDispatcher != nil {
		o.routeDispatcher.SetHeavyAgent(heavy)
	}
}

func (o *MessageOrchestrator) SetReportStore(store ReportStore) {
	o.reporter = store
	if o.autonomousExecutions != nil {
		o.autonomousExecutions.SetReportStore(store)
	}
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *MessageOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
	if o.idleBusyGuards != nil {
		o.idleBusyGuards.SetNotifier(n)
	}
}

// SetTTSBridge sets an optional TTS bridge.
func (o *MessageOrchestrator) SetTTSBridge(b TTSBridge) {
	o.ttsBridge = b
}

// SetVTuberBridge sets an optional VTuber bridge.
func (o *MessageOrchestrator) SetVTuberBridge(b VTuberBridge) {
	o.vtuberBridge = b
}

// ProcessMessage はメッセージを処理
func (o *MessageOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	log.Printf("[MessageOrch] ProcessMessage START: sessionID=%s channel=%s chatID=%s message=%q",
		req.SessionID, req.Channel, req.ChatID, req.UserMessage)

	endChatBusy := o.idleBusyGuards.BeginChat()
	defer endChatBusy()

	sess, err := o.sessions.LoadForRequest(ctx, req)
	if err != nil {
		return ProcessMessageResponse{}, err
	}

	o.emitMessageReceived(req)
	if resp, handled, err := o.preRoutingCommands.Handle(ctx, req); err != nil {
		return ProcessMessageResponse{}, err
	} else if handled {
		return resp, nil
	}

	t, jobID, ttsSessionID := o.buildTaskForRequest(req)
	decision, err := o.routeDecisions.Decide(ctx, t, req, jobID)
	if err != nil {
		return ProcessMessageResponse{}, err
	}

	t = t.WithRoute(decision.Route)
	o.startTTSSessionForRoute(ctx, req, jobID, decision, ttsSessionID)

	endWorkerBusy := o.idleBusyGuards.BeginWorker(decision.Route)
	defer endWorkerBusy()

	// 4. ルートに応じて実行
	response, err := o.routeDispatcher.ExecuteTask(ctx, t, decision.Route, req.SessionID, req.Channel, req.ChatID, ttsSessionID)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("task execution failed: %w", err)
	}
	o.endTTSSession(ctx, ttsSessionID)

	if err := o.sessions.SaveCompletedTask(ctx, sess, t); err != nil {
		return ProcessMessageResponse{}, err
	}

	log.Printf("[MessageOrch] ProcessMessage COMPLETE: jobID=%s route=%s response_len=%d",
		jobID.String(), decision.Route, len(response))

	return o.responses.Build(response, decision, jobID), nil
}
