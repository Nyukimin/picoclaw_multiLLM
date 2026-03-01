package worker

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
)

// ExecutionModule handles direct task execution by Worker.
// For tasks that don't require Order agents (e.g., CHAT, OPS, RESEARCH),
// Worker executes them directly using this module.
type ExecutionModule struct {
	agent *modules.AgentCore
}

// NewExecutionModule creates a new execution module.
func NewExecutionModule() *ExecutionModule {
	return &ExecutionModule{}
}

// Name returns the module name.
func (m *ExecutionModule) Name() string {
	return "Execution"
}

// Initialize initializes the module with the agent core.
func (m *ExecutionModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *ExecutionModule) Shutdown(ctx context.Context) error {
	return nil
}

// ExecuteTask executes a task directly using Worker's capabilities.
// This is used for non-coding tasks that don't require delegation to Order agents.
func (m *ExecutionModule) ExecuteTask(ctx context.Context, jobID, route, userText string) (string, error) {
	logger.InfoCF("execution", "worker.execute", map[string]interface{}{
		"job_id": jobID,
		"route":  route,
	})

	if m.agent.Provider == nil {
		return "", fmt.Errorf("LLM provider not configured for Worker")
	}

	// Execute based on route
	switch route {
	case "CHAT":
		return m.executeChat(ctx, userText)
	case "OPS":
		return m.executeOps(ctx, userText)
	case "RESEARCH":
		return m.executeResearch(ctx, userText)
	case "PLAN":
		return m.executePlan(ctx, userText)
	case "ANALYZE":
		return m.executeAnalyze(ctx, userText)
	default:
		return "", fmt.Errorf("unsupported route for direct execution: %s", route)
	}
}

func (m *ExecutionModule) executeChat(ctx context.Context, userText string) (string, error) {
	// Use Provider to generate chat response
	messages := []providers.Message{
		{
			Role:    "user",
			Content: userText,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return "", fmt.Errorf("chat execution failed: %w", err)
	}

	logger.InfoCF("execution", "worker.chat.complete", map[string]interface{}{
		"tokens": response.Usage.TotalTokens,
	})

	return response.Content, nil
}

func (m *ExecutionModule) executeOps(ctx context.Context, userText string) (string, error) {
	// Use Provider with OPS-specific system prompt
	systemPrompt := "あなたは運用・手順のスペシャリストです。手順を明確に説明します。"
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userText,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return "", fmt.Errorf("ops execution failed: %w", err)
	}

	return response.Content, nil
}

func (m *ExecutionModule) executeResearch(ctx context.Context, userText string) (string, error) {
	// Use Provider with RESEARCH-specific system prompt
	systemPrompt := "あなたは調査のスペシャリストです。情報を収集・整理してまとめます。"
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userText,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return "", fmt.Errorf("research execution failed: %w", err)
	}

	return response.Content, nil
}

func (m *ExecutionModule) executePlan(ctx context.Context, userText string) (string, error) {
	// Use Provider with PLAN-specific system prompt
	systemPrompt := "あなたは計画立案のスペシャリストです。段取りを整理して提示します。"
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userText,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return "", fmt.Errorf("plan execution failed: %w", err)
	}

	return response.Content, nil
}

func (m *ExecutionModule) executeAnalyze(ctx context.Context, userText string) (string, error) {
	// Use Provider with ANALYZE-specific system prompt
	systemPrompt := "あなたは分析のスペシャリストです。情報を整理・分析して提示します。"
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userText,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return "", fmt.Errorf("analyze execution failed: %w", err)
	}

	return response.Content, nil
}
