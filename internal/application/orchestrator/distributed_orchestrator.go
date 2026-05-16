package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainnode "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/node"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

const (
	distributedDefaultTimeout = 120 * time.Second
	distributedCoderTimeout   = 6 * time.Minute
	distributedWorkerTimeout  = 6 * time.Minute
	distributedCoderRetryMax  = 2
)

// DistributedOrchestrator はTransport経由でメッセージを送受信する分散オーケストレータ
type DistributedOrchestrator struct {
	sessionRepo   SessionRepository
	mio           MioAgent
	wild          WildAgent
	heavy         HeavyAgent
	router        *transport.MessageRouter
	memory        *session.CentralMemory
	sshTransports map[string]domaintransport.Transport // SSH経由のリモートAgent
	listener      EventListener
	reporter      ReportStore
	idleNotifier  IdleNotifier
	nodeSelector  *NodeSelector
	nodeCaps      map[string]domainnode.ResourceProfile
	coderConfigs  map[string]interface{} // v4.1: coder1-4 の CoderConfig（SSH送信用）
	ttsBridge     TTSBridge
	vtuberBridge  VTuberBridge
	maxRepair     int           // 0以下は1とみなす
	coderTimeout  time.Duration // 0以下は distributedCoderTimeout とみなす
	coderRetryMax int           // 0以下は distributedCoderRetryMax とみなす
	events        *distributedEventPort
	evidence      *distributedEvidenceReporter
	ttsLifecycle  *distributedTTSLifecycle
	sessions      *distributedSessionLifecycle
	autonomous    *distributedAutonomousCoordinator
	routes        *distributedRouteDispatcher
	transports    *distributedTransportExecutor
	codeExecution *distributedCodeExecutionCoordinator
	coderSelector *distributedCoderSelection
	attribution   *distributedAttributionGuard
}

// SetMaxRepair は自律実行のリペア上限を設定する（デフォルト: 1）
func (o *DistributedOrchestrator) SetMaxRepair(n int) {
	if n > 0 {
		o.maxRepair = n
	}
}

func (o *DistributedOrchestrator) maxRepairOrDefault() int {
	if o.maxRepair > 0 {
		return o.maxRepair
	}
	return 1
}

// SetDistributedTimeouts は分散実行のタイムアウトとリトライ上限を設定する
func (o *DistributedOrchestrator) SetDistributedTimeouts(coderTimeoutSec, retryMax int) {
	if coderTimeoutSec > 0 {
		o.coderTimeout = time.Duration(coderTimeoutSec) * time.Second
	}
	if retryMax >= 0 {
		o.coderRetryMax = retryMax
	}
}

func (o *DistributedOrchestrator) coderTimeoutOrDefault() time.Duration {
	if o.coderTimeout > 0 {
		return o.coderTimeout
	}
	return distributedCoderTimeout
}

func (o *DistributedOrchestrator) coderRetryMaxOrDefault() int {
	if o.coderRetryMax > 0 {
		return o.coderRetryMax
	}
	return distributedCoderRetryMax
}

type ReportStore interface {
	Save(ctx context.Context, report domainexecution.ExecutionReport) error
}

