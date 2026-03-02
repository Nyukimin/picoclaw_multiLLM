package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_Success(t *testing.T) {
	// テスト用の設定ファイルを作成
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 8080
  host: "0.0.0.0"

ollama:
  base_url: "http://localhost:11434"
  chat_model: "chat-v1"
  worker_model: "worker-v1"

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

	if cfg.Session.StorageDir != "./data/sessions" {
		t.Errorf("Expected session storage dir, got '%s'", cfg.Session.StorageDir)
	}
}

func TestLoadConfig_WithEnvVars(t *testing.T) {
	// 環境変数を設定
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
  chat_model: "chat-v1"
  worker_model: "worker-v1"

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

	// 環境変数から読み込まれるべき
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

	// デフォルト値の確認
	if cfg.Ollama.ChatModel == "" {
		t.Error("ChatModel should have default value")
	}

	if cfg.Log.Level == "" {
		t.Error("Log level should have default value")
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
					BaseURL:     "http://localhost:11434",
					ChatModel:   "chat-v1",
					WorkerModel: "worker-v1",
				},
				Session: SessionConfig{
					StorageDir: "./data/sessions",
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
			name: "Missing session storage dir",
			config: &Config{
				Server: ServerConfig{
					Port: 8080,
				},
				Ollama: OllamaConfig{
					BaseURL: "http://localhost:11434",
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
