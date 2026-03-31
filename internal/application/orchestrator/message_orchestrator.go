package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	autonomousapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/autonomous"
	contractapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/contract"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	domaincontract "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/contract"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
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
	workerExecution service.WorkerExecutionService
	coderStatus     *CoderStatus
	codeExecutor    CodeExecutor // Phase 1リファクタリング: コード実行を委譲
	listener        EventListener
	reporter        ReportStore
	idleNotifier    IdleNotifier
	ttsBridge       TTSBridge
	vtuberBridge    VTuberBridge
	maxRepair       int // 0以下は1とみなす
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

	return &MessageOrchestrator{
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

func (o *MessageOrchestrator) SetReportStore(store ReportStore) {
	o.reporter = store
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *MessageOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
}

// SetTTSBridge sets an optional TTS bridge.
func (o *MessageOrchestrator) SetTTSBridge(b TTSBridge) {
	o.ttsBridge = b
}

// SetVTuberBridge sets an optional VTuber bridge.
func (o *MessageOrchestrator) SetVTuberBridge(b VTuberBridge) {
	o.vtuberBridge = b
}

func (o *MessageOrchestrator) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if o.listener == nil {
		log.Printf("[MessageOrch] emit SKIPPED: no listener (eventType=%s from=%s to=%s)", eventType, from, to)
		return
	}
	log.Printf("[MessageOrch] emit: eventType=%s from=%s to=%s route=%s jobID=%s", eventType, from, to, route, jobID)
	o.listener.OnEvent(NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID))
}

// ProcessMessage はメッセージを処理
func (o *MessageOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	log.Printf("[MessageOrch] ProcessMessage START: sessionID=%s channel=%s chatID=%s message=%q",
		req.SessionID, req.Channel, req.ChatID, req.UserMessage)

	if o.idleNotifier != nil {
		o.idleNotifier.NotifyActivity()
		o.idleNotifier.SetChatBusy(true)
		defer o.idleNotifier.SetChatBusy(false)
	}

	// 1. セッションをロードまたは作成
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to load or create session: %v", err)
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}
	log.Printf("[MessageOrch] Session loaded/created: %s", sess.ID())

	// Event: ユーザーメッセージ受信
	o.emit("message.received", "user", "mio", req.UserMessage, "", "", req.SessionID, req.Channel, req.ChatID)

	// 2. チャットコマンドのチェック（ルーティング前）
	cmdResult, err := o.mio.HandleChatCommand(ctx, req.ChatID, req.UserMessage)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("chat command failed: %w", err)
	}
	if cmdResult.Handled {
		o.emit("agent.response", "mio", "user", cmdResult.Response, "CHAT", "", req.SessionID, req.Channel, req.ChatID)
		return ProcessMessageResponse{
			Response:   cmdResult.Response,
			Route:      routing.RouteCHAT,
			Confidence: 1.0,
			JobID:      task.NewJobID().String(),
		}, nil
	}

	// 3. タスクを作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID)
	ttsSessionID := ""
	if o.ttsBridge != nil {
		ttsSessionID = fmt.Sprintf("%s-%s", req.SessionID, jobID.String())
	}

	// 4. ルーティング決定
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}

	// Event: ルーティング決定
	o.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)

	// タスクにルートを設定
	t = t.WithRoute(decision.Route)
	if o.ttsBridge != nil && ttsSessionID != "" {
		ttsCtx := buildTTSContext(decision.Route, "normal", false)
		voiceID, voiceProfile := voiceForSpeaker(speakerForRoute(decision.Route))
		startReq := TTSSessionStart{
			SessionID:             ttsSessionID,
			ResponseID:            jobID.String(),
			CharacterID:           speakerForRoute(decision.Route),
			VoiceID:               voiceID,
			SpeechMode:            speechModeForRoute(decision.Route),
			Event:                 eventForRoute(decision.Route),
			Urgency:               ttsCtx.Urgency,
			ConversationMode:      ttsCtx.ConversationMode,
			UserAttentionRequired: ttsCtx.UserAttentionRequired,
			Context:               ttsCtx,
			VoiceProfile:          voiceProfile,
		}
		if err := o.ttsBridge.StartSession(ctx, startReq); err != nil {
			log.Printf("[MessageOrch] TTS route update degraded: %v", err)
		}
	}

	workerMarkedBusy := false
	if o.idleNotifier != nil && decision.Route != routing.RouteCHAT {
		o.idleNotifier.SetWorkerBusy(true)
		workerMarkedBusy = true
	}
	if workerMarkedBusy {
		defer o.idleNotifier.SetWorkerBusy(false)
	}

	// 4. ルートに応じて実行
	response, err := o.executeTask(ctx, t, decision.Route, req.SessionID, req.Channel, req.ChatID, ttsSessionID)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("task execution failed: %w", err)
	}
	if ttsSessionID != "" {
		if err := o.ttsBridge.EndSession(ctx, ttsSessionID); err != nil {
			log.Printf("[MessageOrch] TTS end degraded: %v", err)
		}
	}

	// 5. タスクを履歴に追加
	sess.AddTask(t)

	// 6. セッションを保存
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to save session: %v", err)
		return ProcessMessageResponse{}, fmt.Errorf("failed to save session: %w", err)
	}

	log.Printf("[MessageOrch] ProcessMessage COMPLETE: jobID=%s route=%s response_len=%d",
		jobID.String(), decision.Route, len(response))

	return ProcessMessageResponse{
		Response:   response,
		Route:      decision.Route,
		Confidence: decision.Confidence,
		JobID:      jobID.String(),
	}, nil
}

