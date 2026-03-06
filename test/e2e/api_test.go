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

func TestE2E_APIProvider_Generate(t *testing.T) {
	cases := []struct {
		name    string
		envVar  string
		factory func(apiKey string) llm.LLMProvider
	}{
		{"Claude", "ANTHROPIC_API_KEY", func(k string) llm.LLMProvider { return claude.NewClaudeProvider(k, "claude-sonnet-4-20250514") }},
		{"DeepSeek", "DEEPSEEK_API_KEY", func(k string) llm.LLMProvider { return deepseek.NewDeepSeekProvider(k, "deepseek-chat") }},
		{"OpenAI", "OPENAI_API_KEY", func(k string) llm.LLMProvider { return openai.NewOpenAIProvider(k, "gpt-4o-mini") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			apiKey := os.Getenv(tc.envVar)
			if apiKey == "" {
				t.Skipf("%s not set", tc.envVar)
			}

			provider := tc.factory(apiKey)

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
			t.Logf("%s response: %s", tc.name, resp.Content)
		})
	}
}
