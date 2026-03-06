//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
)

func TestE2E_ClaudeProvider_Generate(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set")
	}

	provider := claude.NewClaudeProvider(apiKey, "claude-sonnet-4-20250514")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say hello in one word."},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Claude response: %s", resp.Content)
}

func TestE2E_DeepSeekProvider_Generate(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set")
	}

	provider := deepseek.NewDeepSeekProvider(apiKey, "deepseek-chat")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say hello in one word."},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("DeepSeek response: %s", resp.Content)
}

func TestE2E_OpenAIProvider_Generate(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set")
	}

	provider := openai.NewOpenAIProvider(apiKey, "gpt-4o-mini")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say hello in one word."},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("OpenAI response: %s", resp.Content)
}
