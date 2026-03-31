package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	autonomousapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/autonomous"
	contractapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/contract"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	domaincontract "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/contract"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
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
	router        *transport.MessageRouter
	memory        *session.CentralMemory
	sshTransports map[string]domaintransport.Transport // SSH経由のリモートAgent
	listener      EventListener
	reporter      ReportStore
	idleNotifier  IdleNotifier
	nodeSelector  *NodeSelector
	nodeCaps      map[string]domainnode.ResourceProfile
	coderConfigs  map[string]interface{} // v4.1: coder1-4 の CoderConfig（SSH送信用）
	ttsBridge      TTSBridge
	vtuberBridge   VTuberBridge
	maxRepair      int           // 0以下は1とみなす
	coderTimeout   time.Duration // 0以下は distributedCoderTimeout とみなす
	coderRetryMax  int           // 0以下は distributedCoderRetryMax とみなす
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
	return &DistributedOrchestrator{
		sessionRepo:   sessionRepo,
		mio:           mio,
		router:        router,
		memory:        memory,
		sshTransports: sshTransports,
		nodeSelector:  NewNodeSelector(),
		nodeCaps:      make(map[string]domainnode.ResourceProfile),
	}
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
}

func (o *DistributedOrchestrator) SetReportStore(store ReportStore) {
	o.reporter = store
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *DistributedOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
}

// SetTTSBridge sets an optional TTS bridge.
func (o *DistributedOrchestrator) SetTTSBridge(b TTSBridge) {
	o.ttsBridge = b
}

// SetVTuberBridge sets an optional VTuber bridge.
func (o *DistributedOrchestrator) SetVTuberBridge(b VTuberBridge) {
	o.vtuberBridge = b
}

func (o *DistributedOrchestrator) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if o.listener == nil {
		return
	}
	o.listener.OnEvent(NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID))
}

func (o *DistributedOrchestrator) emitNote(from, to, content, route, jobID, sessionID, channel, chatID string) {
	o.emit("agent.note", from, to, content, route, jobID, sessionID, channel, chatID)
}

func (o *DistributedOrchestrator) emitProgress(eventType, from, to, content string, msg domaintransport.Message) {
	route, channel, chatID := routeAndChannelFromMessage(msg)
	o.emit(eventType, from, to, content, route, msg.JobID, msg.SessionID, channel, chatID)
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
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		log.Printf("[DistributedOrch] ProcessMessage ERROR: failed to load or create session: %v", err)
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}
	log.Printf("[DistributedOrch] Session loaded/created: %s", sess.ID())

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
	ttsSessionID := ""
	if o.ttsBridge != nil {
		ttsSessionID = fmt.Sprintf("%s-%s", req.SessionID, jobID.String())
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
			log.Printf("[DistributedOrch] TTS start degraded: %v", err)
			ttsSessionID = ""
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

	// 4. ルートに応じてTransport経由で実行
	response, err := o.executeDistributed(ctx, t, decision.Route, sess.ID(), ttsSessionID)
	if err != nil {
		if decision.Route == routing.RouteCHAT {
			o.saveExecutionReport(ctx, jobID.String(), req.UserMessage, string(decision.Route), startedAt, time.Now().UTC(), err)
		}
		return ProcessMessageResponse{}, fmt.Errorf("distributed execution failed: %w", err)
	}
	if ttsSessionID != "" {
		if err := o.ttsBridge.EndSession(ctx, ttsSessionID); err != nil {
			log.Printf("[DistributedOrch] TTS end degraded: %v", err)
		}
	}

	// 5. タスクを履歴に追加
	sess.AddTask(t)

	// 6. セッションを保存
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		log.Printf("[DistributedOrch] ProcessMessage ERROR: failed to save session: %v", err)
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
	if o.reporter == nil || strings.TrimSpace(jobID) == "" || strings.TrimSpace(goal) == "" {
		return
	}
	report := domainexecution.ExecutionReport{
		JobID:        jobID,
		Goal:         goal,
		Status:       "passed",
		ErrorKind:    "",
		Acceptance:   distributedAcceptance(route),
		Verification: distributedVerification(route, runErr),
		Steps:        distributedEvidenceSteps(route, runErr),
		RepairCount:  0,
		Error:        "",
		CreatedAt:    startedAt,
		FinishedAt:   finishedAt,
	}
	if runErr != nil {
		report.Status = "failed"
		report.ErrorKind = distributedEvidenceErrorKind(runErr)
		report.Error = runErr.Error()
	}
	if err := o.reporter.Save(ctx, report); err != nil {
		log.Printf("[DistributedOrch] evidence save failed: job=%s err=%v", jobID, err)
	}
}

