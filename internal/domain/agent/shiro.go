package agent

import (
	"context"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
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
	persona         *AgentPersona   // v4.2: Optional Agent Persona
	conversation    conversation.ConversationEngine
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

// WithPersona は AgentPersona を設定する（Builder パターン）
func (s *ShiroAgent) WithPersona(persona AgentPersona) *ShiroAgent {
	s.persona = &persona
	return s
}

func (s *ShiroAgent) WithConversationEngine(engine conversation.ConversationEngine) *ShiroAgent {
	s.conversation = engine
	return s
}

// Execute はWorkerタスクを実行
// v1.0: SubagentManager が設定されている場合は ReActLoop を使ってツールを自律的に選択・実行する
func (s *ShiroAgent) Execute(ctx context.Context, t task.Task) (string, error) {
	systemPrompt := s.systemPrompt
	if s.persona != nil {
		systemPrompt = s.persona.BuildSystemPrompt(s.systemPrompt)
	}

	// SubagentManager が設定されている場合は ReActLoop を使用
	if s.subagentManager != nil {
		result, err := s.subagentManager.RunSync(ctx, SubagentTask{
			AgentName:    "shiro",
			Instruction:  t.UserMessage(),
			SystemPrompt: systemPrompt,
		})
		if err != nil {
			return "", err
		}
		return result.Output, nil
	}

	// フォールバック: SubagentManager がない場合は従来通りの単純な LLM 呼び出し
	messages := []llm.Message{{Role: "system", Content: systemPrompt}}
	if s.conversation != nil {
		recallPack, err := s.conversation.BeginTurn(ctx, t.ChatID(), t.UserMessage())
		if err != nil {
			log.Printf("[Shiro] BeginTurn failed: %v", err)
		} else if recallPack != nil {
			filtered := recallPack.FilterForRole("worker")
			messages = append(messages, filtered.ToPromptMessages()...)
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: t.UserMessage()})
	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   4096,
		Temperature: 0.3, // Workerは確実性重視
	}

	resp, err := s.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	if s.conversation != nil {
		if err := endConversationTurnAs(ctx, s.conversation, t.ChatID(), t.UserMessage(), resp.Content, conversation.SpeakerShiro); err != nil {
			log.Printf("[Shiro] EndTurn failed: %v", err)
		}
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
