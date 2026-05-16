package orchestrator

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type routeDecisionCoordinator struct {
	mio  MioAgent
	emit messageEventEmitter
}

func newRouteDecisionCoordinator(mio MioAgent, emit messageEventEmitter) *routeDecisionCoordinator {
	return &routeDecisionCoordinator{
		mio:  mio,
		emit: emit,
	}
}

func (c *routeDecisionCoordinator) Decide(ctx context.Context, t task.Task, req ProcessMessageRequest, jobID task.JobID) (routing.Decision, error) {
	decision, err := c.mio.DecideAction(ctx, t)
	if err != nil {
		return routing.Decision{}, fmt.Errorf("routing decision failed: %w", err)
	}
	c.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)
	return decision, nil
}