// NewDistributedOrchestrator は新しいDistributedOrchestratorを作成
func NewDistributedOrchestrator(
	sessionRepo SessionRepository,
	mio MioAgent,
	router *transport.MessageRouter,
	memory *session.CentralMemory,
	sshTransports map[string]domaintransport.Transport,
) *DistributedOrchestrator {
	if sshTransports == nil {
		sshTransports = make(map[string]domaintransport.Transport)
	}
	orch := &DistributedOrchestrator{
		sessionRepo:   sessionRepo,
		mio:           mio,
		router:        router,
		memory:        memory,
		sshTransports: sshTransports,
		nodeSelector:  NewNodeSelector(),
		nodeCaps:      make(map[string]domainnode.ResourceProfile),
	}
	orch.events = newDistributedEventPort(nil)
	orch.evidence = newDistributedEvidenceReporter(nil)
	orch.ttsLifecycle = newDistributedTTSLifecycle(nil, nil, orch.emit)
	orch.sessions = newDistributedSessionLifecycle(sessionRepo)
	orch.transports = newDistributedTransportExecutor(router, sshTransports, memory, orch.emitProgress, orch.distributedWaitTimeout)
	orch.coderSelector = newDistributedCoderSelection(router, sshTransports, orch.nodeSelector, orch.nodeCaps)
	orch.attribution = newDistributedAttributionGuard(memory)
	orch.codeExecution = newDistributedCodeExecutionCoordinator(
		memory,
		orch.emit,
		orch.emitNote,
		orch.routeToCoderForMessage,
		func() map[string]interface{} { return orch.coderConfigs },
		orch.coderRetryMaxOrDefault,
		orch.executeToAgentViaMailbox,
		orch.executeToAgent,
	)
	orch.routes = newDistributedRouteDispatcher(
		mio,
		memory,
		orch.emit,
		orch.emitNote,
		orch.withStreamHooks,
		orch.pushTTS,
		orch.executeCodeViaShiro,
		orch.routeToAgent,
		orch.withAttributionGuard,
		orch.executeToAgent,
	)
	orch.autonomous = newDistributedAutonomousCoordinator(nil, orch.maxRepairOrDefault, orch.emit, orch.routes.ExecuteDirect)
	orch.routes.SetAutonomousExecutor(orch.autonomous.Execute)
	return orch
}

// SetNodeCapabilities sets capability map used by RouteCODE coder selection.
func (o *DistributedOrchestrator) SetNodeCapabilities(caps map[string]domainnode.ResourceProfile) {
	if caps == nil {
		o.nodeCaps = make(map[string]domainnode.ResourceProfile)
		if o.coderSelector != nil {
			o.coderSelector.SetNodeCapabilities(o.nodeCaps)
		}
		return
	}
	o.nodeCaps = caps
	if o.coderSelector != nil {
		o.coderSelector.SetNodeCapabilities(caps)
	}
}

// SetCoderConfigs sets CoderConfig map for SSH transport (v4.1)
func (o *DistributedOrchestrator) SetCoderConfigs(configs map[string]interface{}) {
	o.coderConfigs = configs
}

// SetEventListener sets an optional listener for monitoring events.
func (o *DistributedOrchestrator) SetEventListener(l EventListener) {
	o.listener = l
	if o.events != nil {
		o.events.SetListener(l)
	}
}

func (o *DistributedOrchestrator) SetReportStore(store ReportStore) {
	o.reporter = store
	if o.evidence != nil {
		o.evidence.SetReportStore(store)
	}
	if o.autonomous != nil {
		o.autonomous.SetReportStore(store)
	}
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *DistributedOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
}

func (o *DistributedOrchestrator) SetWildAgent(wild WildAgent) {
	o.wild = wild
	if o.routes != nil {
		o.routes.SetWildAgent(wild)
	}
}

func (o *DistributedOrchestrator) SetHeavyAgent(heavy HeavyAgent) {
	o.heavy = heavy
	if o.routes != nil {
		o.routes.SetHeavyAgent(heavy)
	}
}

// SetTTSBridge sets an optional TTS bridge.
func (o *DistributedOrchestrator) SetTTSBridge(b TTSBridge) {
	o.ttsBridge = b
	if o.ttsLifecycle != nil {
		o.ttsLifecycle.SetTTSBridge(b)
	}
}

// SetVTuberBridge sets an optional VTuber bridge.
func (o *DistributedOrchestrator) SetVTuberBridge(b VTuberBridge) {
	o.vtuberBridge = b
	if o.ttsLifecycle != nil {
		o.ttsLifecycle.SetVTuberBridge(b)
	}
}

