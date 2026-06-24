package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestHeavyAgentGenerateUsesHeavyPromptAndStripsCommand(t *testing.T) {
	var gotReq llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			gotReq = req
			return llm.GenerateResponse{Content: "heavy response"}, nil
		},
	}

	heavy := NewHeavyAgent(provider, "kuro system")
	resp, err := heavy.Generate(context.Background(), task.NewTask(task.NewJobID(), "/analyze 原因を調べて", "line", "U123"))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp != "heavy response" {
		t.Fatalf("response: want heavy response, got %q", resp)
	}
	if gotReq.SystemPrompt != "kuro system" {
		t.Fatalf("system prompt: want kuro system, got %q", gotReq.SystemPrompt)
	}
	if len(gotReq.Messages) == 0 || gotReq.Messages[len(gotReq.Messages)-1].Content != "原因を調べて" {
		t.Fatalf("expected stripped user message, got %#v", gotReq.Messages)
	}
}

func TestHeavyAgentDefaultPrompt(t *testing.T) {
	var gotReq llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			gotReq = req
			return llm.GenerateResponse{Content: "ok"}, nil
		},
	}

	heavy := NewHeavyAgent(provider, "")
	_, err := heavy.Generate(context.Background(), task.NewTask(task.NewJobID(), "診断して", "line", "U123"))
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(gotReq.SystemPrompt, "Heavy") {
		t.Fatalf("expected default Heavy prompt, got %q", gotReq.SystemPrompt)
	}
}
