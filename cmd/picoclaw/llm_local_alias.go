package main

import (
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	llmmiddleware "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/middleware"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
	modulellm "github.com/Nyukimin/picoclaw_multiLLM/modules/llm"
)

func buildLocalAliasProvider(cfg *config.Config, alias, model string, timeout time.Duration, global chan struct{}) llm.LLMProvider {
	aliasConfig := modulellm.LocalAliasConfig{
		Alias:       strings.TrimSpace(alias),
		Provider:    modulellm.NormalizeLocalProvider(localLLMProviderFromConfig(cfg)),
		BaseURL:     localLLMBaseURLForAlias(cfg, alias),
		Model:       localLLMModelForAlias(cfg, alias),
		Timeout:     localLLMTimeoutForAlias(cfg, alias),
		Concurrency: localLLMConcurrencyFromConfig(cfg),
		NumCtx:      modulellm.LocalOllamaNumCtxForAlias(alias),
	}
	if model != "" {
		aliasConfig.Model = model
	}
	if timeout > 0 {
		aliasConfig.Timeout = timeout
	}
	return buildLocalAliasProviderFromConfig(cfg, aliasConfig, global)
}

func buildLocalAliasProviderFromConfig(cfg *config.Config, aliasConfig modulellm.LocalAliasConfig, global chan struct{}) llm.LLMProvider {
	var raw llm.LLMProvider
	switch aliasConfig.Provider {
	case modulellm.LocalProviderOllama:
		raw = ollama.NewOllamaProviderWithNumCtx(aliasConfig.BaseURL, aliasConfig.Model, aliasConfig.NumCtx)
	default:
		apiKey := ""
		if cfg != nil {
			apiKey = cfg.LocalLLM.APIKey
		}
		raw = openai.NewOpenAIProviderWithOptions(apiKey, aliasConfig.Model, aliasConfig.BaseURL, aliasConfig.Timeout)
	}
	modelSem := make(chan struct{}, aliasConfig.Concurrency)
	return llmmiddleware.NewLimitedProvider(raw, "local-"+aliasConfig.Alias+"-"+aliasConfig.Model, global, modelSem)
}

func firstNonEmpty(values ...string) string {
	return modulellm.FirstNonEmpty(values...)
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
	return modulellm.MaxDuration(values...)
}

func localLLMProviderFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.LocalLLM.Provider
}

func localLLMConcurrencyFromConfig(cfg *config.Config) int {
	if cfg == nil {
		return 0
	}
	return cfg.LocalLLM.ModelConcurrency
}
