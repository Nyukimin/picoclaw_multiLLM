package config

import (
	"os"
	"path/filepath"
	"strings"
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
	if cfg.Security.NetworkScope != "" {
		t.Errorf("Expected Security NetworkScope '', got '%s'", cfg.Security.NetworkScope)
	}
	if cfg.AudioRouter.ConnectTimeoutMS != 5000 {
		t.Errorf("Expected AudioRouter ConnectTimeoutMS 5000, got %d", cfg.AudioRouter.ConnectTimeoutMS)
	}
	if cfg.AudioRouter.BufferMS != 120 {
		t.Errorf("Expected AudioRouter BufferMS 120, got %d", cfg.AudioRouter.BufferMS)
	}
}

func TestLoadConfig_ConversationL1SQLitePath(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "conversation_l1.yaml")

	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
session:
  storage_dir: "./data/sessions"
conversation:
  enabled: true
  redis_url: "redis://localhost:6379"
  l1_sqlite_path: "./data/l1_memory.db"
  duckdb_path: "./data/memory.duckdb"
  vectordb_url: "localhost:6334"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Conversation.L1SQLitePath != "./data/l1_memory.db" {
		t.Fatalf("unexpected l1 sqlite path: %s", cfg.Conversation.L1SQLitePath)
	}
}

func TestLoadConfig_LocalLLMDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "local_llm.yaml")

	content := `
server:
  port: 8080
session:
  storage_dir: "./data/sessions"
local_llm:
  enabled: true
  base_url: "http://127.0.0.1:8080"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.LocalLLM.Enabled {
		t.Fatal("expected local_llm enabled")
	}
	if cfg.LocalLLM.Provider != "local_openai" {
		t.Fatalf("unexpected local_llm provider: %s", cfg.LocalLLM.Provider)
	}
	if cfg.LocalLLM.ChatModel != "Chat" || cfg.LocalLLM.WorkerModel != "Worker" || cfg.LocalLLM.WildModel != "Wild" {
		t.Fatalf("unexpected model aliases: %+v", cfg.LocalLLM)
	}
	if cfg.LocalLLM.TimeoutSec != 120 {
		t.Fatalf("unexpected timeout_sec: %d", cfg.LocalLLM.TimeoutSec)
	}
	if cfg.LocalLLM.GlobalConcurrency != 2 || cfg.LocalLLM.ModelConcurrency != 1 {
		t.Fatalf("unexpected concurrency defaults: %+v", cfg.LocalLLM)
	}
	if !cfg.LocalLLMWarmupEnabled() {
		t.Fatal("expected local_llm warmup enabled by default")
	}
}

func TestLoadConfig_AudioRouterValidation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "audio_router.yaml")
	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
session:
  storage_dir: "./data/sessions"
audio_router:
  enabled: true
  sse_url: "http://127.0.0.1:18790/audio-router/events"
  device_map:
    mio:
      device_id: "{mio-device}"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.AudioRouter.DeviceMap["mio"].DeviceID != "{mio-device}" {
		t.Fatalf("unexpected mio device_id: %q", cfg.AudioRouter.DeviceMap["mio"].DeviceID)
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
  http_base_url: "https://127.0.0.1:8770"
  tls_skip_verify: true
  timeout_ms: 15000
  voice_id: "mio"
  provider_params:
    style: "Neutral"
    style_weight: 2.8
  provider_priority: ["irodori", "sbv2", "azure", "eleven"]
  playback_commands:
    - name: "ffplay"
      args: ["-autoexit", "{audio}"]
  irodori:
    enabled: true
    base_url: "http://127.0.0.1:7860"
    endpoint_path: "/api/tts"
    voice_id: "mio"
    timeout_sec: 120
    checkpoint: "Aratako/Irodori-TTS-500M-v2"
    model_device: "mps"
    codec_device: "mps"
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
	if got := cfg.TTS.Irodori.BaseURL; got != "http://127.0.0.1:7860" {
		t.Fatalf("unexpected irodori base url: %s", got)
	}
	if cfg.TTS.Irodori.EndpointPath != "/api/tts" || cfg.TTS.Irodori.Checkpoint != "Aratako/Irodori-TTS-500M-v2" || cfg.TTS.Irodori.NumSteps != 16 {
		t.Fatalf("unexpected irodori defaults: %+v", cfg.TTS.Irodori)
	}
	if len(cfg.TTS.PlaybackCommands) != 1 || cfg.TTS.PlaybackCommands[0].Name != "ffplay" {
		t.Fatalf("unexpected playback commands: %+v", cfg.TTS.PlaybackCommands)
	}
	if cfg.TTS.HTTPBaseURL == "" {
		t.Fatalf("expected tts http_base_url to be set")
	}
	if cfg.TTS.TimeoutMS != 15000 {
		t.Fatalf("unexpected tts timeout: %d", cfg.TTS.TimeoutMS)
	}
	if !cfg.TTS.TLSSkipVerify {
		t.Fatal("expected tts tls_skip_verify=true")
	}
	if cfg.TTS.ProviderParams["style"] != "Neutral" {
		t.Fatalf("unexpected provider_params: %+v", cfg.TTS.ProviderParams)
	}
}

func TestLoadConfig_STTSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stt.yaml")
	content := `
