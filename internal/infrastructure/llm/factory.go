package llm

import (
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/gemini"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
)

// CreateProvider は CoderConfig から適切な LLMProvider を生成する。
// Enabled=false の場合は nil を返す（エラーではない）。
// Provider が未知の場合はエラーを返す。
func CreateProvider(cc config.CoderConfig) (llm.LLMProvider, error) {
	if !cc.Enabled {
		return nil, nil
	}

	switch cc.Provider {
	case "deepseek":
		if cc.APIKey == "" {
			return nil, fmt.Errorf("deepseek provider requires api_key")
		}
		return deepseek.NewDeepSeekProvider(cc.APIKey, cc.Model), nil

	case "openai":
		if cc.APIKey == "" {
			return nil, fmt.Errorf("openai provider requires api_key")
		}
		return openai.NewOpenAIProvider(cc.APIKey, cc.Model), nil

	case "claude":
		if cc.APIKey == "" {
			return nil, fmt.Errorf("claude provider requires api_key")
		}
		return claude.NewClaudeProvider(cc.APIKey, cc.Model), nil

	case "gemini":
		if cc.APIKey == "" {
			return nil, fmt.Errorf("gemini provider requires api_key")
		}
		return gemini.NewProvider(cc.APIKey, cc.Model), nil

	case "ollama":
		if cc.BaseURL == "" {
			return nil, fmt.Errorf("ollama provider requires base_url")
		}
		return ollama.NewOllamaProvider(cc.BaseURL, cc.Model), nil

	default:
		return nil, fmt.Errorf("unknown provider: %s", cc.Provider)
	}
}
