package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestWildAgentGenerateUsesWildPromptAndStripsCommand(t *testing.T) {
	var captured llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			captured = req
			return llm.GenerateResponse{Content: " vivid prompt "}, nil
		},
	}
	wild := NewWildAgent(provider, "creative system")

	resp, err := wild.Generate(context.Background(), task.NewTask(task.NewJobID(), "/wild 森の魔女の画像プロンプト", "line", "U123"))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp != "vivid prompt" {
		t.Fatalf("response should be trimmed, got %q", resp)
	}
	if captured.SystemPrompt != "creative system" {
		t.Fatalf("SystemPrompt: want custom prompt, got %q", captured.SystemPrompt)
	}
	if len(captured.Messages) != 1 || captured.Messages[0].Role != "user" {
		t.Fatalf("expected one user message, got %#v", captured.Messages)
	}
	if strings.Contains(captured.Messages[0].Content, "/wild") {
		t.Fatalf("wild command should be stripped, got %q", captured.Messages[0].Content)
	}
}
