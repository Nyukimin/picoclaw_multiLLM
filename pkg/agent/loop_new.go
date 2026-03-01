package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/jobid"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/chat"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/order"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules/worker"
)

// processMessageNewArch implements the new architecture message flow.
// Flow: Chat (reception) → Worker (routing) → Order (if needed) → Worker (aggregation) → Chat (decision)
func (al *AgentLoop) processMessageNewArch(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Generate JobID for this work unit
	jobID := al.jobIDGen.Next()

	logger.InfoCF("agent", "new_arch.start", map[string]interface{}{
		"job_id":      jobID,
		"channel":     msg.Channel,
		"session_key": msg.SessionKey,
	})

	// Step 1: Chat agent receives task (lightweight reception)
	task := al.chatReceptionModule.ReceiveTask(msg, jobID)

	logger.InfoCF("agent", "new_arch.task_received", map[string]interface{}{
		"job_id":    jobID,
		"user_text": task.UserText,
	})

	// Step 2: Worker agent makes routing decision
	flags := al.sessions.GetFlags(msg.SessionKey)
	routingInput := worker.RoutingInput{
		JobID:    jobID,
		UserText: task.UserText,
		Flags:    flags,
	}

	decision, err := al.workerRoutingModule.Decide(ctx, routingInput)
	if err != nil {
		return "", fmt.Errorf("routing decision failed: %w", err)
	}

	// Update session flags with routing decision
	flags.LocalOnly = decision.LocalOnly
	al.sessions.SetFlags(msg.SessionKey, flags)

	// Step 3: Notify user if delegating to Order agent
	if isOrderRoute(decision.Route) && msg.Channel != "system" {
		role, alias := al.resolveRouteRoleAlias(decision.Route)
		display := role
		if alias != "" && !strings.EqualFold(alias, role) {
			display = fmt.Sprintf("%s（%s）", role, alias)
		}
		chatAlias := al.cfg.Routing.LLM.ChatAlias
		if chatAlias == "" {
			chatAlias = "Mio"
		}

		al.bus.PublishOutbound(bus.OutboundMessage{
			Channel: msg.Channel,
			ChatID:  msg.ChatID,
			Content: fmt.Sprintf("%sから%sに作業依頼して進めるね。完了したら報告するよ。", chatAlias, display),
		})
	}

	// Step 4: Execute based on routing decision
	var result chat.TaskResult

	if isOrderRoute(decision.Route) {
		// Delegate to Order agent
		orderResult, err := al.delegateToOrderNewArch(ctx, decision.Route, task)
		if err != nil {
			result = chat.TaskResult{
				JobID:   jobID,
				Success: false,
				Error:   err,
			}
		} else {
			// Worker aggregates Order result
			aggregated, err := al.workerAggregationModule.Aggregate(ctx, []worker.OrderResult{orderResult})
			if err != nil {
				result = chat.TaskResult{
					JobID:   jobID,
					Success: false,
					Error:   err,
				}
			} else {
				result = chat.TaskResult{
					JobID:    jobID,
					Success:  aggregated.Success,
					Output:   aggregated.Output,
					Error:    aggregated.Error,
					Metadata: aggregated.Metadata,
				}
			}
		}
	} else {
		// Worker executes directly
		output, err := al.workerExecutionModule.ExecuteTask(ctx, jobID, decision.Route, task.UserText)
		if err != nil {
			result = chat.TaskResult{
				JobID:   jobID,
				Success: false,
				Error:   err,
			}
		} else {
			result = chat.TaskResult{
				JobID:   jobID,
				Success: true,
				Output:  output,
			}
		}
	}

	// Step 5: Chat agent makes final decision
	response := al.chatDecisionModule.MakeFinalDecision(ctx, result)

	logger.InfoCF("agent", "new_arch.complete", map[string]interface{}{
		"job_id":  jobID,
		"success": result.Success,
	})

	return response, nil
}

