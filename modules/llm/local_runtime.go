package llm

import (
	"strings"
	"time"
)

const (
	LocalProviderOllama = "ollama"
	LocalProviderOpenAI = "openai"

	LocalDefaultTimeout = 120 * time.Second
	LocalChatTimeout    = 10 * time.Second
	LocalWildTimeout    = 15 * time.Second
	LocalHeavyTimeout   = 30 * time.Second

	LocalOllamaDefaultNumCtx = 32768
	LegacyOllamaChatNumCtx   = 32768
	LegacyOllamaWorkerNumCtx = 16384
)

type LocalRuntimeConfig struct {
	Provider         string
	BaseURL          string
	ChatBaseURL      string
	WorkerBaseURL    string
	HeavyBaseURL     string
	WildBaseURL      string
	ChatModel        string
	WorkerModel      string
	HeavyModel       string
	WildModel        string
	TimeoutSec       int
	ModelConcurrency int
}

type LocalAliasConfig struct {
	Alias       string
	Provider    string
	BaseURL     string
	Model       string
	Timeout     time.Duration
	Concurrency int
	NumCtx      int
}

func BuildLocalAliasConfig(cfg LocalRuntimeConfig, alias string) LocalAliasConfig {
	return LocalAliasConfig{
		Alias:       strings.TrimSpace(alias),
		Provider:    NormalizeLocalProvider(cfg.Provider),
		BaseURL:     LocalBaseURLForAlias(cfg, alias),
		Model:       LocalModelForAlias(cfg, alias),
		Timeout:     LocalTimeoutForAlias(cfg, alias),
		Concurrency: cfg.ModelConcurrency,
		NumCtx:      LocalOllamaNumCtxForAlias(alias),
	}
}

func NormalizeLocalProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case LocalProviderOllama:
		return LocalProviderOllama
	default:
		return LocalProviderOpenAI
	}
}

func LocalOllamaNumCtxForAlias(_ string) int {
	return LocalOllamaDefaultNumCtx
}

func LocalTimeoutForAlias(cfg LocalRuntimeConfig, alias string) time.Duration {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case RoleChat:
		return LocalChatTimeout
	case RoleWild:
		return LocalWildTimeout
	case RoleHeavy:
		return LocalHeavyTimeout
	}
	if cfg.TimeoutSec <= 0 {
		return LocalDefaultTimeout
	}
	return time.Duration(cfg.TimeoutSec) * time.Second
}

func LocalBaseURLForAlias(cfg LocalRuntimeConfig, alias string) string {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case RoleChat:
		return FirstNonEmpty(cfg.ChatBaseURL, cfg.BaseURL)
	case RoleWorker:
		return FirstNonEmpty(cfg.WorkerBaseURL, cfg.BaseURL)
	case RoleHeavy:
		return FirstNonEmpty(cfg.HeavyBaseURL, cfg.WorkerBaseURL, cfg.BaseURL)
	case RoleWild:
		return FirstNonEmpty(cfg.WildBaseURL, cfg.BaseURL)
	default:
		return strings.TrimSpace(cfg.BaseURL)
	}
}

func LocalModelForAlias(cfg LocalRuntimeConfig, alias string) string {
	switch strings.ToLower(strings.TrimSpace(alias)) {
	case RoleChat:
		return cfg.ChatModel
	case RoleWorker:
		return cfg.WorkerModel
	case RoleHeavy:
		if strings.TrimSpace(cfg.HeavyBaseURL) == "" && strings.TrimSpace(cfg.WorkerBaseURL) != "" {
			return cfg.WorkerModel
		}
		return cfg.HeavyModel
	case RoleWild:
		return cfg.WildModel
	default:
		return ""
	}
}

func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func MaxDuration(values ...time.Duration) time.Duration {
	var max time.Duration
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}
