package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	llmmiddleware "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/middleware"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
)

func buildConversationTextProvider(cfg *config.Config, providers primaryLLMProviders) (llm.LLMProvider, string) {
	if cfg.LocalLLM.Enabled && providers.Worker != nil {
		return providers.Worker, "local_llm Worker"
	}
	summaryModel := strings.TrimSpace(cfg.Conversation.SummaryModel)
	if summaryModel == "" {
		summaryModel = cfg.Ollama.Model
	}
	if summaryModel == "" {
		return nil, ""
	}
	summaryProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, summaryModel, 32768)
	return llmmiddleware.NewRawLogProvider(summaryProvider, "conversation-summary"), fmt.Sprintf("%s (model: %s)", cfg.Ollama.BaseURL, summaryModel)
}

func buildConversationEmbedder(cfg *config.Config) (conversation.EmbeddingProvider, string) {
	model := strings.TrimSpace(cfg.Conversation.EmbedModel)
	if model == "" {
		return nil, ""
	}
	embedProvider := strings.ToLower(strings.TrimSpace(cfg.Conversation.EmbedProvider))
	embedBaseURL := strings.TrimSpace(cfg.Conversation.EmbedBaseURL)
	if embedProvider == "ollama" {
		if embedBaseURL == "" {
			embedBaseURL = cfg.Ollama.BaseURL
		}
		return ollama.NewOllamaEmbedder(embedBaseURL, model),
			fmt.Sprintf("conversation embedding ollama: %s (model: %s)", embedBaseURL, model)
	}
	timeout := time.Duration(cfg.LocalLLM.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if embedProvider == "openai" {
		if embedBaseURL == "" {
			embedBaseURL = cfg.LocalLLM.BaseURL
		}
		return openai.NewOpenAIEmbedderWithOptions(cfg.LocalLLM.APIKey, model, embedBaseURL, timeout),
			fmt.Sprintf("conversation embedding openai: %s (model: %s)", embedBaseURL, model)
	}
	if cfg.LocalLLM.Enabled && cfg.LocalLLM.Provider != "ollama" {
		return openai.NewOpenAIEmbedderWithOptions(cfg.LocalLLM.APIKey, model, cfg.LocalLLM.BaseURL, timeout),
			fmt.Sprintf("local_llm embedding: %s (model: %s)", cfg.LocalLLM.BaseURL, model)
	}
	baseURL := cfg.Ollama.BaseURL
	if cfg.LocalLLM.Enabled && cfg.LocalLLM.Provider == "ollama" {
		baseURL = cfg.LocalLLM.BaseURL
	}
	return ollama.NewOllamaEmbedder(baseURL, model), fmt.Sprintf("%s (model: %s)", baseURL, model)
}
