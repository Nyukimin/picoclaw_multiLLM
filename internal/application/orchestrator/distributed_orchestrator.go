package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

const (
	distributedTimeout = 120 * time.Second
)

// DistributedOrchestrator はTransport経由でメッセージを送受信する分散オーケストレータ
type DistributedOrchestrator struct {
	sessionRepo   SessionRepository
	mio           MioAgent
	router        *transport.MessageRouter
	memory        *session.CentralMemory
	sshTransports map[string]domaintransport.Transport // SSH経由のリモートAgent
	listener      EventListener
	idleNotifier  IdleNotifier
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
	}
}

// SetEventListener sets an optional listener for monitoring events.
func (o *DistributedOrchestrator) SetEventListener(l EventListener) {
	o.listener = l
}

// SetIdleNotifier sets an optional notifier used to control idle chat.
func (o *DistributedOrchestrator) SetIdleNotifier(n IdleNotifier) {
	o.idleNotifier = n
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

// ProcessMessage は既存MessageOrchestratorと同じシグネチャでメッセージを処理
// 分散環境ではTransport経由でAgent間通信を行う
func (o *DistributedOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	log.Printf("[DistributedOrch] ProcessMessage START: sessionID=%s channel=%s chatID=%s message=%q",
		req.SessionID, req.Channel, req.ChatID, req.UserMessage)

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
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}

	o.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)
	o.emitNote("mio", "user",
		fmt.Sprintf("%s", routeNoticeText(decision.Route, req.UserMessage)),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)

	t = t.WithRoute(decision.Route)

	workerMarkedBusy := false
	if o.idleNotifier != nil && decision.Route != routing.RouteCHAT {
		o.idleNotifier.SetWorkerBusy(true)
		workerMarkedBusy = true
	}
	if workerMarkedBusy {
		defer o.idleNotifier.SetWorkerBusy(false)
	}

	// 4. ルートに応じてTransport経由で実行
	response, err := o.executeDistributed(ctx, t, decision.Route, sess.ID())
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("distributed execution failed: %w", err)
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

	return ProcessMessageResponse{
		Response:   response,
		Route:      decision.Route,
		Confidence: decision.Confidence,
		JobID:      jobID.String(),
	}, nil
}

func (o *DistributedOrchestrator) loadOrCreateSession(ctx context.Context, id, channel, chatID string) (*session.Session, error) {
	sess, err := o.sessionRepo.Load(ctx, id)
	if err != nil {
		return session.NewSession(id, channel, chatID), nil
	}
	return sess, nil
}