server:
  port: 8443
  tls:
    enabled: true
    cert_file: "./certs/dev.crt"
    key_file: "./certs/dev.key"
ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"
session:
  storage_dir: "./data/sessions"
stt:
  enabled: true
  provider: "external_http"
  language: "ja"
  model: "remote-stt"
  timeout_ms: 9000
  endpoint_path: "/stt"
  vad: true
  debug:
    save_audio: false
    save_transcript: true
  external_http:
    url: "http://127.0.0.1:8080/inference"
    stream_url: "wss://127.0.0.1:8443/stt/stream"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.Server.TLS.Enabled || cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
		t.Fatalf("expected TLS settings: %+v", cfg.Server.TLS)
	}
	if !cfg.STT.Enabled || cfg.STT.Provider != "external_http" || cfg.STT.Language != "ja" {
		t.Fatalf("unexpected stt config: %+v", cfg.STT)
	}
	if cfg.STT.TimeoutMS != 9000 || cfg.STT.ProviderURL != "http://127.0.0.1:8080/inference" {
		t.Fatalf("unexpected stt timeout/provider url: %+v", cfg.STT)
	}
	if cfg.STT.StreamURL != "wss://127.0.0.1:8443/stt/stream" {
		t.Fatalf("unexpected stt stream url: %+v", cfg.STT)
	}
}

func TestLoadConfig_STTProviderURLBackwardCompatibility(t *testing.T) {
	t.Setenv("STT_PROVIDER_URL", "http://127.0.0.1:8080/inference")
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "stt_env.yaml")
	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"
session:
  storage_dir: "./data/sessions"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.STT.Provider != "external_http" || cfg.STT.ProviderURL != "http://127.0.0.1:8080/inference" {
		t.Fatalf("expected STT_PROVIDER_URL compatibility, got %+v", cfg.STT)
	}
}

func TestLoadConfig_TTSLocalHTTPSDefaultsToTLSSkipVerify(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "tts_local_https.yaml")
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
  http_base_url: "https://127.0.0.1:8770"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.TTS.TLSSkipVerify {
		t.Fatal("expected tls_skip_verify to auto-enable for local https")
	}
}

