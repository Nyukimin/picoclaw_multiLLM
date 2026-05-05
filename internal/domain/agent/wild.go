package agent

import (
	"context"
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

const defaultWildSystemPrompt = `You are Wild, a creative LLM for RenCrow.
Focus on story generation, image prompts, mood, composition, clothing, texture, and visual interpretation for creative work.
Answer naturally and concretely in the user's language.`

// WildAgent は創作Wild用のLLM呼び出しを担当する。
type WildAgent struct {
	llmProvider        llm.LLMProvider
	systemPrompt       string
	conversationEngine conversation.ConversationEngine
}

func NewWildAgent(llmProvider llm.LLMProvider, systemPrompt string) *WildAgent {
	if strings.TrimSpace(systemPrompt) == "" {
		systemPrompt = defaultWildSystemPrompt
	}
	return &WildAgent{llmProvider: llmProvider, systemPrompt: systemPrompt}
}

func (w *WildAgent) WithConversationEngine(engine conversation.ConversationEngine) *WildAgent {
	w.conversationEngine = engine
	return w
}

func (w *WildAgent) Generate(ctx context.Context, t task.Task) (string, error) {
	userMessage := stripWildCommand(t.UserMessage())
	messages := []llm.Message{}
	if w.conversationEngine != nil {
		recallPack, err := w.conversationEngine.BeginTurn(ctx, t.ChatID(), userMessage)
		if err != nil {
			log.Printf("[Wild] BeginTurn failed: %v", err)
		} else if recallPack != nil {
			filtered := recallPack.FilterForRole("wild")
			messages = append(messages, filtered.ToPromptMessages()...)
		}
	}
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})
	req := llm.GenerateRequest{
		SystemPrompt: w.systemPrompt,
		Messages:     messages,
		MaxTokens:    2048,
		Temperature:  0.8,
	}
	resp, err := w.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}
	response := strings.TrimSpace(resp.Content)
	if w.conversationEngine != nil {
		if err := endConversationTurnAs(ctx, w.conversationEngine, t.ChatID(), userMessage, response, conversation.Speaker("wild")); err != nil {
			log.Printf("[Wild] EndTurn failed: %v", err)
		}
	}
	return response, nil
}

func stripWildCommand(message string) string {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "/wild") {
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "/wild"))
	}
	return trimmed
}
