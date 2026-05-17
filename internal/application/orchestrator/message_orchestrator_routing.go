package orchestrator

import (
	"context"
	"fmt"
	"strings"

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
		fmt.Sprintf("confidence %.0f%% evidence=%s", decision.Confidence*100, routeDecisionEvidenceSummary(decision.Evidence)),
		string(decision.Route), jobID.String(), req.SessionID, req.Channel, req.ChatID)
	return decision, nil
}

func routeDecisionEvidenceSummary(evidence []routing.DecisionEvidence) string {
	if len(evidence) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(evidence))
	for _, ev := range evidence {
		state := "miss"
		if ev.Matched {
			state = "matched"
		}
		route := string(ev.Route)
		if route == "" {
			route = "-"
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%s", ev.Source, state, route))
	}
	return strings.Join(parts, ",")
}
