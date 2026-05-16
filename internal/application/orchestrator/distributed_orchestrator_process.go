package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

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
