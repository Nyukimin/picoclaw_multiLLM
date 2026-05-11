package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
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

func TestWildAgentGenerateAppliesWildRecallRoleFilter(t *testing.T) {
	engine := &mockConversationEngine{
		beginTurnFunc: func(ctx context.Context, sessionID, msg string) (*conversation.RecallPack, error) {
			return &conversation.RecallPack{
				MidSummaries: []conversation.ThreadSummary{
					{Summary: "wild mood board", Roles: []string{"wild"}},
					{Summary: "worker plan", Roles: []string{"worker"}},
				},
				SearchCacheSnippets: []conversation.SearchCacheSnippet{
					{Query: "worker report", Roles: []string{"worker"}},
				},
			}, nil
		},
	}
	var captured llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			captured = req
			return llm.GenerateResponse{Content: " vivid prompt "}, nil
		},
	}
	wild := NewWildAgent(provider, "creative system").WithConversationEngine(engine)

	if _, err := wild.Generate(context.Background(), task.NewTask(task.NewJobID(), "/wild 森の魔女", "line", "U123")); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	var prompt strings.Builder
	for _, msg := range captured.Messages {
		prompt.WriteString(msg.Content)
		prompt.WriteString("\n")
	}
	got := prompt.String()
	if !strings.Contains(got, "wild mood board") {
		t.Fatalf("wild recall should be included, got:\n%s", got)
	}
	if strings.Contains(got, "worker plan") || strings.Contains(got, "worker report") {
		t.Fatalf("worker recall should be filtered for wild, got:\n%s", got)
	}
}
