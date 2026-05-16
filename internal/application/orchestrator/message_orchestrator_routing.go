package orchestrator

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func (o *MessageOrchestrator) decideRouteForTask(ctx context.Context, t task.Task, req ProcessMessageRequest, jobID task.JobID) (routing.Decision, error) {
	decision, err := o.mio.DecideAction(ctx, t)
	if err != nil {
		return routing.Decision{}, fmt.Errorf("routing decision failed: %w", err)
	}
	o.emit("routing.decision", "mio", "",
		fmt.Sprintf("confidence %.0f%%", decision.Confidence*100),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)
	return decision, nil
}