func distributedAcceptance(route string) []string {
	items := []string{"ルーティング完了", "最終応答生成"}
	switch strings.ToUpper(strings.TrimSpace(route)) {
	case "CHAT":
		items = append(items, "Mio 応答完了")
	case "OPS":
		items = append(items, "Worker 応答完了")
	case "CODE", "CODE1", "CODE2", "CODE3", "CODE4":
		items = append(items, "Coder 実行完了", "Worker 取りまとめ完了")
	default:
		items = append(items, "Agent 応答完了")
	}
	return items
}

func distributedVerification(route string, runErr error) []string {
	items := []string{"viewer jobs に記録されること"}
	if strings.TrimSpace(route) != "" {
		items = append(items, fmt.Sprintf("route=%s", strings.ToUpper(strings.TrimSpace(route))))
	}
	if runErr == nil {
		items = append(items, "final:passed")
		return items
	}
	items = append(items, "final:failed")
	return items
}

func distributedEvidenceSteps(route string, runErr error) []string {
	items := []string{"message.received", "routing.decision"}
	switch strings.ToUpper(strings.TrimSpace(route)) {
	case "CHAT":
		items = append(items, "mio.chat")
	case "OPS":
		items = append(items, "shiro.execute")
	case "CODE", "CODE1", "CODE2", "CODE3", "CODE4":
		items = append(items, "shiro.delegate", "coder.execute", "shiro.verify")
	default:
		items = append(items, "agent.execute")
	}
	if runErr != nil {
		items = append(items, "error")
	} else {
		items = append(items, "done")
	}
	return items
}

func distributedEvidenceErrorKind(runErr error) string {
	if runErr == nil {
		return ""
	}
	lower := strings.ToLower(runErr.Error())
	switch {
	case strings.Contains(lower, "verify"):
		return "verify"
	case strings.Contains(lower, "repair"), strings.Contains(lower, "retry"):
		return "repair"
	case strings.Contains(lower, "patch"), strings.Contains(lower, "command"), strings.Contains(lower, "timeout"), strings.Contains(lower, "error"):
		return "apply"
	default:
		return "other"
	}
}

func (o *DistributedOrchestrator) loadOrCreateSession(ctx context.Context, id, channel, chatID string) (*session.Session, error) {
	sess, err := o.sessionRepo.Load(ctx, id)
	if err != nil {
		return session.NewSession(id, channel, chatID), nil
	}
	return sess, nil
}

// executeDistributed はルートに応じてTransport経由でAgent間通信
func (o *DistributedOrchestrator) executeDistributed(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
	if route != routing.RouteCHAT {
		return o.executeAutonomousDistributed(ctx, t, route, sessionID, ttsSessionID)
	}
	return o.executeDistributedDirect(ctx, t, route, sessionID, ttsSessionID)
}