func (o *DistributedOrchestrator) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	o.events.Emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID)
}

func (o *DistributedOrchestrator) emitNote(from, to, content, route, jobID, sessionID, channel, chatID string) {
	o.events.EmitNote(from, to, content, route, jobID, sessionID, channel, chatID)
}

func (o *DistributedOrchestrator) emitProgress(eventType, from, to, content string, msg domaintransport.Message) {
	o.events.EmitProgress(eventType, from, to, content, msg)
}

// ProcessMessage は既存MessageOrchestratorと同じシグネチャでメッセージを処理
// 分散環境ではTransport経由でAgent間通信を行う
func (o *DistributedOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	log.Printf("[DistributedOrch] ProcessMessage START: sessionID=%s channel=%s chatID=%s message=%q",
		req.SessionID, req.Channel, req.ChatID, req.UserMessage)
	startedAt := time.Now().UTC()

	if o.idleNotifier != nil {
		o.idleNotifier.NotifyActivity()
		o.idleNotifier.SetChatBusy(true)
		defer o.idleNotifier.SetChatBusy(false)
	}

	// 1. セッションをロードまたは作成
	sess, err := o.sessions.LoadForRequest(ctx, req)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}

	o.emit("message.received", "user", "mio", req.UserMessage, "", "", req.SessionID, req.Channel, req.ChatID)

	// 2. タスクを作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID)

	// 3. mio がルーティング決定
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		o.saveExecutionReport(ctx, jobID.String(), req.UserMessage, "", startedAt, time.Now().UTC(), err)
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}
	log.Printf("[DistributedOrch] routing decision: route=%s confidence=%.2f reason=%q",
		decision.Route, decision.Confidence, decision.Reason)

	o.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)
	o.emitNote("mio", "user",
		fmt.Sprintf("%s", routeNoticeText(decision.Route, req.UserMessage)),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)

	t = t.WithRoute(decision.Route)
	ttsSessionID := o.ttsLifecycle.StartSessionForRoute(ctx, req, jobID, decision)

	workerMarkedBusy := false
	if o.idleNotifier != nil && decision.Route != routing.RouteCHAT {
		o.idleNotifier.SetWorkerBusy(true)
		workerMarkedBusy = true
	}
	if workerMarkedBusy {
		defer o.idleNotifier.SetWorkerBusy(false)
	}

	// 4. ルートに応じてTransport経由で実行
	response, err := o.executeDistributed(ctx, t, decision.Route, sess.ID(), ttsSessionID)
	if err != nil {
		if decision.Route == routing.RouteCHAT {
			o.saveExecutionReport(ctx, jobID.String(), req.UserMessage, string(decision.Route), startedAt, time.Now().UTC(), err)
		}
		return ProcessMessageResponse{}, fmt.Errorf("distributed execution failed: %w", err)
	}
	o.ttsLifecycle.EndSession(ctx, ttsSessionID)

	// 5. タスクを履歴に追加し、セッションを保存
	if err := o.sessions.SaveCompletedTask(ctx, sess, t); err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to save session: %w", err)
	}

	log.Printf("[DistributedOrch] ProcessMessage COMPLETE: jobID=%s route=%s response_len=%d",
		jobID.String(), decision.Route, len(response))
	if decision.Route == routing.RouteCHAT {
		o.saveExecutionReport(ctx, jobID.String(), req.UserMessage, string(decision.Route), startedAt, time.Now().UTC(), nil)
	}

	return ProcessMessageResponse{
		Response:   response,
		Route:      decision.Route,
		Confidence: decision.Confidence,
		JobID:      jobID.String(),
	}, nil
}

func (o *DistributedOrchestrator) saveExecutionReport(ctx context.Context, jobID, goal, route string, startedAt, finishedAt time.Time, runErr error) {
	o.evidence.Save(ctx, jobID, goal, route, startedAt, finishedAt, runErr)
}