// delegateToOrderNewArch delegates a task to an Order agent in the new architecture.
func (al *AgentLoop) delegateToOrderNewArch(ctx context.Context, route string, task chat.Task) (worker.OrderResult, error) {
	orderID := routeToOrderID(route)

	logger.InfoCF("agent", "new_arch.delegate_order", map[string]interface{}{
		"job_id":   task.JobID,
		"route":    route,
		"order_id": orderID,
	})

	// Create Order agent with modules
	orderAgent, err := NewAgentWithModules(ctx, orderID, al.cfg, al.bus, al.sessions, al.router)
	if err != nil {
		return worker.OrderResult{
			JobID:   task.JobID,
			OrderID: orderID,
			Success: false,
			Error:   fmt.Errorf("failed to create Order agent: %w", err),
		}, nil
	}

	// Find ProposalGenerationModule
	var proposalModule *order.ProposalGenerationModule
	for _, module := range orderAgent.Modules {
		if pm, ok := module.(*order.ProposalGenerationModule); ok {
			proposalModule = pm
			break
		}
	}

	if proposalModule == nil {
		return worker.OrderResult{
			JobID:   task.JobID,
			OrderID: orderID,
			Success: false,
			Error:   fmt.Errorf("ProposalGenerationModule not found in Order agent"),
		}, nil
	}

	// Generate proposal
	proposal, err := proposalModule.GenerateProposal(ctx, order.ProposalRequest{
		JobID:    task.JobID,
		Route:    route,
		UserText: task.UserText,
		Context:  task.Metadata,
	})

	if err != nil {
		return worker.OrderResult{
			JobID:   task.JobID,
			OrderID: orderID,
			Success: false,
			Error:   fmt.Errorf("proposal generation failed: %w", err),
		}, nil
	}

	// Convert proposal to OrderResult
	output := fmt.Sprintf("【%sからの提案】\n\n## 計画\n%s\n\n## 実装\n%s\n\n## リスク\n%s",
		orderAgent.Alias, proposal.Plan, proposal.Patch, proposal.Risk)

	return worker.OrderResult{
		JobID:    task.JobID,
		OrderID:  orderID,
		Success:  true,
		Output:   output,
		Plan:     proposal.Plan,
		Patch:    proposal.Patch,
		Risk:     proposal.Risk,
		Metadata: proposal.Metadata,
	}, nil
}

// isOrderRoute returns true if the route requires an Order agent.
func isOrderRoute(route string) bool {
	r := strings.ToUpper(strings.TrimSpace(route))
	return r == RouteCode || r == RouteCode1 || r == RouteCode2 || r == RouteCode3
}

// routeToOrderID converts a route to an Order agent ID.
func routeToOrderID(route string) string {
	r := strings.ToUpper(strings.TrimSpace(route))
	switch r {
	case RouteCode1:
		return "order1"
	case RouteCode2:
		return "order2"
	case RouteCode3:
		return "order3"
	case RouteCode:
		// Default CODE route uses order1
		return "order1"
	default:
		return "order1"
	}
}

// initializeNewArchitecture sets up the new architecture components.
// This is called during AgentLoop initialization when UseNewArchitecture is true.
func (al *AgentLoop) initializeNewArchitecture(ctx context.Context) error {
	logger.InfoCF("agent", "new_arch.initialize", map[string]interface{}{
		"enable_heartbeat":    al.cfg.Architecture.EnableHeartbeat,
		"enable_deliberation": al.cfg.Architecture.EnableDeliberation,
	})

	// Initialize JobID generator
	al.jobIDGen = jobid.NewGenerator()

	// Create Chat agent with modules
	chatAgent, err := NewAgentWithModules(ctx, "chat", al.cfg, al.bus, al.sessions, al.router)
	if err != nil {
		return fmt.Errorf("failed to create Chat agent: %w", err)
	}

	// Extract modules from Chat agent
	for _, module := range chatAgent.Modules {
		switch m := module.(type) {
		case *chat.LightweightReceptionModule:
			al.chatReceptionModule = m
		case *chat.FinalDecisionModule:
			al.chatDecisionModule = m
		}
	}

	// Create Worker agent with modules
	workerAgent, err := NewAgentWithModules(ctx, "worker", al.cfg, al.bus, al.sessions, al.router)
	if err != nil {
		return fmt.Errorf("failed to create Worker agent: %w", err)
	}

	// Extract modules from Worker agent
	for _, module := range workerAgent.Modules {
		switch m := module.(type) {
		case *worker.RoutingModule:
			al.workerRoutingModule = m
		case *worker.ExecutionModule:
			al.workerExecutionModule = m
		case *worker.AggregationModule:
			al.workerAggregationModule = m
		case *worker.HeartbeatCollectorModule:
			al.workerHeartbeatModule = m
		}
	}

	logger.InfoCF("agent", "new_arch.initialized", map[string]interface{}{
		"chat_modules":   len(chatAgent.Modules),
		"worker_modules": len(workerAgent.Modules),
	})

	return nil
}