// executeDistributed はルートに応じてTransport経由でAgent間通信
func (o *DistributedOrchestrator) executeDistributed(ctx context.Context, t task.Task, route routing.Route, sessionID string) (string, error) {
	jid := t.JobID().String()
	if isCodeRoute(route) {
		return o.executeCodeViaShiro(ctx, t, route, sessionID, jid)
	}
	targetAgent := o.routeToAgent(route)

	if targetAgent == "" {
		// ローカル処理（CHAT など mio が直接処理）
		guardedTask := o.withAttributionGuard(t, "mio", sessionID)
		userMsg := domaintransport.NewMessage("user", "mio", sessionID, jid, t.UserMessage())
		userMsg.Type = domaintransport.MessageTypeTask
		o.memory.RecordMessage(userMsg)

		o.emit("agent.start", "mio", "user", "考え中...", string(route), jid, sessionID, t.Channel(), t.ChatID())
		// ストリーミングコールバック: トークンを agent.thinking イベントとして配信
		streamCtx := llm.ContextWithStreamCallback(ctx, func(token string) {
			o.emit("agent.thinking", "mio", "user", token, string(route), jid, sessionID, t.Channel(), t.ChatID())
		})
		resp, err := o.mio.Chat(streamCtx, guardedTask)
		if err == nil {
			respMsg := domaintransport.NewMessage("mio", "user", sessionID, jid, resp)
			respMsg.Type = domaintransport.MessageTypeResult
			o.memory.RecordMessage(respMsg)
			o.emit("agent.response", "mio", "user", resp, string(route), jid, sessionID, t.Channel(), t.ChatID())
			o.emitNote("mio", "user", "会話処理が終わったよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
		}
		return resp, err
	}

	guardedTask := o.withAttributionGuard(t, targetAgent, sessionID)
	msg := domaintransport.NewMessage("mio", targetAgent, sessionID, jid, guardedTask.UserMessage())
	msg.Type = domaintransport.MessageTypeTask

	o.emit("agent.start", "mio", targetAgent, t.UserMessage(), string(route), jid, sessionID, t.Channel(), t.ChatID())

	// メモリに記録
	o.memory.RecordMessage(msg)

	result, err := o.executeToAgent(ctx, targetAgent, msg)
	if err == nil {
		o.emit("agent.response", targetAgent, "mio", result.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
		o.emitNote(targetAgent, "mio",
			fmt.Sprintf("%s の作業が終わりました。", displayAgentName(targetAgent)),
			string(route), jid, sessionID, t.Channel(), t.ChatID())
	}
	return result.Content, err
}

func (o *DistributedOrchestrator) executeCodeViaShiro(
	ctx context.Context,
	t task.Task,
	route routing.Route,
	sessionID, jid string,
) (string, error) {
	coderAgent := o.routeToCoder(route)
	if coderAgent == "" {
		return "", fmt.Errorf("no coder mapped for route %s", route)
	}

	o.emit("agent.start", "mio", "shiro", "コードタスクをShiro経由で実行", string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emitNote("mio", "user", "しろにコード実装の取りまとめをお願いしたよ。", string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emit("agent.start", "shiro", coderAgent, t.UserMessage(), string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emitNote("shiro", "mio", fmt.Sprintf("%sにコーディング依頼しました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())

	coderMsg := domaintransport.NewMessage("shiro", coderAgent, sessionID, jid, t.UserMessage())
	coderMsg.Type = domaintransport.MessageTypeTask
	coderMsg.Context = map[string]interface{}{"route": string(route)}
	o.memory.RecordMessage(coderMsg)

	coderResult, err := o.executeToAgent(ctx, coderAgent, coderMsg)
	if err != nil {
		return "", err
	}
	o.emit("agent.response", coderAgent, "shiro", coderResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emitNote(coderAgent, "shiro", "おわったっす。", string(route), jid, sessionID, t.Channel(), t.ChatID())

	if coderResult.Proposal == nil {
		// Proposalが返らない場合もShiroを必ず経由して最終応答を返す。
		o.emit("agent.start", "shiro", "mio", "Coder結果をShiroで整形", string(route), jid, sessionID, t.Channel(), t.ChatID())
		shiroTask := domaintransport.NewMessage("mio", "shiro", sessionID, jid, coderResult.Content)
		shiroTask.Type = domaintransport.MessageTypeTask
		shiroTask.Context = map[string]interface{}{"route": string(route), "coder_agent": coderAgent}
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
	execMsg.Context = map[string]interface{}{"route": string(route), "coder_agent": coderAgent}
	execMsg.Proposal = coderResult.Proposal
	o.memory.RecordMessage(execMsg)

	shiroResult, err := o.executeToAgent(ctx, "shiro", execMsg)
	if err != nil {
		return "", err
	}
	o.emit("agent.response", "shiro", "mio", shiroResult.Content, string(route), jid, sessionID, t.Channel(), t.ChatID())
	o.emitNote("shiro", "mio", fmt.Sprintf("%sの作業が終わりました。", displayAgentName(coderAgent)), string(route), jid, sessionID, t.Channel(), t.ChatID())
	return shiroResult.Content, nil
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
	timeoutCtx, cancel := context.WithTimeout(ctx, distributedTimeout)
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
	if sshTransport, ok := o.sshTransports[targetAgent]; ok {
		if err := sshTransport.Send(ctx, msg); err != nil {
			return domaintransport.Message{}, fmt.Errorf("failed to send message to %s via SSH: %w", targetAgent, err)
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, distributedTimeout)
		defer cancel()
		result, err := sshTransport.Receive(timeoutCtx)
		if err != nil {
			return domaintransport.Message{}, fmt.Errorf("waiting for SSH response from %s: %w", targetAgent, err)
		}
		o.memory.RecordMessage(result)
		if result.Type == domaintransport.MessageTypeError {
			return domaintransport.Message{}, fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
		}
		return result, nil
	}
	return o.executeViaLocal(ctx, targetAgent, msg, msg.From)
}

// executeViaLocal はMessageRouter経由でローカルAgentと通信
func (o *DistributedOrchestrator) executeViaLocal(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
	agentTransport, ok := o.router.GetAgent(targetAgent)
	if !ok {
		return domaintransport.Message{}, fmt.Errorf("agent '%s' not registered in router", targetAgent)
	}

	// メッセージ送信
	if err := agentTransport.PutInboundMessage(msg); err != nil {
		return domaintransport.Message{}, fmt.Errorf("failed to send message to %s: %w", targetAgent, err)
	}

	log.Printf("[DistributedOrch] Sent task to %s via Local (job=%s)", targetAgent, msg.JobID)

	// 応答待機（指定agent経由。未登録ならmioにフォールバック）
	receiveTransport, ok := o.router.GetAgent(receiveOnAgent)
	if !ok {
		receiveTransport, ok = o.router.GetAgent("mio")
	}
	if !ok {
		return domaintransport.Message{}, fmt.Errorf("receive transport not registered (agent=%s)", receiveOnAgent)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, distributedTimeout)
	defer cancel()

	result, err := receiveTransport.Receive(timeoutCtx)
	if err != nil {
		return domaintransport.Message{}, fmt.Errorf("waiting for response from %s: %w", targetAgent, err)
	}

	// メモリに記録
	o.memory.RecordMessage(result)

	log.Printf("[DistributedOrch] Received response from %s (type=%s)", result.From, result.Type)

	if result.Type == domaintransport.MessageTypeError {
		return domaintransport.Message{}, fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
	}

	return result, nil
}

// routeToAgent はルートをAgent名にマッピング
func (o *DistributedOrchestrator) routeToAgent(route routing.Route) string {
	switch route {
	case routing.RouteOPS:
		return "shiro"
	case routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return "shiro"
	case routing.RouteCHAT, routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return "" // mio がローカル処理
	default:
		return ""
	}
}

func (o *DistributedOrchestrator) routeToCoder(route routing.Route) string {
	switch route {
	case routing.RouteCODE, routing.RouteCODE1:
		return "coder1"
	case routing.RouteCODE2:
		return "coder2"
	case routing.RouteCODE3:
		return "coder3"
	default:
		return ""
	}
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
