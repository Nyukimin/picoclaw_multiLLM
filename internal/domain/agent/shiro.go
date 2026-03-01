package agent

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ShiroAgent は Worker（実行・道具係）を担当するエンティティ
type ShiroAgent struct {
	llmProvider llm.LLMProvider
	toolRunner  ToolRunner
	mcpClient   MCPClient
}

// NewShiroAgent は新しいShiroAgentを作成
func NewShiroAgent(
	llmProvider llm.LLMProvider,
	toolRunner ToolRunner,
	mcpClient MCPClient,
) *ShiroAgent {
	return &ShiroAgent{
		llmProvider: llmProvider,
		toolRunner:  toolRunner,
		mcpClient:   mcpClient,
	}
}

// Execute はWorkerタスクを実行
func (s *ShiroAgent) Execute(ctx context.Context, t task.Task) (string, error) {
	// Workerロジック: LLMにツール定義を渡して、適切なツールを選択・実行させる
	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: "You are a worker agent. Execute tasks using available tools.",
			},
			{
				Role:    "user",
				Content: t.UserMessage(),
			},
		},
		MaxTokens:   4096,
		Temperature: 0.3, // Workerは確実性重視
	}

	resp, err := s.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// ExecuteTool はツールを実行
func (s *ShiroAgent) ExecuteTool(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	return s.toolRunner.Execute(ctx, toolName, args)
}

// ExecuteMCPTool はMCPツールを実行
func (s *ShiroAgent) ExecuteMCPTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error) {
	return s.mcpClient.CallTool(ctx, serverName, toolName, args)
}