func (o *DistributedOrchestrator) executeAutonomousDistributed(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
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
			log.Printf("[AutonomousExecutor] entry.stage=%s route=%s job=%s", stage, route.String(), t.JobID().String())
			o.emit("entry.stage", t.Channel(), "system", string(stage), route.String(), t.JobID().String(), sessionID, t.Channel(), t.ChatID())
		},
		ReportStore: o.reporter,
		Execute: func(execCtx context.Context, attempt int, failureKind, failureReason string) (autonomousapp.AttemptResult, error) {
			log.Printf("[AutonomousExecutor] execute start route=%s job=%s attempt=%d failure_kind=%q", route.String(), t.JobID().String(), attempt, failureKind)
			execTask := t
			if attempt > 0 {
				execTask = execTask.WithUserMessage(buildExecutorRetryMessage(t.UserMessage(), route, failureKind, failureReason, attempt))
			}
			resp, runErr := o.executeDistributedDirect(execCtx, execTask, route, sessionID, ttsSessionID)
			resultKind := classifyExecutorFailure(runErr)
			log.Printf("[AutonomousExecutor] execute complete route=%s job=%s attempt=%d success=%t failure_kind=%q", route.String(), t.JobID().String(), attempt, runErr == nil, resultKind)
			return autonomousapp.AttemptResult{
				Response:      resp,
				Steps:         routeExecutionSteps(route, runErr == nil),
				FailureKind:   resultKind,
				FailureReason: errorString(runErr),
			}, runErr
		},
		Verify: func(_ context.Context, c domaincontract.Contract, last autonomousapp.AttemptResult) (bool, string, string, error) {
			ok, kind, reason := verifyByContract(route, c, last)
			log.Printf("[AutonomousExecutor] verify route=%s job=%s passed=%t failure_kind=%q reason=%q", route.String(), t.JobID().String(), ok, kind, reason)
			return ok, kind, reason, nil
		},
	})
	if err != nil {
		return result.Response, err
	}
	return result.Response, nil
}

