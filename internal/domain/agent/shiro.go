package agent

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// ShiroAgent は Worker（実行・道具係）を担当するエンティティ
type ShiroAgent struct {
	llmProvider     llm.LLMProvider
	toolRunner      ToolRunner
	mcpClient       MCPClient
	systemPrompt    string
	subagentManager SubagentManager // v1.0: ReActループ統合
}

// NewShiroAgent は新しいShiroAgentを作成
func NewShiroAgent(
	llmProvider llm.LLMProvider,
	toolRunner ToolRunner,
	mcpClient MCPClient,
	systemPrompt string,
	subagentManager SubagentManager,
) *ShiroAgent {
	return &ShiroAgent{
		llmProvider:     llmProvider,
		toolRunner:      toolRunner,
		mcpClient:       mcpClient,
		systemPrompt:    systemPrompt,
		subagentManager: subagentManager,
	}
}

// Execute はWorkerタスクを実行
// v1.0: SubagentManager が設定されている場合は ReActLoop を使ってツールを自律的に選択・実行する
func (s *ShiroAgent) Execute(ctx context.Context, t task.Task) (string, error) {
	// SubagentManager が設定されている場合は ReActLoop を使用
	if s.subagentManager != nil {
		result, err := s.subagentManager.RunSync(ctx, SubagentTask{
			AgentName:    "shiro",
			Instruction:  t.UserMessage(),
			SystemPrompt: s.systemPrompt,
		})
		if err != nil {
			return "", err
		}
		return result.Output, nil
	}

	// フォールバック: SubagentManager がない場合は従来通りの単純な LLM 呼び出し
	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: s.systemPrompt,
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