// executeDistributed はルートに応じてTransport経由でAgent間通信
func (o *DistributedOrchestrator) executeDistributed(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
	return o.routes.ExecuteTask(ctx, t, route, sessionID, ttsSessionID)
}

func (o *DistributedOrchestrator) executeAutonomousDistributed(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
	return o.autonomous.Execute(ctx, t, route, sessionID, ttsSessionID)
}

func (o *DistributedOrchestrator) executeDistributedDirect(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
	return o.routes.ExecuteDirect(ctx, t, route, sessionID, ttsSessionID)
}

func (o *DistributedOrchestrator) withStreamHooks(
	ctx context.Context,
	route routing.Route,
	jid, sessionID, channel, chatID, ttsSessionID string,
) (context.Context, *streamBundle) {
	return o.ttsLifecycle.WithStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
}

func (o *DistributedOrchestrator) pushTTS(ctx context.Context, sessionID string, route routing.Route, eventType, text string) {
	o.ttsLifecycle.Push(ctx, sessionID, route, eventType, text)
}

func (o *DistributedOrchestrator) executeCodeViaShiro(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	sessionID, jid string,
) (string, error) {
	return o.codeExecution.Execute(ctx, t, route, sessionID, jid)
}

// executeViaSSH はSSH Transport経由でリモートAgentと通信
// SSHTransportは1:1接続のため、同一transport上でSend→Receiveする
func (o *DistributedOrchestrator) executeViaSSH(ctx context.Context, sshTransport domaintransport.Transport, targetAgent string, msg domaintransport.Message) (string, error) {
	return o.transports.ExecuteViaSSH(ctx, sshTransport, targetAgent, msg)
}

func (o *DistributedOrchestrator) executeToAgent(ctx context.Context, targetAgent string, msg domaintransport.Message) (domaintransport.Message, error) {
	return o.transports.ExecuteToAgent(ctx, targetAgent, msg)
}

func (o *DistributedOrchestrator) executeToAgentViaMailbox(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
	return o.transports.ExecuteToAgentViaMailbox(ctx, targetAgent, msg, receiveOnAgent)
}

// executeViaLocal はMessageRouter経由でローカルAgentと通信
func (o *DistributedOrchestrator) executeViaLocal(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
	return o.transports.ExecuteViaLocal(ctx, targetAgent, msg, receiveOnAgent)
}

func (o *DistributedOrchestrator) distributedWaitTimeout(targetAgent string, msg domaintransport.Message) time.Duration {
	if isCoderAgent(targetAgent) {
		return o.coderTimeoutOrDefault()
	}
	return distributedWaitTimeout(targetAgent, msg)
}

// routeToAgent はルートをAgent名にマッピング
func (o *DistributedOrchestrator) routeToAgent(route routing.Route) string {
	switch route {
	case routing.RouteOPS:
		return "shiro"
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return "shiro"
	case routing.RouteCHAT, routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return "" // mio がローカル処理
	default:
		return ""
	}
}

func (o *DistributedOrchestrator) routeToCoder(route routing.Route) string {
	return o.coderSelector.RouteToCoder(route)
}

func (o *DistributedOrchestrator) routeToCoderForMessage(route routing.Route, userMessage string) string {
	return o.coderSelector.RouteToCoderForMessage(route, userMessage)
}

func (o *DistributedOrchestrator) isCoderConnected(agent string) bool {
	return o.coderSelector.IsCoderConnected(agent)
}

func isCodeRoute(route routing.Route) bool {
	switch route {
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return true
	default:
		return false
	}
}

func (o *DistributedOrchestrator) withAttributionGuard(t task.Task, targetAgent, sessionID string) task.Task {
	return o.attribution.Apply(t, targetAgent, sessionID)
}

func (o *DistributedOrchestrator) buildAttributionGuardedMessage(userMessage, targetAgent, sessionID string) string {
	return o.attribution.BuildMessage(userMessage, targetAgent, sessionID)
}
