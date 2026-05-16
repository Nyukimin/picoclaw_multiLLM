package main

import (
	"context"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	llmmiddleware "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/middleware"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
)

type primaryLLMProviders struct {
	Chat   llm.LLMProvider
	Worker llm.LLMProvider
	Heavy  llm.LLMProvider
	Wild   llm.LLMProvider
}

const (
	localLLMDefaultTimeout = 120 * time.Second
	localLLMChatTimeout    = 10 * time.Second
	localLLMWildTimeout    = 15 * time.Second
	localLLMHeavyTimeout   = 30 * time.Second
)

func buildPrimaryLLMProviders(cfg *config.Config) primaryLLMProviders {
	if cfg.LocalLLM.Enabled {
		global := make(chan struct{}, cfg.LocalLLM.GlobalConcurrency)
		chatTimeout := localLLMTimeoutForAlias(cfg, "Chat")
		workerTimeout := localLLMTimeoutForAlias(cfg, "Worker")
		heavyTimeout := localLLMTimeoutForAlias(cfg, "Heavy")
		wildTimeout := localLLMTimeoutForAlias(cfg, "Wild")
		chat := buildLocalAliasProvider(cfg, "Chat", cfg.LocalLLM.ChatModel, chatTimeout, global)
		worker := buildLocalAliasProvider(cfg, "Worker", cfg.LocalLLM.WorkerModel, workerTimeout, global)
		heavy := buildLocalAliasProvider(cfg, "Heavy", localLLMModelForAlias(cfg, "Heavy"), heavyTimeout, global)
		wild := buildLocalAliasProvider(cfg, "Wild", cfg.LocalLLM.WildModel, wildTimeout, global)
		if cfg.LocalLLMWarmupEnabled() {
			go warmPrimaryLLMProviders(context.Background(), map[string]llm.LLMProvider{
				"Chat":   chat,
				"Worker": worker,
				"Heavy":  heavy,
				"Wild":   wild,
			}, maxDuration(chatTimeout, workerTimeout, heavyTimeout, wildTimeout))
		}
		return primaryLLMProviders{
			Chat:   llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(chat), "chat"),
			Worker: llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(worker), "worker"),
			Heavy:  llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(heavy), "heavy"),
			Wild:   llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(wild), "wild"),
		}
	}

	chatRawProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, cfg.Ollama.Model, 32768)
	workerModel := strings.TrimSpace(cfg.Ollama.WorkerModel)
	if workerModel == "" {
		workerModel = cfg.Ollama.Model
	}
	workerRawProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, workerModel, 16384)
	return primaryLLMProviders{
		Chat:   llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(chatRawProvider), "chat"),
		Worker: llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(workerRawProvider), "worker"),
		Heavy:  llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(workerRawProvider), "heavy"),
		Wild:   llmmiddleware.NewRawLogProvider(llmmiddleware.NewDateTimeProvider(workerRawProvider), "wild"),
	}
}

func localLLMTimeoutForAlias(cfg *config.Config, alias string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "chat":
		return localLLMChatTimeout
	case "wild":
		return localLLMWildTimeout
	case "heavy":
		return localLLMHeavyTimeout
	}
	if cfg == nil || cfg.LocalLLM.TimeoutSec <= 0 {
		return localLLMDefaultTimeout
	}
	return time.Duration(cfg.LocalLLM.TimeoutSec) * time.Second
}

func localLLMBaseURLForAlias(cfg *config.Config, alias string) string {
	if cfg == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "chat":
		return firstNonEmpty(cfg.LocalLLM.ChatBaseURL, cfg.LocalLLM.BaseURL)
	case "worker":
		return firstNonEmpty(cfg.LocalLLM.WorkerBaseURL, cfg.LocalLLM.BaseURL)
	case "heavy":
		return firstNonEmpty(cfg.LocalLLM.HeavyBaseURL, cfg.LocalLLM.WorkerBaseURL, cfg.LocalLLM.BaseURL)
	case "wild":
		return firstNonEmpty(cfg.LocalLLM.WildBaseURL, cfg.LocalLLM.BaseURL)
	default:
		return cfg.LocalLLM.BaseURL
	}
}

func localLLMModelForAlias(cfg *config.Config, alias string) string {
	if cfg == nil {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case "chat":
		return cfg.LocalLLM.ChatModel
	case "worker":
		return cfg.LocalLLM.WorkerModel
	case "heavy":
		if strings.TrimSpace(cfg.LocalLLM.HeavyBaseURL) == "" && strings.TrimSpace(cfg.LocalLLM.WorkerBaseURL) != "" {
			return cfg.LocalLLM.WorkerModel
		}
		return cfg.LocalLLM.HeavyModel
	case "wild":
		return cfg.LocalLLM.WildModel
	default:
		return ""
	}
}