func (o *DistributedOrchestrator) executeDistributedDirect(ctx context.Context, t task.Task, route routing.Route, sessionID, ttsSessionID string) (string, error) {
	jid := t.JobID().String()
	if isCodeRoute(route) {
		resp, err := o.executeCodeViaShiro(ctx, t, route, sessionID, jid)
		if err == nil {
			o.emit("agent.response", "mio", "user", resp, string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.emitNote("mio", "user", "コード作業の報告をまとめて返したよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.pushTTS(ctx, ttsSessionID, route, "agent.response", resp)
		}
		return resp, err
	}
	targetAgent := o.routeToAgent(route)

	if targetAgent == "" {
		// ローカル処理（CHAT など mio が直接処理）
		guardedTask := o.withAttributionGuard(t, "mio", sessionID)
		userMsg := domaintransport.NewMessage("user", "mio", sessionID, jid, t.UserMessage())
		userMsg.Type = domaintransport.MessageTypeTask
		o.memory.RecordMessage(userMsg)

		o.emit("agent.start", "mio", "user", "考え中...", string(route), jid, sessionID, t.Channel(), t.ChatID())
		// ストリーミングコールバック: トークンを agent.thinking イベントとして配信しつつ、文単位でTTSへ送る。
		streamCtx, ttsStream := o.withStreamHooks(ctx, route, jid, sessionID, t.Channel(), t.ChatID(), ttsSessionID)
		resp, err := o.mio.Chat(streamCtx, guardedTask)
		if err == nil {
			respMsg := domaintransport.NewMessage("mio", "user", sessionID, jid, resp)
			respMsg.Type = domaintransport.MessageTypeResult
			o.memory.RecordMessage(respMsg)
			o.emit("agent.response", "mio", "user", resp, string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.emitNote("mio", "user", "会話処理が終わったよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
			ttsStream.Finalize(ctx, resp)
		}
		return resp, err
	}

	guardedTask := o.withAttributionGuard(t, targetAgent, sessionID)
	msg := domaintransport.NewMessage("mio", targetAgent, sessionID, jid, guardedTask.UserMessage())
	msg.Type = domaintransport.MessageTypeTask
	msg.Context = map[string]interface{}{
		"route":   string(route),
		"channel": t.Channel(),
		"chat_id": t.ChatID(),
	}

	o.emit("agent.start", "mio", targetAgent, t.UserMessage(), string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emit("agent.dispatch", "mio", targetAgent, "ルーティング先へ依頼を転送", string(route), jid, sessionID, t.Channel(), t.ChatID())

	// メモリに記録
	o.memory.RecordMessage(msg)

	result, err := o.executeToAgent(ctx, targetAgent, msg)
	if err == nil {
		o.emit("agent.response", targetAgent, "mio", result.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote(targetAgent, "mio",
			fmt.Sprintf("%s の作業が終わりました。", displayAgentName(targetAgent)),
			string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emit("agent.response", "mio", "user", result.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote("mio", "user", fmt.Sprintf("%sの報告をまとめて返したよ。", displayAgentName(targetAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.pushTTS(ctx, ttsSessionID, route, "agent.response", result.Content)
	}
	return result.Content, err
}

func (o *DistributedOrchestrator) withStreamHooks(
	ctx context.Context,
	route routing.Route,
	jid, sessionID, channel, chatID, ttsSessionID string,
) (context.Context, *streamBundle) {
	prev := llm.StreamCallbackFromContext(ctx)
	ttsStream := newTTSStreamForwarder(o.ttsBridge, ttsSessionID, route, "agent.response", "[DistributedOrch] TTS push degraded:")
	vtuberStream := newVTuberStreamForwarder(o.vtuberBridge, ttsSessionID, route, "agent.response", "[DistributedOrch] VTuber push degraded:")
	return llm.ContextWithStreamCallback(ctx, func(token string) {
		if prev != nil {
			prev(token)
		}
		o.emit("agent.thinking", "mio", "user", token, string(route), jid, sessionID, channel, chatID)
		ttsStream.OnToken(ctx, token)
		vtuberStream.OnToken(ctx, token)
	}), &streamBundle{tts: ttsStream, vtuber: vtuberStream}
}

func (o *DistributedOrchestrator) pushTTS(ctx context.Context, sessionID string, route routing.Route, eventType, text string) {
	ttsCtx := buildTTSContext(route, "normal", false)
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	filtered, emotion := buildTTSPayload(eventType, route, text, ttsCtx, voiceProfile)
	pushTTS(ctx, o.ttsBridge, sessionID, filtered, emotion, "[DistributedOrch] TTS push degraded:")
	req, ok := buildVTuberRequest(eventType, route, sessionID, text, ttsCtx, voiceProfile)
	if ok {
		pushVTuber(ctx, o.vtuberBridge, req, "[DistributedOrch] VTuber push degraded:")
	}
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
	// メッセージ送信
	if err := sshTransport.Send(ctx, msg); err != nil {
		return "", fmt.Errorf("failed to send message to %s via SSH: %w", targetAgent, err)
	}

	log.Printf("[DistributedOrch] Sent task to %s via SSH (job=%s)", targetAgent, msg.JobID)

	// 応答待機（同一transport上で受信）
	waitTimeout := o.distributedWaitTimeout(targetAgent, msg)
	timeoutCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()

	result, err := sshTransport.Receive(timeoutCtx)
	if err != nil {
		return "", fmt.Errorf("waiting for SSH response from %s: %w", targetAgent, err)
	}

	// メモリに記録
	o.memory.RecordMessage(result)

	log.Printf("[DistributedOrch] Received SSH response from %s (type=%s)", result.From, result.Type)

	if result.Type == domaintransport.MessageTypeError {
		return "", fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
	}

	return result.Content, nil
}

func (o *DistributedOrchestrator) executeToAgent(ctx context.Context, targetAgent string, msg domaintransport.Message) (domaintransport.Message, error) {
	return o.executeToAgentViaMailbox(ctx, targetAgent, msg, msg.From)
}

func (o *DistributedOrchestrator) executeToAgentViaMailbox(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
	log.Printf("[DistributedOrch] mailbox send target=%s receive_on=%s via=%s job=%s type=%s has_proposal=%t", targetAgent, receiveOnAgent, transportMode(o.sshTransports, targetAgent), msg.JobID, msg.Type, msg.Proposal != nil)
	o.emitProgress("mailbox.sent", msg.From, targetAgent, fmt.Sprintf("via=%s receive_on=%s type=%s", transportMode(o.sshTransports, targetAgent), receiveOnAgent, msg.Type), msg)
	if sshTransport, ok := o.sshTransports[targetAgent]; ok {
		if err := sshTransport.Send(ctx, msg); err != nil {
			o.emitProgress("mailbox.error", targetAgent, receiveOnAgent, err.Error(), msg)
			return domaintransport.Message{}, fmt.Errorf("failed to send message to %s via SSH: %w", targetAgent, err)
		}
		waitTimeout := o.distributedWaitTimeout(targetAgent, msg)
		timeoutCtx, cancel := context.WithTimeout(ctx, waitTimeout)
		defer cancel()
		log.Printf("[DistributedOrch] mailbox wait target=%s via=ssh timeout=%s job=%s", targetAgent, waitTimeout, msg.JobID)
		o.emitProgress("mailbox.waiting", receiveOnAgent, targetAgent, fmt.Sprintf("via=ssh timeout=%s", waitTimeout), msg)
		result, err := sshTransport.Receive(timeoutCtx)
		if err != nil {
			log.Printf("[DistributedOrch] mailbox wait error target=%s via=ssh job=%s err=%v", targetAgent, msg.JobID, err)
			o.emitProgress("mailbox.error", targetAgent, receiveOnAgent, err.Error(), msg)
			return domaintransport.Message{}, fmt.Errorf("waiting for SSH response from %s: %w", targetAgent, err)
		}
		o.memory.RecordMessage(result)
		log.Printf("[DistributedOrch] mailbox recv target=%s via=ssh from=%s type=%s job=%s", targetAgent, result.From, result.Type, result.JobID)
		o.emitProgress("mailbox.received", result.From, receiveOnAgent, fmt.Sprintf("via=ssh type=%s", result.Type), msg)
		if result.Type == domaintransport.MessageTypeError {
			o.emitProgress("agent.error", result.From, receiveOnAgent, result.Content, msg)
			return domaintransport.Message{}, fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
		}
		return result, nil
	}
	return o.executeViaLocal(ctx, targetAgent, msg, receiveOnAgent)
}

// executeViaLocal はMessageRouter経由でローカルAgentと通信
func (o *DistributedOrchestrator) executeViaLocal(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
	agentTransport, ok := o.router.GetAgent(targetAgent)
	if !ok {
		return domaintransport.Message{}, fmt.Errorf("agent '%s' not registered in router", targetAgent)
	}

	// メッセージ送信
	if err := agentTransport.PutInboundMessage(msg); err != nil {
		o.emitProgress("mailbox.error", targetAgent, receiveOnAgent, err.Error(), msg)
		return domaintransport.Message{}, fmt.Errorf("failed to send message to %s: %w", targetAgent, err)
	}

	log.Printf("[DistributedOrch] Sent task to %s via Local (job=%s type=%s receive_on=%s)", targetAgent, msg.JobID, msg.Type, receiveOnAgent)
	o.emitProgress("mailbox.waiting", receiveOnAgent, targetAgent, fmt.Sprintf("via=local timeout=%s", o.distributedWaitTimeout(targetAgent, msg)), msg)

	// 応答待機（指定agent経由。未登録ならmioにフォールバック）
	receiveTransport, ok := o.router.GetAgent(receiveOnAgent)
	if !ok {
		receiveTransport, ok = o.router.GetAgent("mio")
	}
	if !ok {
		o.emitProgress("mailbox.error", targetAgent, receiveOnAgent, "receive transport not registered", msg)
		return domaintransport.Message{}, fmt.Errorf("receive transport not registered (agent=%s)", receiveOnAgent)
	}

	waitTimeout := o.distributedWaitTimeout(targetAgent, msg)
	timeoutCtx, cancel := context.WithTimeout(ctx, waitTimeout)
	defer cancel()
	log.Printf("[DistributedOrch] wait local response target=%s receive_on=%s timeout=%s job=%s", targetAgent, receiveOnAgent, waitTimeout, msg.JobID)

	result, err := receiveTransport.Receive(timeoutCtx)
	if err != nil {
		log.Printf("[DistributedOrch] wait local response error target=%s receive_on=%s job=%s err=%v", targetAgent, receiveOnAgent, msg.JobID, err)
		o.emitProgress("mailbox.error", targetAgent, receiveOnAgent, err.Error(), msg)
		return domaintransport.Message{}, fmt.Errorf("waiting for response from %s: %w", targetAgent, err)
	}

	// メモリに記録
	o.memory.RecordMessage(result)

	log.Printf("[DistributedOrch] Received response from %s (type=%s job=%s to=%s)", result.From, result.Type, result.JobID, result.To)
	o.emitProgress("mailbox.received", result.From, receiveOnAgent, fmt.Sprintf("via=local type=%s", result.Type), msg)

	if result.Type == domaintransport.MessageTypeError {
		o.emitProgress("agent.error", result.From, receiveOnAgent, result.Content, msg)
		return domaintransport.Message{}, fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
	}

	return result, nil
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
