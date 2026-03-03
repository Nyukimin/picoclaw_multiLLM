package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

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
	sessionRepo SessionRepository
	mio         MioAgent
	router      *transport.MessageRouter
	memory      *session.CentralMemory
}

// NewDistributedOrchestrator は新しいDistributedOrchestratorを作成
func NewDistributedOrchestrator(
	sessionRepo SessionRepository,
	mio MioAgent,
	router *transport.MessageRouter,
	memory *session.CentralMemory,
) *DistributedOrchestrator {
	return &DistributedOrchestrator{
		sessionRepo: sessionRepo,
		mio:         mio,
		router:      router,
		memory:      memory,
	}
}

// ProcessMessage は既存MessageOrchestratorと同じシグネチャでメッセージを処理
// 分散環境ではTransport経由でAgent間通信を行う
func (o *DistributedOrchestrator) ProcessMessage(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, error) {
	// 1. セッションをロードまたは作成
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to load or create session: %w", err)
	}

	// 2. タスクを作成
	jobID := task.NewJobID()
	t := task.NewTask(jobID, req.UserMessage, req.Channel, req.ChatID)

	// 3. Mioがルーティング決定
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("routing decision failed: %w", err)
	}

	t = t.WithRoute(decision.Route)

	// 4. ルートに応じてTransport経由で実行
	response, err := o.executeDistributed(ctx, t, decision.Route, sess.ID())
	if err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("distributed execution failed: %w", err)
	}

	// 5. タスクを履歴に追加
	sess.AddTask(t)

	// 6. セッションを保存
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		return ProcessMessageResponse{}, fmt.Errorf("failed to save session: %w", err)
	}

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
	targetAgent := o.routeToAgent(route)

	if targetAgent == "" {
		// ローカル処理（CHATなどMioが直接処理）
		return o.mio.Chat(ctx, t)
	}

	// Transport経由でメッセージ送信
	agentTransport, ok := o.router.GetAgent(targetAgent)
	if !ok {
		return "", fmt.Errorf("agent '%s' not registered in router", targetAgent)
	}

	msg := domaintransport.NewMessage("Mio", targetAgent, sessionID, t.JobID().String(), t.UserMessage())
	msg.Type = domaintransport.MessageTypeTask

	// メモリに記録
	o.memory.RecordMessage(msg)

	// メッセージ送信
	if err := agentTransport.PutInboundMessage(msg); err != nil {
		return "", fmt.Errorf("failed to send message to %s: %w", targetAgent, err)
	}

	log.Printf("[DistributedOrch] Sent task to %s (job=%s)", targetAgent, t.JobID().String())

	// 応答待機（Mioのトランスポート経由）
	mioTransport, ok := o.router.GetAgent("Mio")
	if !ok {
		return "", fmt.Errorf("Mio transport not registered")
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, distributedTimeout)
	defer cancel()

	result, err := mioTransport.Receive(timeoutCtx)
	if err != nil {
		return "", fmt.Errorf("waiting for response from %s: %w", targetAgent, err)
	}

	// メモリに記録
	o.memory.RecordMessage(result)

	log.Printf("[DistributedOrch] Received response from %s (type=%s)", result.From, result.Type)

	if result.Type == domaintransport.MessageTypeError {
		return "", fmt.Errorf("agent %s returned error: %s", result.From, result.Content)
	}

	return result.Content, nil
}

// routeToAgent はルートをAgent名にマッピング
func (o *DistributedOrchestrator) routeToAgent(route routing.Route) string {
	switch route {
	case routing.RouteOPS:
		return "Shiro"
	case routing.RouteCODE, routing.RouteCODE1:
		return "Coder1"
	case routing.RouteCODE2:
		return "Coder2"
	case routing.RouteCODE3:
		return "Coder3"
	case routing.RouteCHAT, routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return "" // Mioがローカル処理
	default:
		return ""
	}
}
