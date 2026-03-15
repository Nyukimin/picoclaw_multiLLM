package subagent

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

const defaultSystemPrompt = "You are a helpful assistant. Use the provided tools to complete the task."

// Manager はサブエージェントタスクの実行を管理する
type Manager struct {
	provider   llm.ToolCallingProvider
	toolRunner tool.RunnerV2
	toolDefs   []llm.ToolDefinition
	loopConfig toolloop.Config
}

// NewManager は新しい Manager を作成する
func NewManager(
	provider llm.ToolCallingProvider,
	toolRunner tool.RunnerV2,
	toolDefs []llm.ToolDefinition,
	loopConfig toolloop.Config,
) *Manager {
	return &Manager{
		provider:   provider,
		toolRunner: toolRunner,
		toolDefs:   toolDefs,
		loopConfig: loopConfig,
	}
}

// RunSync はサブエージェントタスクを同期実行する
func (m *Manager) RunSync(ctx context.Context, task agent.SubagentTask) (agent.SubagentResult, error) {
	if task.Instruction == "" {
		return agent.SubagentResult{}, fmt.Errorf("instruction is required")
	}
	log.Printf("[Subagent] start agent=%s instruction_len=%d", task.AgentName, len(task.Instruction))

	systemPrompt := task.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = defaultSystemPrompt
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: task.Instruction},
	}

	output, err := toolloop.Run(ctx, m.provider, m.toolRunner, m.toolDefs, messages, m.loopConfig)
	if err != nil {
		log.Printf("[Subagent] error agent=%s err=%v", task.AgentName, err)
		return agent.SubagentResult{}, fmt.Errorf("subagent %s failed: %w", task.AgentName, err)
	}
	log.Printf("[Subagent] complete agent=%s output_len=%d", task.AgentName, len(output))

	return agent.SubagentResult{
		AgentName: task.AgentName,
		Output:    output,
	}, nil
}
