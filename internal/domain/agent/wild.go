package agent

import (
	"context"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

const defaultWildSystemPrompt = `You are Wild, a creative LLM for RenCrow.
Focus on story generation, image prompts, mood, composition, clothing, texture, and visual interpretation for creative work.
Answer naturally and concretely in the user's language.`

// WildAgent は創作Wild用のLLM呼び出しを担当する。
type WildAgent struct {
	llmProvider  llm.LLMProvider
	systemPrompt string
}

func NewWildAgent(llmProvider llm.LLMProvider, systemPrompt string) *WildAgent {
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultWildSystemPrompt
	}
	return &WildAgent{llmProvider: llmProvider, systemPrompt: systemPrompt}
}

func (w *WildAgent) Generate(ctx context.Context, t task.Task) (string, error) {
	req := llm.GenerateRequest{
		SystemPrompt: w.systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: stripWildCommand(t.UserMessage())},
		},
		MaxTokens:   2048,
		Temperature: 0.8,
	}
	resp, err := w.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func stripWildCommand(message string) string {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "/wild") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "/wild"))
	}
	return trimmed
}
