package agent

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/chat"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/order"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/worker"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/session"
)

// NewAgentWithModules creates an agent core with modules attached based on agent ID.
// This factory is used by the new architecture to instantiate agents.
func NewAgentWithModules(
	ctx context.Context,
	id string,
	cfg *config.Config,
	msgBus *bus.MessageBus,
	sessions *session.SessionManager,
	router *Router,
) (*modules.AgentCore, error) {
	// Resolve alias, provider, and model for this agent
	alias := resolveAlias(id, cfg)
	provider, providerName := resolveProvider(id, cfg)
	model := resolveModel(id, cfg)

	logger.InfoCF("factory", "agent.create", map[string]interface{}{
		"agent_id": id,
		"alias":    alias,
		"provider": providerName,
		"model":    model,
	})

	// Create core
	core := modules.NewAgentCore(id, alias, provider, model, cfg, msgBus, sessions)

	// Attach modules based on agent ID
	var modulesToAttach []modules.Module

	switch id {
	case "chat":
		modulesToAttach = []modules.Module{
			chat.NewLightweightReceptionModule(),
			chat.NewFinalDecisionModule(),
			// ApprovalUIModule will be added when UI is implemented
		}

	case "worker":
		// Use RouterAdapter to avoid circular dependency
		routerAdapter := NewRouterAdapter(router)
		modulesToAttach = []modules.Module{
			worker.NewRoutingModule(routerAdapter, cfg.Routing),
			worker.NewExecutionModule(),
			worker.NewAggregationModule(),
		}

		// Add heartbeat collector if enabled
		if cfg.Architecture.EnableHeartbeat {
			modulesToAttach = append(modulesToAttach, worker.NewHeartbeatCollectorModule())
		}

	case "order1", "order2", "order3":
		modulesToAttach = []modules.Module{
			order.NewProposalGenerationModule(),
			order.NewCodeAnalysisModule(),
		}

		// Order3 (Gin/Claude) has approval flow
		if id == "order3" {
			modulesToAttach = append(modulesToAttach, order.NewApprovalFlowModule())
		}

	default:
		return nil, fmt.Errorf("unknown agent ID: %s", id)
	}

	// Initialize all modules
	for _, module := range modulesToAttach {
		if err := core.AttachModule(ctx, module); err != nil {
			return nil, fmt.Errorf("failed to attach module %s to agent %s: %w", module.Name(), id, err)
		}

		logger.InfoCF("factory", "module.attached", map[string]interface{}{
			"agent_id":    id,
			"module_name": module.Name(),
		})
	}

	return core, nil
}

// resolveAlias returns the friendly name for an agent ID.
func resolveAlias(id string, cfg *config.Config) string {
	switch id {
	case "chat":
		if cfg.Routing.LLM.ChatAlias != "" {
			return cfg.Routing.LLM.ChatAlias
		}
		return "Mio"
	case "worker":
		if cfg.Routing.LLM.WorkerAlias != "" {
			return cfg.Routing.LLM.WorkerAlias
		}
		return "Shiro"
	case "order1":
		if cfg.Routing.LLM.CoderAlias != "" {
			return cfg.Routing.LLM.CoderAlias
		}
		return "Aka"
	case "order2":
		if cfg.Routing.LLM.Coder2Alias != "" {
			return cfg.Routing.LLM.Coder2Alias
		}
		return "Ao"
	case "order3":
		if cfg.Routing.LLM.Coder3Alias != "" {
			return cfg.Routing.LLM.Coder3Alias
		}
		return "Gin"
	default:
		return id
	}
}

// resolveProvider returns the LLM provider for an agent ID.
func resolveProvider(id string, cfg *config.Config) (providers.LLMProvider, string) {
	// TODO: Create actual providers based on configuration
	// For now, return nil - will be implemented in integration phase
	switch id {
	case "chat":
		return nil, cfg.Routing.LLM.ChatProvider
	case "worker":
		return nil, cfg.Routing.LLM.WorkerProvider
	case "order1":
		return nil, cfg.Routing.LLM.CoderProvider
	case "order2":
		return nil, cfg.Routing.LLM.Coder2Provider
	case "order3":
		return nil, cfg.Routing.LLM.Coder3Provider
	default:
		return nil, ""
	}
}

// resolveModel returns the model name for an agent ID.
func resolveModel(id string, cfg *config.Config) string {
	switch id {
	case "chat":
		return cfg.Routing.LLM.ChatModel
	case "worker":
		return cfg.Routing.LLM.WorkerModel
	case "order1":
		return cfg.Routing.LLM.CoderModel
	case "order2":
		return cfg.Routing.LLM.Coder2Model
	case "order3":
		return cfg.Routing.LLM.Coder3Model
	default:
		return ""
	}
}