// loadOrCreateSession はセッションをロードまたは作成
func (o *MessageOrchestrator) loadOrCreateSession(ctx context.Context, id, channel, chatID string) (*session.Session, error) {
	sess, err := o.sessionRepo.Load(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrSessionNotFound) {
			// 新規セッション作成
			return session.NewSession(id, channel, chatID), nil
		}
		return nil, err
	}
	return sess, nil
}

// executeTask はルートに応じてタスクを実行
func (o *MessageOrchestrator) executeTask(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	if route != routing.RouteCHAT {
		return o.executeAutonomousTask(ctx, t, route, sessionID, channel, chatID, ttsSessionID)
	}

	switch route {
	case routing.RouteCHAT:
		o.emit("agent.start", "mio", "user", "考え中...", "CHAT", jid, sessionID, channel, chatID)
		streamCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(streamCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "CHAT", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err

	case routing.RouteOPS:
		o.emit("agent.start", "mio", "shiro", "タスクを実行依頼", "OPS", jid, sessionID, channel, chatID)
		resp, err := o.shiro.Execute(ctx, t)
		if err == nil {
			o.emit("agent.response", "shiro", "mio", resp, "OPS", jid, sessionID, channel, chatID)
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err

	case routing.RouteCODE:
		resp, err := o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)
		if err == nil {
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err

	case routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		resp, err := o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)
		if err == nil {
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err

	case routing.RoutePLAN:
		o.emit("agent.start", "mio", "user", "計画を検討中...", "PLAN", jid, sessionID, channel, chatID)
		planCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(planCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "PLAN", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err

	case routing.RouteANALYZE:
		o.emit("agent.start", "mio", "user", "分析中...", "ANALYZE", jid, sessionID, channel, chatID)
		analyzeCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(analyzeCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "ANALYZE", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err

	case routing.RouteRESEARCH:
		o.emit("agent.start", "mio", "user", "調査中...", "RESEARCH", jid, sessionID, channel, chatID)
		researchCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(researchCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "RESEARCH", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err

	default:
		return "", fmt.Errorf("unknown route: %s", route)
	}
}

func (o *MessageOrchestrator) executeAutonomousTask(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	if !isAutonomousRoute(route) {
		return "", fmt.Errorf("unknown route: %s", route)
	}
	contract, err := contractapp.NormalizeRequestWithRoute(t.UserMessage(), route.String())
	if err != nil {
		return "", err
	}
	result, err := autonomousapp.RunExecutor(ctx, autonomousapp.ExecuteRequest{
		JobID:      t.JobID().String(),
		Route:      route.String(),
		Capability: capabilityForRoute(route),
		Contract:   contract,
		MaxRepair:  o.maxRepairOrDefault(),
		Observe: func(stage autonomousapp.Stage) {
			o.emit("entry.stage", channel, "system", string(stage), route.String(), t.JobID().String(), sessionID, channel, chatID)
		},
		ReportStore: o.reporter,
		Execute: func(execCtx context.Context, attempt int, failureKind, failureReason string) (autonomousapp.AttemptResult, error) {
			execTask := t
			if attempt > 0 {
				execTask = execTask.WithUserMessage(buildExecutorRetryMessage(t.UserMessage(), route, failureKind, failureReason, attempt))
			}
			resp, runErr := o.executeRouteDirect(execCtx, execTask, route, sessionID, channel, chatID, ttsSessionID)
			return autonomousapp.AttemptResult{
				Response:      resp,
				Steps:         routeExecutionSteps(route, runErr == nil),
				FailureKind:   classifyExecutorFailure(runErr),
				FailureReason: errorString(runErr),
			}, runErr
		},
		Verify: func(_ context.Context, c domaincontract.Contract, last autonomousapp.AttemptResult) (bool, string, string, error) {
			ok, kind, reason := verifyByContract(route, c, last)
			return ok, kind, reason, nil
		},
	})
	if err != nil {
		return result.Response, err
	}
	return result.Response, nil
}

func (o *MessageOrchestrator) executeRouteDirect(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	switch route {
	case routing.RouteOPS:
		o.emit("agent.start", "mio", "shiro", "タスクを実行依頼", "OPS", jid, sessionID, channel, chatID)
		resp, err := o.shiro.Execute(ctx, t)
		if err == nil {
			o.emit("agent.response", "shiro", "mio", resp, "OPS", jid, sessionID, channel, chatID)
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		resp, err := o.executeCodeViaShiro(ctx, t, route, sessionID, channel, chatID)
		if err == nil {
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err
	case routing.RoutePLAN:
		o.emit("agent.start", "mio", "user", "計画を検討中...", "PLAN", jid, sessionID, channel, chatID)
		planCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(planCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "PLAN", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err
	case routing.RouteANALYZE:
		o.emit("agent.start", "mio", "user", "分析中...", "ANALYZE", jid, sessionID, channel, chatID)
		analyzeCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(analyzeCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "ANALYZE", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err
	case routing.RouteRESEARCH:
		o.emit("agent.start", "mio", "user", "調査中...", "RESEARCH", jid, sessionID, channel, chatID)
		researchCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, channel, chatID, ttsSessionID)
		resp, err := o.mio.Chat(researchCtx, t)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, "RESEARCH", jid, sessionID, channel, chatID)
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err
	default:
		return "", fmt.Errorf("unsupported autonomous route: %s", route)
	}
}

func (o *MessageOrchestrator) withStreamHooks(
	ctx context.Context,
	route routing.Route,
	jid, sessionID, channel, chatID, ttsSessionID string,
) (context.Context, *streamBundle) {
	prev := llm.StreamCallbackFromContext(ctx)
	ttsStream := newTTSStreamForwarder(o.ttsBridge, ttsSessionID, route, "agent.response", "[MessageOrch] TTS push degraded:")
	vtuberStream := newVTuberStreamForwarder(o.vtuberBridge, ttsSessionID, route, "agent.response", "[MessageOrch] VTuber push degraded:")
	return llm.ContextWithStreamCallback(ctx, func(token string) {
		if prev != nil {
			prev(token)
		}
		o.emit("agent.thinking", "mio", "user", token, string(route), jid, sessionID, channel, chatID)
		ttsStream.OnToken(ctx, token)
		vtuberStream.OnToken(ctx, token)
	}), &streamBundle{tts: ttsStream, vtuber: vtuberStream}
}

func (o *MessageOrchestrator) pushTTS(ctx context.Context, sessionID string, route routing.Route, eventType, text string) {
	ttsCtx := buildTTSContext(route, "normal", false)
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	filtered, emotion := buildTTSPayload(eventType, route, text, ttsCtx, voiceProfile)
	pushTTS(ctx, o.ttsBridge, sessionID, filtered, emotion, "[MessageOrch] TTS push degraded:")
	req, ok := buildVTuberRequest(eventType, route, sessionID, text, ttsCtx, voiceProfile)
	if ok {
		pushVTuber(ctx, o.vtuberBridge, req, "[MessageOrch] VTuber push degraded:")
	}
}

func speechModeForRoute(route routing.Route) string {
	switch route {
	case routing.RouteOPS:
		return "report"
	case routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return "report"
	default:
		return "conversational"
	}
}

func (o *MessageOrchestrator) executeCodeViaShiro(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	sessionID, channel, chatID string,
) (string, error) {
	// Phase 1リファクタリング: CodeExecutorに委譲
	req := CodeExecutionRequest{
		Task:      t,
		Route:     route,
		SessionID: sessionID,
		Channel:   channel,
		ChatID:    chatID,
		JobID:     t.JobID().String(),
	}
	resp, err := o.codeExecutor.ExecuteCode(ctx, req)
	return resp.Response, err
}

func capabilityForRoute(route routing.Route) autonomousapp.CapabilityPack {
	switch route {
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return autonomousapp.CapabilityCodeChange
	default:
		return autonomousapp.CapabilityGenericExecution
	}
}

func isAutonomousRoute(route routing.Route) bool {
	switch route {
	case routing.RouteOPS, routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return true
	default:
		return false
	}
}

func routeExecutionSteps(route routing.Route, ok bool) []string {
	items := []string{"routing.decision"}
	switch route {
	case routing.RouteOPS:
		items = append(items, "shiro.execute")
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		items = append(items, "shiro.delegate", "coder.execute", "shiro.verify")
	case routing.RoutePLAN:
		items = append(items, "mio.plan")
	case routing.RouteANALYZE:
		items = append(items, "mio.analyze")
	case routing.RouteRESEARCH:
		items = append(items, "mio.research")
	}
	if ok {
		items = append(items, "done")
	} else {
		items = append(items, "error")
	}
	return items
}

func classifyExecutorFailure(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	switch {
	case strings.Contains(lower, "proposal"):
		return "proposal_invalid"
	case strings.Contains(lower, "not found"), strings.Contains(lower, "exit status 127"):
		return "command_missing"
	case strings.Contains(lower, "provider"), strings.Contains(lower, "model"), strings.Contains(lower, "ollama"):
		return "provider_unavailable"
	default:
		return "apply"
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func responseLooksLikeFailure(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "失敗: 0") || strings.Contains(lower, "failures: 0") || strings.Contains(lower, "failed: 0") {
		return false
	}
	return strings.Contains(lower, "error") || strings.Contains(lower, "失敗") ||
		strings.Contains(content, "エラー")
}

func shortFailureReason(content string) string {
	text := strings.TrimSpace(content)
	if len(text) <= 160 {
		return text
	}
	return text[:157] + "..."
}

// verifyByContract はルートと実行契約に基づいて AttemptResult を検証する。
// verifyAutonomousRouteResponse の後継。
func verifyByContract(
	route routing.Route,
	c domaincontract.Contract,
	last autonomousapp.AttemptResult,
) (bool, string, string) {
	// (1) 全ルート共通: 空レスポンス拒否
	if strings.TrimSpace(last.Response) == "" {
		return false, "verification_failed", "empty response"
	}

	// (2) TTS CapabilityPack 検証
	if isTTSCapability(c) {
		return verifyTTSResult(last)
	}

	// (3) CODE ルート検証
	if isCodeRoute(route) {
		if looksLikeNonExecutable(last.Response) {
			return false, "non_executable_output",
				"Coder output contains design document only; executable patch is required"
		}
		if responseLooksLikeFailure(last.Response) {
			return false, "verification_failed", shortFailureReason(last.Response)
		}
		return true, "", ""
	}

	// (4) OPS / PLAN / ANALYZE / RESEARCH
	if responseLooksLikeFailure(last.Response) {
		return false, "verification_failed", shortFailureReason(last.Response)
	}
	return true, "", ""
}

// isTTSCapability は契約の Acceptance フィールドから TTS CapabilityPack かどうかを判定する。
func isTTSCapability(c domaincontract.Contract) bool {
	for _, a := range c.Acceptance {
		if strings.Contains(a, "実再生") || strings.Contains(a, "音声ファイル生成") {
			return true
		}
	}
	return false
}

// verifyTTSResult は TTS CapabilityPack の E2E 検証を行う。
// PlaybackCode/TTSAudioFile が未設定の場合は暫定フォールバック（レスポンス文字列チェック）。
func verifyTTSResult(last autonomousapp.AttemptResult) (bool, string, string) {
	// Phase 2 で TTS ブリッジ結果が注入されるまでの暫定フォールバック
	if last.TTSAudioFile == "" && last.PlaybackCode == 0 {
		if responseLooksLikeFailure(last.Response) {
			return false, "verification_failed", shortFailureReason(last.Response)
		}
		return true, "", ""
	}
	if last.TTSAudioFile == "" {
		return false, "tts_no_audio", "音声ファイルが生成されていない (TTSAudioFile が空)"
	}
	if last.PlaybackCode != 0 {
		return false, "playback_failed",
			fmt.Sprintf("再生コマンドが終了コード %d で終了した", last.PlaybackCode)
	}
	return true, "", ""
}

// looksLikeNonExecutable は Coder の出力が設計文書のみで実行可能形式を含まないかを判定する。
func looksLikeNonExecutable(response string) bool {
	lower := strings.ToLower(response)
	executables := []string{
		"```",             // コードブロック
		"patch:",          // Shiro patch セクション
		"apply:",          // patch 適用指示
		"execute:",        // 実行指示
		"$ ",              // シェルコマンド
		"#!/",             // シェバン
		"execution result", // formatExecutionResult のセクションヘッダー（実行証跡）
		"success rate",    // formatExecutionResult の実行結果（実行証跡）
	}
	for _, marker := range executables {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func buildExecutorRetryMessage(userMessage string, route routing.Route, failureKind, failureReason string, attempt int) string {
	return fmt.Sprintf(`%s

## Executor Retry Context
- retry_attempt: %d
- route: %s
- failure_kind: %s
- failure_reason: %s

## Requirements
- Keep the response executable and directly verifiable
- Include the missing repair steps in the next result
- Do not defer required fixes to the user
`, userMessage, attempt, route, fallbackString(failureKind, "unknown"), fallbackString(failureReason, "execution failed"))
}

