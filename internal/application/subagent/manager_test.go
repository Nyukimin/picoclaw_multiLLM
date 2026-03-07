package subagent

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// --- モック ---

type mockProvider struct {
	responses []llm.ChatResponse
	callIndex int
	lastReq   llm.ChatRequest
}

func (m *mockProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	return llm.GenerateResponse{}, fmt.Errorf("not implemented")
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	m.lastReq = req
	if m.callIndex >= len(m.responses) {
		return llm.ChatResponse{}, fmt.Errorf("no more responses")
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

type mockRunner struct {
	results map[string]*tool.ToolResponse
}

func (m *mockRunner) ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*tool.ToolResponse, error) {
	if r, ok := m.results[toolName]; ok {
		return r, nil
	}
	return nil, fmt.Errorf("unknown tool: %s", toolName)
}

func (m *mockRunner) ListTools(ctx context.Context) ([]tool.ToolMetadata, error) {
	return nil, nil
}

// --- テスト ---

func TestRunSync_Success(t *testing.T) {
	provider := &mockProvider{
		responses: []llm.ChatResponse{
			{
				Message: llm.ChatMessage{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{
						{ID: "c1", Function: llm.ToolCallFunction{Name: "web_search", Arguments: map[string]any{"query": "test"}}},
					},
				},
				FinishReason: "tool_calls",
			},
			{
				Message:      llm.ChatMessage{Role: "assistant", Content: "検索完了しました"},
				FinishReason: "stop",
			},
		},
	}

	runner := &mockRunner{
		results: map[string]*tool.ToolResponse{
			"web_search": tool.NewSuccess("search result"),
		},
	}

	mgr := NewManager(provider, runner, nil, toolloop.Config{MaxIterations: 10})
	result, err := mgr.RunSync(context.Background(), agent.SubagentTask{
		AgentName:   "worker",
		Instruction: "testを検索して",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AgentName != "worker" {
		t.Errorf("expected agent name 'worker', got '%s'", result.AgentName)
	}
	if result.Output != "検索完了しました" {
		t.Errorf("expected output '検索完了しました', got '%s'", result.Output)
	}
}

func TestRunSync_WithSystemPrompt(t *testing.T) {
	provider := &mockProvider{
		responses: []llm.ChatResponse{
			{
				Message:      llm.ChatMessage{Role: "assistant", Content: "done"},
				FinishReason: "stop",
			},
		},
	}

	mgr := NewManager(provider, &mockRunner{}, nil, toolloop.Config{MaxIterations: 10})
	_, err := mgr.RunSync(context.Background(), agent.SubagentTask{
		AgentName:    "worker",
		Instruction:  "do something",
		SystemPrompt: "You are a custom agent.",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Chat に渡されたメッセージの先頭が custom system prompt であること
	if len(provider.lastReq.Messages) < 1 {
		t.Fatal("expected at least 1 message")
	}
	if provider.lastReq.Messages[0].Content != "You are a custom agent." {
		t.Errorf("expected custom system prompt, got '%s'", provider.lastReq.Messages[0].Content)
	}
}

func TestRunSync_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider := &mockProvider{
		responses: []llm.ChatResponse{
			{Message: llm.ChatMessage{Role: "assistant", Content: "x"}, FinishReason: "stop"},
		},
	}

	mgr := NewManager(provider, &mockRunner{}, nil, toolloop.Config{MaxIterations: 10})
	_, err := mgr.RunSync(ctx, agent.SubagentTask{
		AgentName:   "worker",
		Instruction: "test",
	})

	if err == nil {
		t.Fatal("expected context cancelled error")
	}
}

func TestRunSync_EmptyInstruction(t *testing.T) {
	mgr := NewManager(nil, nil, nil, toolloop.Config{})
	_, err := mgr.RunSync(context.Background(), agent.SubagentTask{
		AgentName:   "worker",
		Instruction: "",
	})

	if err == nil {
		t.Fatal("expected error for empty instruction")
	}
}
