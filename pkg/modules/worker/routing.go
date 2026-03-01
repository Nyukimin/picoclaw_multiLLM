// Package worker provides modules for the Worker agent (Shiro).
package worker

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/session"
)

// RoutingInput contains the information needed for routing decisions.
type RoutingInput struct {
	JobID     string
	UserText  string
	Flags     session.SessionFlags
}

// RoutingDecision contains the routing decision with JobID tracking.
// This mirrors agent.RoutingDecision but avoids circular dependency.
type RoutingDecision struct {
	Route                string
	Source               string
	Confidence           float64
	Reason               string
	Evidence             []string
	LocalOnly            bool
	PrevRoute            string
	CleanUserText        string
	Declaration          string
	DirectResponse       string
	ErrorReason          string
	ClassifierConfidence float64
	JobID                string
}

// Router interface defines the routing decision method.
// This avoids circular dependency with pkg/agent.
type Router interface {
	Decide(ctx context.Context, userText string, flags session.SessionFlags) RouterDecision
}

// RouterDecision represents the base routing decision from the router.
type RouterDecision struct {
	Route                string
	Source               string
	Confidence           float64
	Reason               string
	Evidence             []string
	LocalOnly            bool
	PrevRoute            string
	CleanUserText        string
	Declaration          string
	DirectResponse       string
	ErrorReason          string
	ClassifierConfidence float64
}

// RoutingModule wraps the existing Router and adds JobID tracking.
// This module is responsible for deciding which agent (Worker/Order1/Order2/Order3)
// should handle the task.
type RoutingModule struct {
	agent  *modules.AgentCore
	router Router
	cfg    config.RoutingConfig
}

// NewRoutingModule creates a new routing module that wraps the existing Router.
func NewRoutingModule(router Router, cfg config.RoutingConfig) *RoutingModule {
	return &RoutingModule{
		router: router,
		cfg:    cfg,
	}
}

// Name returns the module name.
func (m *RoutingModule) Name() string {
	return "Routing"
}

// Initialize initializes the module with the agent core.
func (m *RoutingModule) Initialize(ctx context.Context, agentCore *modules.AgentCore) error {
	m.agent = agentCore
	return nil
}

// Shutdown cleans up module resources.
func (m *RoutingModule) Shutdown(ctx context.Context) error {
	return nil
}

// Decide makes a routing decision using the existing Router.
// It adds JobID tracking and enhanced logging for the new architecture.
func (m *RoutingModule) Decide(ctx context.Context, input RoutingInput) (RoutingDecision, error) {
	// Delegate to existing Router
	baseDecision := m.router.Decide(ctx, input.UserText, input.Flags)

	// Convert to RoutingDecision with JobID
	decision := RoutingDecision{
		Route:                baseDecision.Route,
		Source:               baseDecision.Source,
		Confidence:           baseDecision.Confidence,
		Reason:               baseDecision.Reason,
		Evidence:             baseDecision.Evidence,
		LocalOnly:            baseDecision.LocalOnly,
		PrevRoute:            baseDecision.PrevRoute,
		CleanUserText:        baseDecision.CleanUserText,
		Declaration:          baseDecision.Declaration,
		DirectResponse:       baseDecision.DirectResponse,
		ErrorReason:          baseDecision.ErrorReason,
		ClassifierConfidence: baseDecision.ClassifierConfidence,
		JobID:                input.JobID,
	}

	// Log with new architecture format
	logger.InfoCF("routing", "router.decision", map[string]interface{}{
		"job_id":     decision.JobID,
		"route":      decision.Route,
		"source":     decision.Source,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"local_only": decision.LocalOnly,
	})

	return decision, nil
}
