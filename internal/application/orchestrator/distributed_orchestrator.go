package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
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
		return
	}
	o.nodeCaps = caps
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
	coderAgent := o.routeToCoderForMessage(route, t.UserMessage())
	if coderAgent == "" {
		return "", fmt.Errorf("no coder mapped for route %s", route)
	}
	log.Printf("[DistributedOrch] code handoff route=%s target=%s job=%s", route, coderAgent, jid)

	o.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emitNote("mio", "user", "しろにコード実装の取りまとめをお願いしたよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
	requestText := t.UserMessage()

	for attempt := 0; attempt <= o.coderRetryMaxOrDefault(); attempt++ {
		o.emit("agent.start", "shiro", coderAgent, requestText, string(route), jid, sessionID, t.Channel(), t.ChatID())
		if attempt == 0 {
			o.emitNote("shiro", "mio", fmt.Sprintf("%sにコーディング依頼しました。進捗を監視して、必要なら作業を前に進めます。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
		} else {
			o.emit("worker.retry_request", "shiro", coderAgent, fmt.Sprintf("retry=%d", attempt), string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.emitNote("shiro", "mio", fmt.Sprintf("%sに修正版patchを再依頼します。retry=%d", displayAgentName(coderAgent), attempt), string(route), jid, sessionID, t.Channel(), t.ChatID())
		}

		coderMsg := domaintransport.NewMessage("shiro", coderAgent, sessionID, jid, requestText)
		coderMsg.Type = domaintransport.MessageTypeTask
		coderMsg.Context = map[string]interface{}{
			"route":         string(route),
			"retry_attempt": attempt,
			"channel":       t.Channel(),
			"chat_id":       t.ChatID(),
		}
		// v4.1: SSH 経由の場合、CoderConfig を Context に含める
		if o.coderConfigs != nil {
			if coderCfg, ok := o.coderConfigs[coderAgent]; ok {
				coderMsg.Context["coder_config"] = coderCfg
			}
		}
		o.memory.RecordMessage(coderMsg)

		coderResult, err := o.executeToAgentViaMailbox(ctx, coderAgent, coderMsg, "mio")
		if err != nil {
			failureKind, reason, retryable := classifyDistributedExecutionError(err)
			if retryable && attempt < o.coderRetryMaxOrDefault() {
				o.emit("worker.classified_failure", "shiro", coderAgent, fmt.Sprintf("%s: %s", failureKind, reason), string(route), jid, sessionID, t.Channel(), t.ChatID())
				requestText = buildCoderRetryInstruction(t.UserMessage(), nil, failureKind, reason, attempt+1)
				continue
			}
			return "", err
		}
		o.emit("agent.response", coderAgent, "shiro", coderResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote(coderAgent, "shiro", "おわったっす。", string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote("shiro", "mio", fmt.Sprintf("%sの結果を受け取って、内容確認と仕上げを進めます。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())

		if coderResult.Proposal == nil {
			o.emit("agent.start", "shiro", "mio", "Coder結果をShiroで整形", string(route), jid, sessionID, t.Channel(), t.ChatID())
			shiroTask := domaintransport.NewMessage("mio", "shiro", sessionID, jid, coderResult.Content)
			shiroTask.Type = domaintransport.MessageTypeTask
			shiroTask.Context = map[string]interface{}{
				"route":       string(route),
				"coder_agent": coderAgent,
				"channel":     t.Channel(),
				"chat_id":     t.ChatID(),
			}
			o.memory.RecordMessage(shiroTask)
			shiroResult, err := o.executeToAgent(ctx, "shiro", shiroTask)
			if err != nil {
				return "", err
			}
			o.emit("agent.response", "shiro", "mio", shiroResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.emitNote("shiro", "mio", fmt.Sprintf("%sの作業が終わりました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
			return shiroResult.Content, nil
		}

		o.emit("agent.start", "shiro", "mio", "CoderのProposalをWorker実行", string(route), jid, sessionID, t.Channel(), t.ChatID())

		execMsg := domaintransport.NewMessage("mio", "shiro", sessionID, jid, "Execute coder proposal")
		execMsg.Type = domaintransport.MessageTypeTask
		execMsg.Context = map[string]interface{}{
			"route":         string(route),
			"coder_agent":   coderAgent,
			"retry_attempt": attempt,
			"channel":       t.Channel(),
			"chat_id":       t.ChatID(),
		}
		execMsg.Proposal = coderResult.Proposal
		o.memory.RecordMessage(execMsg)

		shiroResult, err := o.executeToAgent(ctx, "shiro", execMsg)
		if err != nil {
			failureKind, reason, retryable := classifyDistributedExecutionError(err)
			if retryable && attempt < o.coderRetryMaxOrDefault() {
				o.emit("worker.classified_failure", "shiro", coderAgent, fmt.Sprintf("%s: %s", failureKind, reason), string(route), jid, sessionID, t.Channel(), t.ChatID())
				requestText = buildCoderRetryInstruction(t.UserMessage(), coderResult.Proposal, failureKind, reason, attempt+1)
				continue
			}
			return "", err
		}
		o.emit("agent.response", "shiro", "mio", shiroResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote("shiro", "mio", fmt.Sprintf("%sの作業が終わりました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())

		if retryReq, ok := nextCoderRetryRequest(t.UserMessage(), coderResult.Proposal, shiroResult, attempt); ok {
			requestText = retryReq
			continue
		}
		return shiroResult.Content, nil
	}

	return "", fmt.Errorf("coder retry budget exhausted for job %s", jid)
}

func nextCoderRetryRequest(userMessage string, proposal *domaintransport.ProposalPayload, shiroResult domaintransport.Message, attempt int) (string, bool) {
	if shiroResult.Result == nil || shiroResult.Result.Success || !shiroResult.Result.Retryable {
		return "", false
	}
	if attempt >= distributedCoderRetryMax {
		return "", false
	}
	return buildCoderRetryInstruction(userMessage, proposal, shiroResult.Result.FailureKind, shiroResult.Result.FailureReason, attempt+1), true
}

func classifyDistributedExecutionError(err error) (string, string, bool) {
	if err == nil {
		return "", "", false
	}
	text := err.Error()
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, agent.ProposalFailureEmpty),
		strings.Contains(lower, agent.ProposalFailureMissingPlan),
		strings.Contains(lower, agent.ProposalFailureMissingPatch),
		strings.Contains(lower, agent.ProposalFailureInvalidPatch):
		return proposalFailureKindFromText(lower), text, true
	case strings.Contains(lower, agent.ProposalFailureDisallowedCommand):
		return agent.ProposalFailureDisallowedCommand, text, false
	case strings.Contains(lower, "patch parse error"):
		return "patch_parse_failed", text, true
	case strings.Contains(lower, "command not found"), strings.Contains(lower, "exit status 127"), strings.Contains(lower, "not found"):
		return "missing_command", text, true
	case strings.Contains(lower, "security error"), strings.Contains(lower, "protected file"):
		return "unsafe_operation", text, false
	default:
		return "unknown", text, false
	}
}

func proposalFailureKindFromText(lower string) string {
	switch {
	case strings.Contains(lower, agent.ProposalFailureMissingPlan):
		return agent.ProposalFailureMissingPlan
	case strings.Contains(lower, agent.ProposalFailureMissingPatch):
		return agent.ProposalFailureMissingPatch
	case strings.Contains(lower, agent.ProposalFailureInvalidPatch):
		return agent.ProposalFailureInvalidPatch
	default:
		return agent.ProposalFailureEmpty
	}
}

func buildCoderRetryInstruction(userMessage string, proposal *domaintransport.ProposalPayload, failureKind, failureReason string, retry int) string {
	var prevPlan, prevPatch string
	if proposal != nil {
		prevPlan = strings.TrimSpace(proposal.Plan)
		prevPatch = strings.TrimSpace(proposal.Patch)
	}
	return fmt.Sprintf(`%s

## Retry Context
- retry_attempt: %d
- failure_kind: %s
- failure_reason: %s

## Worker Requirements
- Return a Worker-executable patch only
- Keep the patch directly parseable and runnable
- Include the environment repair or verification steps inside Patch
- Do not use bare pip; use python3 -m pip or python -m pip if Python package installation is truly required
- Prefer concrete file edits and deterministic non-interactive commands

## Previous Proposal Plan
%s

## Previous Proposal Patch
%s
`, userMessage, retry, fallbackString(failureKind, "unknown"), fallbackString(failureReason, "execution failed"), fallbackString(prevPlan, "(none)"), truncate(fallbackString(prevPatch, "(none)"), 1600))
}

func fallbackString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
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

func transportMode(sshTransports map[string]domaintransport.Transport, targetAgent string) string {
	if _, ok := sshTransports[targetAgent]; ok {
		return "ssh"
	}
	return "local"
}

func routeAndChannelFromMessage(msg domaintransport.Message) (route, channel, chatID string) {
	if msg.Context == nil {
		return "", "", ""
	}
	route = stringContextValue(msg.Context, "route")
	channel = stringContextValue(msg.Context, "channel")
	chatID = stringContextValue(msg.Context, "chat_id")
	return route, channel, chatID
}

func stringContextValue(ctx map[string]interface{}, key string) string {
	raw, ok := ctx[key]
	if !ok || raw == nil {
		return ""
	}
	v, ok := raw.(string)
	if !ok {
		return ""
	}
	return v
}

// distributedWaitTimeout はエージェント種別とメッセージ内容に基づくタイムアウト時間を返す（パッケージレベル関数）。
// テストから直接呼べるよう、デフォルト定数を使う版。
func distributedWaitTimeout(targetAgent string, msg domaintransport.Message) time.Duration {
	switch {
	case strings.HasPrefix(targetAgent, "coder"):
		return distributedCoderTimeout
	case targetAgent == "shiro" && msg.Proposal != nil:
		return distributedWorkerTimeout
	default:
		return distributedDefaultTimeout
	}
}

func (o *DistributedOrchestrator) distributedWaitTimeout(targetAgent string, msg domaintransport.Message) time.Duration {
	if strings.HasPrefix(targetAgent, "coder") {
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
	switch route {
	case routing.RouteCODE:
		for _, coder := range []string{"coder1", "coder2", "coder3", "coder4"} {
			if o.isCoderConnected(coder) {
				log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=fallback_chain", route, coder)
				return coder
			}
			log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, coder)
		}
		return ""
	case routing.RouteCODE1:
		if o.isCoderConnected("coder1") {
			log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=explicit", route, "coder1")
			return "coder1"
		}
		log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, "coder1")
		return ""
	case routing.RouteCODE2:
		if o.isCoderConnected("coder2") {
			log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=explicit", route, "coder2")
			return "coder2"
		}
		log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, "coder2")
		return ""
	case routing.RouteCODE3:
		if o.isCoderConnected("coder3") {
			log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=explicit", route, "coder3")
			return "coder3"
		}
		log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, "coder3")
		return ""
	case routing.RouteCODE4:
		if o.isCoderConnected("coder4") {
			log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=explicit", route, "coder4")
			return "coder4"
		}
		log.Printf("[DistributedOrch] coder skip route=%s target=%s reason=unconnected", route, "coder4")
		return ""
	default:
		return ""
	}
}

func (o *DistributedOrchestrator) routeToCoderForMessage(route routing.Route, userMessage string) string {
	if route != routing.RouteCODE || o.nodeSelector == nil || len(o.nodeCaps) == 0 {
		return o.routeToCoder(route)
	}
	candidates := make([]string, 0, 4)
	for _, coder := range []string{"coder1", "coder2", "coder3", "coder4"} {
		if o.isCoderConnected(coder) {
			candidates = append(candidates, coder)
		}
	}
	req := inferTaskRequirement(userMessage)
	selected := o.nodeSelector.Select(candidates, o.nodeCaps, req)
	if selected != "" {
		log.Printf("[DistributedOrch] coder selected route=%s target=%s mode=capability candidates=%v req=%+v", route, selected, candidates, req)
		return selected
	}
	log.Printf("[DistributedOrch] coder capability select fell back route=%s candidates=%v req=%+v", route, candidates, req)
	return o.routeToCoder(route)
}

func (o *DistributedOrchestrator) isCoderConnected(agent string) bool {
	if _, ok := o.sshTransports[agent]; ok {
		return true
	}
	if o.router == nil {
		return false
	}
	_, ok := o.router.GetAgent(agent)
	return ok
}

func isCodeRoute(route routing.Route) bool {
	switch route {
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return true
	default:
		return false
	}
}

func displayAgentName(agentID string) string {
	switch strings.ToLower(agentID) {
	case "mio":
		return "みお"
	case "shiro":
		return "しろ"
	case "coder1":
		return "あか"
	case "coder2":
		return "あお"
	case "coder3":
		return "ぎん"
	default:
		return agentID
	}
}

func routeNoticeText(route routing.Route, userMessage string) string {
	switch route {
	case routing.RouteCHAT:
		return "みおが会話として対応するよ。"
	case routing.RouteOPS:
		return "しろに運用作業をお願いしたよ。"
	case routing.RoutePLAN:
		return "計画モードで整理するよ。"
	case routing.RouteANALYZE:
		return "分析として進めるよ。"
	case routing.RouteRESEARCH:
		return "調査タスクとして進めるよ。"
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return fmt.Sprintf("しろ経由でコーディング依頼に回したよ（依頼: %s）。", truncateForNote(userMessage, 32))
	default:
		return "処理経路を決めて進めるよ。"
	}
}

func truncateForNote(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "..."
}

func (o *DistributedOrchestrator) withAttributionGuard(t task.Task, targetAgent, sessionID string) task.Task {
	if targetAgent == "" || isCodeRoute(t.Route()) || strings.Contains(t.UserMessage(), "【発言帰属ガード】") {
		return t
	}
	guarded := o.buildAttributionGuardedMessage(t.UserMessage(), targetAgent, sessionID)
	if guarded == t.UserMessage() {
		return t
	}
	out := task.NewTask(t.JobID(), guarded, t.Channel(), t.ChatID())
	if t.HasForcedRoute() {
		out = out.WithForcedRoute(t.ForcedRoute())
	}
	if t.Route() != "" {
		out = out.WithRoute(t.Route())
	}
	return out
}

func (o *DistributedOrchestrator) buildAttributionGuardedMessage(userMessage, targetAgent, sessionID string) string {
	entries := o.memory.GetUnifiedView(120)
	selfLines := make([]string, 0, 3)
	otherLines := make([]string, 0, 3)

	for i := len(entries) - 1; i >= 0 && (len(selfLines) < 3 || len(otherLines) < 3); i-- {
		m := entries[i].Message
		if m.SessionID != sessionID || strings.TrimSpace(m.Content) == "" {
			continue
		}
		if m.Type == domaintransport.MessageTypeIdleChat || strings.HasPrefix(strings.ToLower(m.SessionID), "idle-") {
			continue
		}
		line := truncateForNote(strings.TrimSpace(m.Content), 90)
		if strings.EqualFold(m.From, targetAgent) {
			if len(selfLines) < 3 {
				selfLines = append(selfLines, line)
			}
			continue
		}
		if len(otherLines) < 3 {
			otherLines = append(otherLines, fmt.Sprintf("%s: %s", m.From, line))
		}
	}

	if len(selfLines) == 0 && len(otherLines) == 0 {
		return userMessage
	}
	if len(selfLines) == 0 {
		selfLines = append(selfLines, "なし")
	}
	if len(otherLines) == 0 {
		otherLines = append(otherLines, "なし")
	}

	guard := fmt.Sprintf(
		"【発言帰属ガード】\nあなたは %s。\n自分の過去発言: %s\n他者の発言: %s\n要件: 他者の発言や既出案を自分の新規アイデアとして言い換えない。参照時は発言者を明示する。",
		targetAgent,
		strings.Join(selfLines, " / "),
		strings.Join(otherLines, " / "),
	)
	return guard + "\n\n【ユーザー依頼】\n" + userMessage
}
