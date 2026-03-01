package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// Mock ToolRunner
type mockToolRunner struct {
	executeFunc func(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
	listFunc    func(ctx context.Context) ([]string, error)
}

func (m *mockToolRunner) Execute(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, toolName, args)
	}
	return "tool executed", nil
}

func (m *mockToolRunner) List(ctx context.Context) ([]string, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx)
	}
	return []string{"tool1", "tool2"}, nil
}

// Mock MCPClient
type mockMCPClient struct {
	callToolFunc  func(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error)
	listToolsFunc func(ctx context.Context, serverName string) ([]string, error)
}

func (m *mockMCPClient) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error) {
	if m.callToolFunc != nil {
		return m.callToolFunc(ctx, serverName, toolName, args)
	}
	return "mcp tool executed", nil
}

func (m *mockMCPClient) ListTools(ctx context.Context, serverName string) ([]string, error) {
	if m.listToolsFunc != nil {
		return m.listToolsFunc(ctx, serverName)
	}
	return []string{"mcp_tool1", "mcp_tool2"}, nil
}

func TestNewShiroAgent(t *testing.T) {
	llmProvider := &mockLLMProvider{}
	toolRunner := &mockToolRunner{}
	mcpClient := &mockMCPClient{}

	shiro := NewShiroAgent(llmProvider, toolRunner, mcpClient)

	if shiro == nil {
		t.Fatal("NewShiroAgent should not return nil")
	}

	if shiro.llmProvider != llmProvider {
		t.Error("llmProvider not set correctly")
	}

	if shiro.toolRunner != toolRunner {
		t.Error("toolRunner not set correctly")
	}

	if shiro.mcpClient != mcpClient {
		t.Error("mcpClient not set correctly")
	}
}

func TestShiroAgentExecute(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			// システムプロンプトにWorkerの指示が含まれているか確認
			if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
				if req.Messages[0].Content != "You are a worker agent. Execute tasks using available tools." {
					t.Errorf("Unexpected system prompt: %s", req.Messages[0].Content)
				}
			}

			return llm.GenerateResponse{
				Content:      "Task executed successfully",
				TokensUsed:   50,
				FinishReason: "stop",
			}, nil
		},
	}

	shiro := NewShiroAgent(llmProvider, &mockToolRunner{}, &mockMCPClient{})

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "ファイルを作成して", "line", "U123")

	result, err := shiro.Execute(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result != "Task executed successfully" {
		t.Errorf("Expected 'Task executed successfully', got '%s'", result)
	}
}

func TestShiroAgentExecute_LLMError(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{}, errors.New("LLM connection failed")
		},
	}

	shiro := NewShiroAgent(llmProvider, &mockToolRunner{}, &mockMCPClient{})

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "テスト", "line", "U123")

	_, err := shiro.Execute(context.Background(), testTask)
	if err == nil {
		t.Error("Expected error when LLM fails")
	}

	if err.Error() != "LLM connection failed" {
		t.Errorf("Expected 'LLM connection failed', got '%s'", err.Error())
	}
}

func TestShiroAgentExecuteTool(t *testing.T) {
	toolRunner := &mockToolRunner{
		executeFunc: func(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
			if toolName != "file_read" {
				t.Errorf("Expected tool 'file_read', got '%s'", toolName)
			}

			path, ok := args["path"].(string)
			if !ok || path != "/test/file.txt" {
				t.Errorf("Expected path '/test/file.txt', got '%v'", args["path"])
			}

			return "File content", nil
		},
	}

	shiro := NewShiroAgent(&mockLLMProvider{}, toolRunner, &mockMCPClient{})

	result, err := shiro.ExecuteTool(context.Background(), "file_read", map[string]interface{}{
		"path": "/test/file.txt",
	})

	if err != nil {
		t.Fatalf("ExecuteTool failed: %v", err)
	}

	if result != "File content" {
		t.Errorf("Expected 'File content', got '%s'", result)
	}
}

func TestShiroAgentExecuteMCPTool(t *testing.T) {
	mcpClient := &mockMCPClient{
		callToolFunc: func(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error) {
			if serverName != "browser" {
				t.Errorf("Expected server 'browser', got '%s'", serverName)
			}

			if toolName != "navigate" {
				t.Errorf("Expected tool 'navigate', got '%s'", toolName)
			}

			url, ok := args["url"].(string)
			if !ok || url != "https://example.com" {
				t.Errorf("Expected url 'https://example.com', got '%v'", args["url"])
			}

			return "Navigated to https://example.com", nil
		},
	}

	shiro := NewShiroAgent(&mockLLMProvider{}, &mockToolRunner{}, mcpClient)

	result, err := shiro.ExecuteMCPTool(context.Background(), "browser", "navigate", map[string]interface{}{
		"url": "https://example.com",
	})

	if err != nil {
		t.Fatalf("ExecuteMCPTool failed: %v", err)
	}

	if result != "Navigated to https://example.com" {
		t.Errorf("Expected navigation message, got '%s'", result)
	}
}

func TestShiroAgentExecuteTool_Error(t *testing.T) {
	toolRunner := &mockToolRunner{
		executeFunc: func(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
			return "", errors.New("tool execution failed")
		},
	}

	shiro := NewShiroAgent(&mockLLMProvider{}, toolRunner, &mockMCPClient{})

	_, err := shiro.ExecuteTool(context.Background(), "failing_tool", map[string]interface{}{})

	if err == nil {
		t.Error("Expected error when tool fails")
	}

	if err.Error() != "tool execution failed" {
		t.Errorf("Expected 'tool execution failed', got '%s'", err.Error())
	}
}
