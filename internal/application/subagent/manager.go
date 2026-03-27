package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

const defaultSystemPrompt = "You are a helpful assistant. Use the provided tools to complete the task."

// ManagerOption は Manager の追加設定オプション
type ManagerOption func(*Manager)

// WithToolRegistry は ToolRegistry を Manager に注入する（Phase 4）
// RunSync 毎に承認済みツールを toolDefs に動的マージする
func WithToolRegistry(reg capability.ToolRegistry) ManagerOption {
	return func(m *Manager) {
		m.registry = reg
	}
}

// Manager はサブエージェントタスクの実行を管理する
type Manager struct {
	provider   llm.ToolCallingProvider
	toolRunner tool.RunnerV2
	toolDefs   []llm.ToolDefinition
	loopConfig toolloop.Config
	registry   capability.ToolRegistry // Phase 4: 動的ツール読込用（nil = 無効）
}

// NewManager は新しい Manager を作成する
func NewManager(
	provider llm.ToolCallingProvider,
	toolRunner tool.RunnerV2,
	toolDefs []llm.ToolDefinition,
	loopConfig toolloop.Config,
	opts ...ManagerOption,
) *Manager {
	m := &Manager{
		provider:   provider,
		toolRunner: toolRunner,
		toolDefs:   toolDefs,
		loopConfig: loopConfig,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
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

	mergedDefs := m.mergeToolDefs(ctx)
	output, err := toolloop.Run(ctx, m.provider, m.toolRunner, mergedDefs, messages, m.loopConfig)
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

// mergeToolDefs は base toolDefs と ToolRegistry の承認済みツールをマージする
// base のツールが優先される（名前重複は base が勝つ）
func (m *Manager) mergeToolDefs(ctx context.Context) []llm.ToolDefinition {
	if m.registry == nil {
		return m.toolDefs
	}

	entries, err := m.registry.ListForPlatform(ctx, runtime.GOOS)
	if err != nil {
		log.Printf("[Subagent] WARN: registry list failed: %v", err)
		return m.toolDefs
	}

	if len(entries) == 0 {
		return m.toolDefs
	}

	// base ツールの名前セット（重複チェック用）
	existing := make(map[string]bool, len(m.toolDefs))
	for _, d := range m.toolDefs {
		existing[d.Function.Name] = true
	}

	merged := make([]llm.ToolDefinition, len(m.toolDefs))
	copy(merged, m.toolDefs)

	for _, entry := range entries {
		if existing[entry.Name] {
			continue // base ツールが優先
		}
		var toolDef llm.ToolDefinition
		if err := json.Unmarshal([]byte(entry.SchemaJSON), &toolDef); err != nil {
			log.Printf("[Subagent] WARN: skip registry tool %q: invalid schema: %v", entry.Name, err)
			continue
		}
		merged = append(merged, toolDef)
		existing[entry.Name] = true
	}

	return merged
}
