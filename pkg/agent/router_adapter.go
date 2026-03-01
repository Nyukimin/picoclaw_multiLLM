package agent

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/worker"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/session"
)

// RouterAdapter adapts the agent.Router to the worker.Router interface.
// This breaks the circular dependency between pkg/agent and pkg/modules/worker.
type RouterAdapter struct {
	router *Router
}

// NewRouterAdapter creates a new router adapter.
func NewRouterAdapter(router *Router) *RouterAdapter {
	return &RouterAdapter{router: router}
}

// Decide implements the worker.Router interface.
func (ra *RouterAdapter) Decide(ctx context.Context, userText string, flags session.SessionFlags) worker.RouterDecision {
	decision := ra.router.Decide(ctx, userText, flags)

	return worker.RouterDecision{
		Route:                decision.Route,
		Source:               decision.Source,
		Confidence:           decision.Confidence,
		Reason:               decision.Reason,
		Evidence:             decision.Evidence,
		LocalOnly:            decision.LocalOnly,
		PrevRoute:            decision.PrevRoute,
		CleanUserText:        decision.CleanUserText,
		Declaration:          decision.Declaration,
		DirectResponse:       decision.DirectResponse,
		ErrorReason:          decision.ErrorReason,
		ClassifierConfidence: decision.ClassifierConfidence,
	}
}
