package main

import (
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	llmmiddleware "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/middleware"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
)

func buildLocalAliasProvider(cfg *config.Config, alias, model string, timeout time.Duration, global chan struct{}) llm.LLMProvider {
	var raw llm.LLMProvider
	baseURL := localLLMBaseURLForAlias(cfg, alias)
	switch cfg.LocalLLM.Provider {
	case "ollama":
		raw = ollama.NewOllamaProviderWithNumCtx(baseURL, model, 32768)
	default:
		raw = openai.NewOpenAIProviderWithOptions(cfg.LocalLLM.APIKey, model, baseURL, timeout)
	}
	modelSem := make(chan struct{}, cfg.LocalLLM.ModelConcurrency)
	return llmmiddleware.NewLimitedProvider(raw, "local-"+alias+"-"+model, global, modelSem)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstNonNilLLMProvider(values ...llm.LLMProvider) llm.LLMProvider {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func maxDuration(values ...time.Duration) time.Duration {
	var max time.Duration
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
