package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
  host: "0.0.0.0"

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"

log:
  level: "info"
  format: "json"
`

	err := os.WriteFile(configPath, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected host '0.0.0.0', got '%s'", cfg.Server.Host)
	}

	if cfg.Ollama.BaseURL != "http://localhost:11434" {
		t.Errorf("Expected Ollama base URL, got '%s'", cfg.Ollama.BaseURL)
	}

	if cfg.Ollama.Model != "picoclaw-v1" {
		t.Errorf("Expected Ollama model 'picoclaw-v1', got '%s'", cfg.Ollama.Model)
	}

	if cfg.Session.StorageDir != "./data/sessions" {
		t.Errorf("Expected session storage dir, got '%s'", cfg.Session.StorageDir)
	}
}

func TestLoadConfig_V3CompatModel(t *testing.T) {
	// v3形式のchat_model/worker_modelでも動作することを確認
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  chat_model: "chat-v1"
  worker_model: "worker-v1"

session:
  storage_dir: "./data/sessions"
`

	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig with v3 compat fields failed: %v", err)
	}

	// chat_model が Model にマッピングされるべき
	if cfg.Ollama.Model != "chat-v1" {
		t.Errorf("Expected Model to be mapped from ChatModel 'chat-v1', got '%s'", cfg.Ollama.Model)
	}
}

func TestLoadConfig_WithEnvVars(t *testing.T) {
	os.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	os.Setenv("DEEPSEEK_API_KEY", "test-deepseek-key")
	os.Setenv("OPENAI_API_KEY", "test-openai-key")
	defer func() {
		os.Unsetenv("ANTHROPIC_API_KEY")
		os.Unsetenv("DEEPSEEK_API_KEY")
		os.Unsetenv("OPENAI_API_KEY")
	}()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

claude:
  api_key: "${ANTHROPIC_API_KEY}"

deepseek:
  api_key: "${DEEPSEEK_API_KEY}"

openai:
  api_key: "${OPENAI_API_KEY}"

session:
  storage_dir: "./data/sessions"

log:
  level: "info"
`

	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Claude.APIKey != "test-anthropic-key" {
		t.Errorf("Expected Anthropic API key from env, got '%s'", cfg.Claude.APIKey)
	}

	if cfg.DeepSeek.APIKey != "test-deepseek-key" {
		t.Errorf("Expected DeepSeek API key from env, got '%s'", cfg.DeepSeek.APIKey)
	}

	if cfg.OpenAI.APIKey != "test-openai-key" {
		t.Errorf("Expected OpenAI API key from env, got '%s'", cfg.OpenAI.APIKey)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("Expected error for non-existent config file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	invalidContent := `
server:
  port: invalid_port
  host: "localhost"
invalid yaml content here
`

	os.WriteFile(configPath, []byte(invalidContent), 0644)

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected error for invalid YAML")
	}
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")

	minimalContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"

session:
  storage_dir: "./data/sessions"
`

	os.WriteFile(configPath, []byte(minimalContent), 0644)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Ollamaモデルデフォルト
	if cfg.Ollama.Model != "picoclaw-v1" {
		t.Errorf("Expected Ollama model 'picoclaw-v1', got '%s'", cfg.Ollama.Model)
	}

	if cfg.Log.Level == "" {
		t.Error("Log level should have default value")
	}

	// Worker設定デフォルト値の確認
	if cfg.Worker.CommitMessagePrefix != "[Worker Auto-Commit]" {
		t.Errorf("Expected Worker CommitMessagePrefix '[Worker Auto-Commit]', got '%s'", cfg.Worker.CommitMessagePrefix)
	}

	if cfg.Worker.CommandTimeout != 300 {
		t.Errorf("Expected Worker CommandTimeout 300, got %d", cfg.Worker.CommandTimeout)
	}

	if cfg.Worker.GitTimeout != 30 {
		t.Errorf("Expected Worker GitTimeout 30, got %d", cfg.Worker.GitTimeout)
	}

	if len(cfg.Worker.ProtectedPatterns) != 4 {
		t.Errorf("Expected 4 protected patterns, got %d", len(cfg.Worker.ProtectedPatterns))
	}

	if cfg.Worker.ActionOnProtected != "error" {
		t.Errorf("Expected Worker ActionOnProtected 'error', got '%s'", cfg.Worker.ActionOnProtected)
	}

	if cfg.Worker.Workspace != "." {
		t.Errorf("Expected Worker Workspace '.', got '%s'", cfg.Worker.Workspace)
	}

	// v4デフォルト
	if cfg.Worker.MaxParallelism != 4 {
		t.Errorf("Expected Worker MaxParallelism 4, got %d", cfg.Worker.MaxParallelism)
	}

	// Distributed/IdleChat はデフォルトで無効
	if cfg.Distributed.Enabled {
		t.Error("Distributed should be disabled by default")
	}
	if cfg.IdleChat.Enabled {
		t.Error("IdleChat should be disabled by default")
	}

	if cfg.Security.PolicyMode != "balanced" {
		t.Errorf("Expected Security PolicyMode 'balanced', got '%s'", cfg.Security.PolicyMode)
	}
	if cfg.Security.ApprovalMode != "never" {
		t.Errorf("Expected Security ApprovalMode 'never', got '%s'", cfg.Security.ApprovalMode)
	}
	if cfg.Security.ApprovalTTLMinutes != 10 {
		t.Errorf("Expected Security ApprovalTTLMinutes 10, got %d", cfg.Security.ApprovalTTLMinutes)
	}
	if cfg.Security.NetworkScope != "" {
		t.Errorf("Expected Security NetworkScope '', got '%s'", cfg.Security.NetworkScope)
	}
}

