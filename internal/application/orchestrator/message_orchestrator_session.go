package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func (o *MessageOrchestrator) loadSessionForRequest(ctx context.Context, req ProcessMessageRequest) (*session.Session, error) {
	sess, err := o.loadOrCreateSession(ctx, req.SessionID, req.Channel, req.ChatID)
	if err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to load or create session: %v", err)
		return nil, fmt.Errorf("failed to load or create session: %w", err)
	}
	log.Printf("[MessageOrch] Session loaded/created: %s", sess.ID())
	return sess, nil
}

func (o *MessageOrchestrator) saveCompletedTask(ctx context.Context, sess *session.Session, t task.Task) error {
	sess.AddTask(t)
	if err := o.sessionRepo.Save(ctx, sess); err != nil {
		log.Printf("[MessageOrch] ProcessMessage ERROR: failed to save session: %v", err)
		return fmt.Errorf("failed to save session: %w", err)
	}
	return nil
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
