package config

import (
	"fmt"
	"log"
	"strings"
)

// Validate は設定の妥当性を検証
func (c *Config) Validate() error {
	// サーバー設定検証
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	if c.LocalLLM.Enabled {
		if c.LocalLLM.Provider != "local_openai" && c.LocalLLM.Provider != "ollama" {
			return fmt.Errorf("local_llm.provider must be one of [local_openai, ollama], got '%s'", c.LocalLLM.Provider)
		}
		if c.LocalLLM.BaseURL == "" && c.LocalLLM.ChatBaseURL == "" && c.LocalLLM.WorkerBaseURL == "" && c.LocalLLM.HeavyBaseURL == "" && c.LocalLLM.WildBaseURL == "" {
			return fmt.Errorf("local_llm base_url or role-specific base_url is required when enabled=true")
		}
		if c.LocalLLM.ChatModel == "" {
			return fmt.Errorf("local_llm chat_model is required when enabled=true")
		}
		if c.LocalLLM.WorkerModel == "" {
			return fmt.Errorf("local_llm worker_model is required when enabled=true")
		}
		if c.LocalLLM.HeavyModel == "" {
			return fmt.Errorf("local_llm heavy_model is required when enabled=true")
		}
		if c.LocalLLM.WildModel == "" {
			return fmt.Errorf("local_llm wild_model is required when enabled=true")
		}
		if c.LocalLLM.TimeoutSec < 1 {
			return fmt.Errorf("local_llm timeout_sec must be >= 1")
		}
		if c.LocalLLM.GlobalConcurrency < 1 {
			return fmt.Errorf("local_llm global_concurrency must be >= 1")
		}
		if c.LocalLLM.ModelConcurrency < 1 {
			return fmt.Errorf("local_llm model_concurrency must be >= 1")
		}
	}

	if c.LLMOps.Enabled {
		if strings.TrimSpace(c.LLMOps.BaseURL) == "" {
			return fmt.Errorf("llm_ops.base_url is required when llm_ops.enabled=true")
		}
	}

	if !c.LocalLLM.Enabled {
		// Ollama設定検証
		if c.Ollama.BaseURL == "" {
			return fmt.Errorf("ollama base_url is required")
		}

		if c.Ollama.Model == "" {
			return fmt.Errorf("ollama model is required")
		}
	}

	// セッション設定検証
	if c.Session.StorageDir == "" {
		return fmt.Errorf("session storage_dir is required")
	}

	// LINE設定検証（片方だけ設定は警告）
	hasSecret := c.Line.ChannelSecret != ""
	hasToken := c.Line.AccessToken != ""
	if hasSecret != hasToken {
		log.Println("WARN: LINE config incomplete - both channel_secret and access_token are required for webhook")
	}

	if c.VTuber.Enabled {
		if c.VTuber.TickIntervalMS < 50 || c.VTuber.TickIntervalMS > 100 {
			return fmt.Errorf("vtuber.tick_interval_ms must be between 50 and 100, got %d", c.VTuber.TickIntervalMS)
		}
		if len(c.VTuber.Characters) == 0 {
			return fmt.Errorf("vtuber.enabled=true requires at least one character")
		}
		for name, ch := range c.VTuber.Characters {
			if ch.AudioOutput == "" {
				return fmt.Errorf("vtuber.characters.%s.audio_output is required", name)
			}
			if ch.VTSHost == "" {
				return fmt.Errorf("vtuber.characters.%s.vts_host is required", name)
			}
			if ch.VTSPort < 1 || ch.VTSPort > 65535 {
				return fmt.Errorf("vtuber.characters.%s.vts_port must be 1-65535, got %d", name, ch.VTSPort)
			}
			if len(ch.ExpressionMap) == 0 {
				return fmt.Errorf("vtuber.characters.%s.expression_map is required", name)
			}
		}
	}

	if c.AudioRouter.Enabled {
		if c.AudioRouter.SSEURL == "" {
			return fmt.Errorf("audio_router.sse_url is required when audio_router.enabled=true")
		}
		if len(c.AudioRouter.DeviceMap) == 0 {
			return fmt.Errorf("audio_router.device_map is required when audio_router.enabled=true")
		}
		for name, dev := range c.AudioRouter.DeviceMap {
			if dev.DeviceID == "" {
				return fmt.Errorf("audio_router.device_map.%s.device_id is required", name)
			}
		}
		if c.AudioRouter.ConnectTimeoutMS < 1000 {
			return fmt.Errorf("audio_router.connect_timeout_ms must be >= 1000")
		}
		if c.AudioRouter.DownloadTimeoutMS < 1000 {
			return fmt.Errorf("audio_router.download_timeout_ms must be >= 1000")
		}
		if c.AudioRouter.RetryDelayMS < 250 {
			return fmt.Errorf("audio_router.retry_delay_ms must be >= 250")
		}
		if c.AudioRouter.BufferMS < 20 || c.AudioRouter.BufferMS > 5000 {
			return fmt.Errorf("audio_router.buffer_ms must be between 20 and 5000")
		}
	}

	// v4.0 Distributed設定検証
	if c.Distributed.Enabled {
		if len(c.Distributed.Transports) == 0 {
			return fmt.Errorf("distributed.enabled=true requires at least one transport")
		}
		for name, tc := range c.Distributed.Transports {
			if tc.Type != "local" && tc.Type != "ssh" {
				return fmt.Errorf("distributed.transports.%s.type must be 'local' or 'ssh', got '%s'", name, tc.Type)
			}
			if tc.Type == "ssh" {
				if tc.RemoteHost == "" {
					return fmt.Errorf("distributed.transports.%s.remote_host is required for ssh type", name)
				}
				if tc.RemoteUser == "" {
					return fmt.Errorf("distributed.transports.%s.remote_user is required for ssh type", name)
				}
				if tc.SSHKeyPath == "" {
					return fmt.Errorf("distributed.transports.%s.ssh_key_path is required for ssh type", name)
				}
			}
		}
	}

	// v4.0 IdleChat設定検証
	if c.IdleChat.Enabled {
		validAgents := map[string]bool{
			"mio": true, "shiro": true, "aka": true, "ao": true, "gin": true,
		}
		for _, p := range c.IdleChat.Participants {
			if !validAgents[p] {
				return fmt.Errorf("idle_chat.participants: unknown agent '%s'", p)
			}
		}
		effectiveIntervalSec := c.IdleChat.IntervalSec
		if effectiveIntervalSec == 0 {
			effectiveIntervalSec = c.IdleChat.IntervalMin * 60
		}
		if effectiveIntervalSec < 1 {
			return fmt.Errorf("idle_chat.interval_sec must be >= 1")
		}
		if c.IdleChat.MaxTurns < 1 || c.IdleChat.MaxTurns > 100 {
			return fmt.Errorf("idle_chat.max_turns must be between 1 and 100")
		}
		if c.IdleChat.Temperature < 0 || c.IdleChat.Temperature > 2.0 {
			return fmt.Errorf("idle_chat.temperature must be between 0 and 2.0")
		}
	}

	// v5.0 Conversation設定検証
	if c.Conversation.Enabled {
		if c.Conversation.RedisURL == "" {
			return fmt.Errorf("conversation.redis_url is required when conversation.enabled=true")
		}
		if c.Conversation.DuckDBPath == "" {
			return fmt.Errorf("conversation.duckdb_path is required when conversation.enabled=true")
		}
		if c.Conversation.VectorDBURL == "" {
			return fmt.Errorf("conversation.vectordb_url is required when conversation.enabled=true")
		}
	}

	if c.Security.Enabled {
		if c.Security.PolicyMode != "strict" && c.Security.PolicyMode != "balanced" && c.Security.PolicyMode != "dev" {
			return fmt.Errorf("security.policy_mode must be 'strict', 'balanced', or 'dev'")
		}
		if c.Security.NetworkScope != "" &&
			c.Security.NetworkScope != "blocked" &&
			c.Security.NetworkScope != "allowlist" &&
			c.Security.NetworkScope != "full" {
			return fmt.Errorf("security.network_scope must be 'blocked', 'allowlist', or 'full'")
		}
		if c.Security.Audit.Backend != "jsonl" && c.Security.Audit.Backend != "sqlite" {
			return fmt.Errorf("security.audit.backend must be 'jsonl' or 'sqlite'")
		}
	}
	if c.ViewerLog.Enabled {
		if c.ViewerLog.RetentionDays < 1 {
			return fmt.Errorf("viewer_log.retention_days must be >= 1")
		}
		if c.ViewerLog.GCIntervalMinutes < 1 {
			return fmt.Errorf("viewer_log.gc_interval_minutes must be >= 1")
		}
		if c.ViewerLog.Path == "" {
			return fmt.Errorf("viewer_log.path is required when viewer_log.enabled=true")
		}
	}

	// v4.1 Coder スロット検証
	coders := []struct {
		name   string
		config *CoderConfig
	}{
		{"coder1", &c.Coder1},
		{"coder2", &c.Coder2},
		{"coder3", &c.Coder3},
		{"coder4", &c.Coder4},
	}

	for _, coder := range coders {
		if err := validateCoderConfig(coder.name, coder.config); err != nil {
			return err
		}
	}

	return nil
}