func TestLoadConfig_SecurityNetworkSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "network.yaml")

	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"
session:
  storage_dir: "./data/sessions"
security:
  enabled: true
  policy_mode: "strict"
  approval_mode: "on_demand"
  approval_ttl_minutes: 10
  network_scope: "allowlist"
  network_allowlist:
    - "api.openai.com"
  audit:
    backend: "jsonl"
    path: "logs/execution_audit.jsonl"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Security.NetworkScope != "allowlist" {
		t.Fatalf("expected network_scope allowlist, got %s", cfg.Security.NetworkScope)
	}
	if len(cfg.Security.NetworkAllowlist) != 1 || cfg.Security.NetworkAllowlist[0] != "api.openai.com" {
		t.Fatalf("unexpected network_allowlist: %+v", cfg.Security.NetworkAllowlist)
	}
}

func TestLoadConfig_TTSSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tts.yaml")
	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"
session:
  storage_dir: "./data/sessions"
tts:
  enabled: true
  output_dir: "./workspace/tts"
  http_base_url: "http://127.0.0.1:8765"
  ws_url: "ws://127.0.0.1:8765/sessions"
  connect_timeout_ms: 3000
  receive_timeout_ms: 15000
  chunk_gap_timeout_ms: 3000
  voice_id: "female_01"
  speech_mode: "conversational"
  provider_priority: ["sbv2", "azure", "eleven"]
  playback_commands:
    - name: "ffplay"
      args: ["-autoexit", "{audio}"]
  sbv2:
    enabled: true
    base_url: "http://127.0.0.1:5000/synthesis"
    voice_id: "mio"
    timeout_sec: 20
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.TTS.Enabled {
		t.Fatal("expected tts enabled")
	}
	if got := cfg.TTS.SBV2.BaseURL; got != "http://127.0.0.1:5000/synthesis" {
		t.Fatalf("unexpected sbv2 base url: %s", got)
	}
	if len(cfg.TTS.PlaybackCommands) != 1 || cfg.TTS.PlaybackCommands[0].Name != "ffplay" {
		t.Fatalf("unexpected playback commands: %+v", cfg.TTS.PlaybackCommands)
	}
	if cfg.TTS.WSURL == "" || cfg.TTS.HTTPBaseURL == "" {
		t.Fatalf("expected tts client urls to be set, got http=%q ws=%q", cfg.TTS.HTTPBaseURL, cfg.TTS.WSURL)
	}
	if cfg.TTS.ConnectTimeoutMS != 3000 || cfg.TTS.ReceiveTimeoutMS != 15000 || cfg.TTS.ChunkGapTimeoutMS != 3000 {
		t.Fatalf("unexpected tts client timeouts: %+v", cfg.TTS)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "Valid config",
			config: &Config{
				Server: ServerConfig{
					Port: 8080,
					Host: "0.0.0.0",
				},
				Ollama: OllamaConfig{
					BaseURL: "http://localhost:11434",
					Model:   "picoclaw-v1",
				},
				Session: SessionConfig{
					StorageDir: "./data/sessions",
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid security approval_mode",
			config: &Config{
				Server:  ServerConfig{Port: 8080},
				Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				Security: SecurityConfig{
					Enabled:      true,
					PolicyMode:   "balanced",
					ApprovalMode: "invalid",
					Audit: SecurityAuditConfig{
						Backend: "jsonl",
						Path:    "logs/execution_audit.jsonl",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid security network_scope",
			config: &Config{
				Server:  ServerConfig{Port: 8080},
				Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				Security: SecurityConfig{
					Enabled:            true,
					PolicyMode:         "balanced",
					ApprovalMode:       "never",
					ApprovalTTLMinutes: 10,
					NetworkScope:       "weird",
					Audit: SecurityAuditConfig{
						Backend: "jsonl",
						Path:    "logs/execution_audit.jsonl",
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid security policy_mode dev",
			config: &Config{
				Server:  ServerConfig{Port: 8080},
				Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				Security: SecurityConfig{
					Enabled:            true,
					PolicyMode:         "dev",
					ApprovalMode:       "never",
					ApprovalTTLMinutes: 10,
					Audit: SecurityAuditConfig{
						Backend: "jsonl",
						Path:    "logs/execution_audit.jsonl",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Invalid port (too low)",
			config: &Config{
				Server: ServerConfig{
					Port: 0,
				},
			},
			wantErr: true,
		},
		{
			name: "Invalid port (too high)",
			config: &Config{
				Server: ServerConfig{
					Port: 70000,
				},
			},
			wantErr: true,
		},
		{
			name: "Missing Ollama base URL",
			config: &Config{
				Server: ServerConfig{
					Port: 8080,
				},
				Ollama: OllamaConfig{
					BaseURL: "",
				},
			},
			wantErr: true,
		},
		{
			name: "Missing Ollama model",
			config: &Config{
				Server: ServerConfig{
					Port: 8080,
				},
				Ollama: OllamaConfig{
					BaseURL: "http://localhost:11434",
					Model:   "",
				},
			},
			wantErr: true,
		},
		{
			name: "Missing session storage dir",
			config: &Config{
				Server: ServerConfig{
					Port: 8080,
				},
				Ollama: OllamaConfig{
					BaseURL: "http://localhost:11434",
					Model:   "picoclaw-v1",
				},
				Session: SessionConfig{
					StorageDir: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Validate_Distributed(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:  ServerConfig{Port: 8080},
			Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
			Session: SessionConfig{StorageDir: "./data"},
		}
	}

	t.Run("Distributed enabled without transports", func(t *testing.T) {
		cfg := base()
		cfg.Distributed.Enabled = true
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for distributed without transports")
		}
	})

	t.Run("Distributed with invalid transport type", func(t *testing.T) {
		cfg := base()
		cfg.Distributed.Enabled = true
		cfg.Distributed.Transports = map[string]TransportConfig{
			"mio": {Type: "invalid"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for invalid transport type")
		}
	})

	t.Run("Distributed SSH missing remote_host", func(t *testing.T) {
		cfg := base()
		cfg.Distributed.Enabled = true
		cfg.Distributed.Transports = map[string]TransportConfig{
			"coder3": {Type: "ssh", RemoteUser: "picoclaw", SSHKeyPath: "/path"},
		}
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for SSH missing remote_host")
		}
	})

	t.Run("Distributed valid config", func(t *testing.T) {
		cfg := base()
		cfg.Distributed.Enabled = true
		cfg.Distributed.Transports = map[string]TransportConfig{
			"mio":    {Type: "local"},
			"coder3": {Type: "ssh", RemoteHost: "192.168.1.100:22", RemoteUser: "picoclaw", SSHKeyPath: "/path"},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Expected valid config, got error: %v", err)
		}
	})
}

func TestConfig_Validate_IdleChat(t *testing.T) {
	base := func() *Config {
		return &Config{
			Server:  ServerConfig{Port: 8080},
			Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
			Session: SessionConfig{StorageDir: "./data"},
		}
	}

	t.Run("IdleChat with unknown agent", func(t *testing.T) {
		cfg := base()
		cfg.IdleChat.Enabled = true
		cfg.IdleChat.Participants = []string{"mio", "Unknown"}
		cfg.IdleChat.IntervalMin = 5
		cfg.IdleChat.MaxTurns = 10
		cfg.IdleChat.Temperature = 0.8
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for unknown agent")
		}
	})

	t.Run("IdleChat with invalid max_turns", func(t *testing.T) {
		cfg := base()
		cfg.IdleChat.Enabled = true
		cfg.IdleChat.Participants = []string{"mio", "shiro"}
		cfg.IdleChat.IntervalMin = 5
		cfg.IdleChat.MaxTurns = 200
		cfg.IdleChat.Temperature = 0.8
		if err := cfg.Validate(); err == nil {
			t.Error("Expected error for max_turns > 100")
		}
	})

	t.Run("IdleChat valid config", func(t *testing.T) {
		cfg := base()
		cfg.IdleChat.Enabled = true
		cfg.IdleChat.Participants = []string{"mio", "shiro"}
		cfg.IdleChat.IntervalMin = 5
		cfg.IdleChat.MaxTurns = 10
		cfg.IdleChat.Temperature = 0.8
		if err := cfg.Validate(); err != nil {
			t.Errorf("Expected valid config, got error: %v", err)
		}
	})
}

func TestLoadConfig_IdleChatDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"

idle_chat:
  enabled: true
`

	os.WriteFile(configPath, []byte(configContent), 0644)

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.IdleChat.Participants) != 2 {
		t.Errorf("Expected 2 default participants, got %d", len(cfg.IdleChat.Participants))
	}

	if cfg.IdleChat.IntervalMin != 5 {
		t.Errorf("Expected IntervalMin 5, got %d", cfg.IdleChat.IntervalMin)
	}

	if cfg.IdleChat.MaxTurns != 10 {
		t.Errorf("Expected MaxTurns 10, got %d", cfg.IdleChat.MaxTurns)
	}

	if cfg.IdleChat.Temperature != 0.8 {
		t.Errorf("Expected Temperature 0.8, got %f", cfg.IdleChat.Temperature)
	}
}

func TestConversationConfig_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
  model: "chat-v1"
session:
  storage_dir: "./data"
`
	os.WriteFile(configPath, []byte(configContent), 0644)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// デフォルト値確認
	if cfg.Conversation.RedisURL != "redis://localhost:6379" {
		t.Errorf("unexpected RedisURL: %s", cfg.Conversation.RedisURL)
	}
	if cfg.Conversation.VectorDBURL != "localhost:6334" {
		t.Errorf("unexpected VectorDBURL: %s", cfg.Conversation.VectorDBURL)
	}
}

func TestConversationConfig_EmbedAndSummaryModel(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
  model: "chat-v1"
session:
  storage_dir: "./data"
conversation:
  enabled: true
  redis_url: "redis://localhost:6379"
  vectordb_url: "localhost:6334"
  embed_model: "nomic-embed-text"
  summary_model: "chat-v1"
`
	os.WriteFile(configPath, []byte(configContent), 0644)
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Conversation.EmbedModel != "nomic-embed-text" {
		t.Errorf("expected EmbedModel 'nomic-embed-text', got %q", cfg.Conversation.EmbedModel)
	}
	if cfg.Conversation.SummaryModel != "chat-v1" {
		t.Errorf("expected SummaryModel 'chat-v1', got %q", cfg.Conversation.SummaryModel)
	}
}