func TestLoadConfig_VTuberSettings(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "vtuber.yaml")

	content := `
server:
  port: 8080
ollama:
  base_url: "http://localhost:11434"
session:
  storage_dir: "./data/sessions"
vtuber:
  enabled: true
  tick_interval_ms: 100
  characters:
    mio:
      audio_output: "Audio-Out-Mio"
      vts_host: "127.0.0.1"
      vts_port: 8001
      expression_map:
        happy: "ExpHappy"
        calm: "ExpCalm"
    shiro:
      audio_output: "Audio-Out-Shiro"
      vts_host: "127.0.0.1"
      vts_port: 8002
      expression_map:
        happy: "ExpHappy"
        calm: "ExpCalm"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.VTuber.Enabled {
		t.Fatalf("expected vtuber enabled")
	}
	if cfg.VTuber.TickIntervalMS != 100 {
		t.Fatalf("expected tick interval 100, got %d", cfg.VTuber.TickIntervalMS)
	}
	if cfg.VTuber.ConnectTimeout != 3000 {
		t.Fatalf("expected default connect timeout 3000, got %d", cfg.VTuber.ConnectTimeout)
	}
	if cfg.VTuber.Characters["mio"].VTSPort != 8001 {
		t.Fatalf("expected mio port 8001, got %d", cfg.VTuber.Characters["mio"].VTSPort)
	}
	if cfg.VTuber.Characters["shiro"].ExpressionMap["happy"] != "ExpHappy" {
		t.Fatalf("expected shiro happy expression mapping")
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
				ViewerLog: ViewerLogConfig{
					Enabled:           true,
					Path:              "./workspace/orchestrator_event_log.jsonl",
					RetentionDays:     14,
					GCIntervalMinutes: 60,
				},
				Coder1: CoderConfig{Name: "aka"},
				Coder2: CoderConfig{Name: "ao"},
				Coder3: CoderConfig{Name: "gin"},
				Coder4: CoderConfig{Name: "kin"},
			},
			wantErr: false,
		},
		{
			name: "Invalid security network_scope",
			config: &Config{
				Server:  ServerConfig{Port: 8080},
				Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				Security: SecurityConfig{
					Enabled:      true,
					PolicyMode:   "balanced",
					NetworkScope: "weird",
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
					Enabled:    true,
					PolicyMode: "dev",
					Audit: SecurityAuditConfig{
						Backend: "jsonl",
						Path:    "logs/execution_audit.jsonl",
					},
				},
				Coder1: CoderConfig{Name: "aka"},
				Coder2: CoderConfig{Name: "ao"},
				Coder3: CoderConfig{Name: "gin"},
				Coder4: CoderConfig{Name: "kin"},
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
			name: "Invalid viewer log retention",
			config: &Config{
				Server:  ServerConfig{Port: 8080},
				Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				ViewerLog: ViewerLogConfig{
					Enabled:           true,
					Path:              "./workspace/orchestrator_event_log.jsonl",
					RetentionDays:     0,
					GCIntervalMinutes: 60,
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
		{
			name: "Valid local LLM without Ollama",
			config: &Config{
				Server: ServerConfig{Port: 8080},
				LocalLLM: LocalLLMConfig{
					Enabled:           true,
					Provider:          "local_openai",
					BaseURL:           "http://127.0.0.1:8080",
					ChatModel:         "Chat",
					WorkerModel:       "Worker",
					WildModel:         "Wild",
					TimeoutSec:        120,
					GlobalConcurrency: 2,
					ModelConcurrency:  1,
				},
				Session: SessionConfig{StorageDir: "./data/sessions"},
				Coder1:  CoderConfig{Name: "aka"},
				Coder2:  CoderConfig{Name: "ao"},
				Coder3:  CoderConfig{Name: "gin"},
				Coder4:  CoderConfig{Name: "kin"},
			},
			wantErr: false,
		},
		{
			name: "Local LLM missing base URL",
			config: &Config{
				Server: ServerConfig{Port: 8080},
				LocalLLM: LocalLLMConfig{
					Enabled:           true,
					Provider:          "local_openai",
					ChatModel:         "Chat",
					WorkerModel:       "Worker",
					WildModel:         "Wild",
					TimeoutSec:        120,
					GlobalConcurrency: 2,
					ModelConcurrency:  1,
				},
				Session: SessionConfig{StorageDir: "./data/sessions"},
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
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
			Session: SessionConfig{StorageDir: "./data"},
		}
		// Coder1-4 の最小限の設定（バリデーションを通すため）
		cfg.Coder1.Name = "aka"
		cfg.Coder2.Name = "ao"
		cfg.Coder3.Name = "gin"
		cfg.Coder4.Name = "kin"
		return cfg
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
		cfg := &Config{
			Server:  ServerConfig{Port: 8080},
			Ollama:  OllamaConfig{BaseURL: "http://localhost:11434", Model: "picoclaw-v1"},
			Session: SessionConfig{StorageDir: "./data"},
		}
		// Coder1-4 の最小限の設定（バリデーションを通すため）
		cfg.Coder1.Name = "aka"
		cfg.Coder2.Name = "ao"
		cfg.Coder3.Name = "gin"
		cfg.Coder4.Name = "kin"
		return cfg
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
	if cfg.IdleChat.IntervalSec != 300 {
		t.Errorf("Expected IntervalSec 300, got %d", cfg.IdleChat.IntervalSec)
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

// TestGlossaryConfig_DefaultValues はGlossaryConfigのデフォルト値を検証
func TestGlossaryConfig_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Glossaryセクションを含まないミニマルな設定
	minimalContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"
`

	err := os.WriteFile(configPath, []byte(minimalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// デフォルト値の検証（config.go:457-472 の実装と一致）
	if cfg.Glossary.DBPath != "./workspace/glossary.db" {
		t.Errorf("Expected Glossary.DBPath './workspace/glossary.db', got '%s'", cfg.Glossary.DBPath)
	}

	if cfg.Glossary.RefreshIntervalHr != 6 {
		t.Errorf("Expected Glossary.RefreshIntervalHr 6, got %d", cfg.Glossary.RefreshIntervalHr)
	}

	if cfg.Glossary.MaxEntries != 8 {
		t.Errorf("Expected Glossary.MaxEntries 8, got %d", cfg.Glossary.MaxEntries)
	}

	if len(cfg.Glossary.FeedURLs) != 3 {
		t.Errorf("Expected 3 default FeedURLs, got %d", len(cfg.Glossary.FeedURLs))
	}

	expectedFeeds := []string{
		"https://www3.nhk.or.jp/rss/news/cat0.xml",
		"https://feeds.bbci.co.uk/news/world/rss.xml",
		"https://feeds.bbci.co.uk/news/technology/rss.xml",
	}

	for i, expectedURL := range expectedFeeds {
		if i >= len(cfg.Glossary.FeedURLs) {
			t.Errorf("FeedURLs[%d] is missing", i)
			continue
		}
		if cfg.Glossary.FeedURLs[i] != expectedURL {
			t.Errorf("FeedURLs[%d]: expected '%s', got '%s'", i, expectedURL, cfg.Glossary.FeedURLs[i])
		}
	}
}

// TestGlossaryConfig_CustomValues はカスタム値が正しく読み込まれることを検証
func TestGlossaryConfig_CustomValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	customContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"

glossary:
  enabled: true
  db_path: "/custom/path/glossary.db"
  refresh_interval_hr: 12
  max_entries: 20
  feed_urls:
    - "https://custom.feed/rss"
`

	err := os.WriteFile(configPath, []byte(customContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// カスタム値の検証
	if !cfg.Glossary.Enabled {
		t.Error("Expected Glossary.Enabled true")
	}

	if cfg.Glossary.DBPath != "/custom/path/glossary.db" {
		t.Errorf("Expected custom DBPath, got '%s'", cfg.Glossary.DBPath)
	}

	if cfg.Glossary.RefreshIntervalHr != 12 {
		t.Errorf("Expected RefreshIntervalHr 12, got %d", cfg.Glossary.RefreshIntervalHr)
	}

	if cfg.Glossary.MaxEntries != 20 {
		t.Errorf("Expected MaxEntries 20, got %d", cfg.Glossary.MaxEntries)
	}

	if len(cfg.Glossary.FeedURLs) != 1 {
		t.Errorf("Expected 1 custom FeedURL, got %d", len(cfg.Glossary.FeedURLs))
	}

	if len(cfg.Glossary.FeedURLs) > 0 && cfg.Glossary.FeedURLs[0] != "https://custom.feed/rss" {
		t.Errorf("Expected custom feed URL, got '%s'", cfg.Glossary.FeedURLs[0])
	}
}

// TestCoderConfig_DefaultValues は Coder1-4 のデフォルト値を検証
func TestCoderConfig_DefaultValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Coder セクションを含まないミニマルな設定
	minimalContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"
`

	err := os.WriteFile(configPath, []byte(minimalContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Coder1 デフォルト値検証
	if cfg.Coder1.Provider != "deepseek" {
		t.Errorf("Coder1.Provider: expected 'deepseek', got '%s'", cfg.Coder1.Provider)
	}
	if cfg.Coder1.Model != "deepseek-coder" {
		t.Errorf("Coder1.Model: expected 'deepseek-coder', got '%s'", cfg.Coder1.Model)
	}
	if cfg.Coder1.Name != "aka" {
		t.Errorf("Coder1.Name: expected 'aka', got '%s'", cfg.Coder1.Name)
	}
	if cfg.Coder1.DisplayName != "赤" {
		t.Errorf("Coder1.DisplayName: expected '赤', got '%s'", cfg.Coder1.DisplayName)
	}
	if cfg.Coder1.LightMemory.MaxTurns != 3 {
		t.Errorf("Coder1.LightMemory.MaxTurns: expected 3, got %d", cfg.Coder1.LightMemory.MaxTurns)
	}

	// Coder2 デフォルト値検証
	if cfg.Coder2.Provider != "openai" {
		t.Errorf("Coder2.Provider: expected 'openai', got '%s'", cfg.Coder2.Provider)
	}
	if cfg.Coder2.Model != "gpt-4-turbo" {
		t.Errorf("Coder2.Model: expected 'gpt-4-turbo', got '%s'", cfg.Coder2.Model)
	}
	if cfg.Coder2.Name != "ao" {
		t.Errorf("Coder2.Name: expected 'ao', got '%s'", cfg.Coder2.Name)
	}
	if cfg.Coder2.DisplayName != "青" {
		t.Errorf("Coder2.DisplayName: expected '青', got '%s'", cfg.Coder2.DisplayName)
	}

	// Coder3 デフォルト値検証
	if cfg.Coder3.Provider != "claude" {
		t.Errorf("Coder3.Provider: expected 'claude', got '%s'", cfg.Coder3.Provider)
	}
	if cfg.Coder3.Model != "claude-sonnet-4" {
		t.Errorf("Coder3.Model: expected 'claude-sonnet-4', got '%s'", cfg.Coder3.Model)
	}
	if cfg.Coder3.Name != "gin" {
		t.Errorf("Coder3.Name: expected 'gin', got '%s'", cfg.Coder3.Name)
	}
	if cfg.Coder3.DisplayName != "銀" {
		t.Errorf("Coder3.DisplayName: expected '銀', got '%s'", cfg.Coder3.DisplayName)
	}

	// Coder4 デフォルト値検証
	if cfg.Coder4.Provider != "gemini" {
		t.Errorf("Coder4.Provider: expected 'gemini', got '%s'", cfg.Coder4.Provider)
	}
	if cfg.Coder4.Model != "gemini-2.0-flash-exp" {
		t.Errorf("Coder4.Model: expected 'gemini-2.0-flash-exp', got '%s'", cfg.Coder4.Model)
	}
	if cfg.Coder4.Name != "kin" {
		t.Errorf("Coder4.Name: expected 'kin', got '%s'", cfg.Coder4.Name)
	}
	if cfg.Coder4.DisplayName != "金" {
		t.Errorf("Coder4.DisplayName: expected '金', got '%s'", cfg.Coder4.DisplayName)
	}
}

// TestCoderConfig_CustomValues はカスタム値が正しく読み込まれることを検証
func TestCoderConfig_CustomValues(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	customContent := `
server:
  port: 8080

ollama:
  base_url: "http://localhost:11434"
  model: "picoclaw-v1"

session:
  storage_dir: "./data/sessions"

coder1:
  name: "custom_aka"
  display_name: "カスタム赤"
  provider: "deepseek"
  model: "deepseek-custom"
  api_key: "test-key-1"
  personality: "あなたはカスタム赤。設計思考が得意。"
  tone: "analytical"
  light_memory:
    enabled: true
    max_turns: 5
  enabled: true

coder4:
  name: "custom_kin"
  display_name: "カスタム金"
  provider: "gemini"
  model: "gemini-pro"
  api_key: "test-key-4"
  personality: "あなたはカスタム金。"
  tone: "fast"
  light_memory:
    enabled: true
    max_turns: 10
  enabled: true
`

	err := os.WriteFile(configPath, []byte(customContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Coder1 カスタム値検証
	if cfg.Coder1.Name != "custom_aka" {
		t.Errorf("Coder1.Name: expected 'custom_aka', got '%s'", cfg.Coder1.Name)
	}
	if cfg.Coder1.DisplayName != "カスタム赤" {
		t.Errorf("Coder1.DisplayName: expected 'カスタム赤', got '%s'", cfg.Coder1.DisplayName)
	}
	if cfg.Coder1.Model != "deepseek-custom" {
		t.Errorf("Coder1.Model: expected 'deepseek-custom', got '%s'", cfg.Coder1.Model)
	}
	if cfg.Coder1.APIKey != "test-key-1" {
		t.Errorf("Coder1.APIKey: expected 'test-key-1', got '%s'", cfg.Coder1.APIKey)
	}
	if cfg.Coder1.Personality != "あなたはカスタム赤。設計思考が得意。" {
		t.Errorf("Coder1.Personality: unexpected value '%s'", cfg.Coder1.Personality)
	}
	if cfg.Coder1.Tone != "analytical" {
		t.Errorf("Coder1.Tone: expected 'analytical', got '%s'", cfg.Coder1.Tone)
	}
	if !cfg.Coder1.LightMemory.Enabled {
		t.Error("Coder1.LightMemory.Enabled: expected true")
	}
	if cfg.Coder1.LightMemory.MaxTurns != 5 {
		t.Errorf("Coder1.LightMemory.MaxTurns: expected 5, got %d", cfg.Coder1.LightMemory.MaxTurns)
	}
	if !cfg.Coder1.Enabled {
		t.Error("Coder1.Enabled: expected true")
	}

	// Coder4 カスタム値検証
	if cfg.Coder4.Name != "custom_kin" {
		t.Errorf("Coder4.Name: expected 'custom_kin', got '%s'", cfg.Coder4.Name)
	}
	if cfg.Coder4.DisplayName != "カスタム金" {
		t.Errorf("Coder4.DisplayName: expected 'カスタム金', got '%s'", cfg.Coder4.DisplayName)
	}
	if cfg.Coder4.Provider != "gemini" {
		t.Errorf("Coder4.Provider: expected 'gemini', got '%s'", cfg.Coder4.Provider)
	}
	if cfg.Coder4.Model != "gemini-pro" {
		t.Errorf("Coder4.Model: expected 'gemini-pro', got '%s'", cfg.Coder4.Model)
	}
	if cfg.Coder4.APIKey != "test-key-4" {
		t.Errorf("Coder4.APIKey: expected 'test-key-4', got '%s'", cfg.Coder4.APIKey)
	}
	if cfg.Coder4.LightMemory.MaxTurns != 10 {
		t.Errorf("Coder4.LightMemory.MaxTurns: expected 10, got %d", cfg.Coder4.LightMemory.MaxTurns)
	}
}

// TestValidateCoderConfig は validateCoderConfig() 関数を直接テスト
func TestValidateCoderConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  CoderConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "有効な設定",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "test-key",
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "無効な provider",
			config: CoderConfig{
				Name:     "test",
				Provider: "invalid-provider",
				Model:    "test-model",
				Enabled:  true,
			},
			wantErr: true,
			errMsg:  "provider must be one of",
		},
		{
			name: "name が空",
			config: CoderConfig{
				Name:     "",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "test-key",
				Enabled:  true,
			},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name: "enabled=true だが model が空",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "",
				APIKey:   "test-key",
				Enabled:  true,
			},
			wantErr: true,
			errMsg:  "model is required",
		},
		{
			name: "enabled=true だが api_key が空（deepseek）",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "",
				Enabled:  true,
			},
			wantErr: true,
			errMsg:  "api_key is required",
		},
		{
			name: "enabled=true だが base_url が空（ollama）",
			config: CoderConfig{
				Name:     "test",
				Provider: "ollama",
				Model:    "test-model",
				BaseURL:  "",
				Enabled:  true,
			},
			wantErr: true,
			errMsg:  "base_url is required",
		},
		{
			name: "light_memory max_turns が範囲外（大きすぎ）",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "test-key",
				LightMemory: LightMemoryConfig{
					Enabled:  true,
					MaxTurns: 100,
				},
				Enabled: true,
			},
			wantErr: true,
			errMsg:  "max_turns must be between 1 and 20",
		},
		{
			name: "light_memory max_turns が範囲外（0）",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "test-key",
				LightMemory: LightMemoryConfig{
					Enabled:  true,
					MaxTurns: 0,
				},
				Enabled: true,
			},
			wantErr: true,
			errMsg:  "max_turns must be between 1 and 20",
		},
		{
			name: "enabled=false の場合は api_key なしでも OK",
			config: CoderConfig{
				Name:     "test",
				Provider: "deepseek",
				Model:    "test-model",
				APIKey:   "",
				Enabled:  false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCoderConfig("test_coder", &tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}