// validateCoderConfig は単一 CoderConfig の妥当性を検証
func validateCoderConfig(name string, cc *CoderConfig) error {
	// Provider 検証
	validProviders := map[string]bool{
		"deepseek":     true,
		"openai":       true,
		"claude":       true,
		"gemini":       true,
		"ollama":       true,
		"local_openai": true,
	}
	if cc.Provider != "" && !validProviders[cc.Provider] {
		return fmt.Errorf("%s.provider must be one of [deepseek, openai, claude, gemini, ollama, local_openai], got '%s'", name, cc.Provider)
	}

	// Model 検証（Enabled=true の場合のみ必須）
	if cc.Enabled && cc.Model == "" {
		return fmt.Errorf("%s.model is required when enabled=true", name)
	}

	// Name 検証（識別子として使用されるため常に必須）
	if cc.Name == "" {
		return fmt.Errorf("%s.name is required", name)
	}

	// DisplayName 検証（UI表示用、空でも許容するがログで警告）
	if cc.DisplayName == "" {
		log.Printf("WARN: %s.display_name is empty, using name '%s' for display", name, cc.Name)
	}

	// LightMemory.MaxTurns 検証
	if cc.LightMemory.Enabled && (cc.LightMemory.MaxTurns < 1 || cc.LightMemory.MaxTurns > 20) {
		return fmt.Errorf("%s.light_memory.max_turns must be between 1 and 20, got %d", name, cc.LightMemory.MaxTurns)
	}

	// APIKey/BaseURL 検証（provider 別、Enabled=true の場合のみ）
	if cc.Enabled {
		switch cc.Provider {
		case "deepseek", "openai", "claude", "gemini":
			if cc.APIKey == "" {
				return fmt.Errorf("%s.api_key is required for provider '%s' when enabled=true", name, cc.Provider)
			}
		case "ollama", "local_openai":
			if cc.BaseURL == "" {
				return fmt.Errorf("%s.base_url is required for provider '%s' when enabled=true", name, cc.Provider)
			}
		}
	}

	return nil
}
