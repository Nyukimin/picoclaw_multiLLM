package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション全体の設定
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Ollama   OllamaConfig   `yaml:"ollama"`
	Claude   ClaudeConfig   `yaml:"claude"`
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
	Session  SessionConfig  `yaml:"session"`
	Log      LogConfig      `yaml:"log"`
}

// ServerConfig はサーバー設定
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// OllamaConfig はOllama設定
type OllamaConfig struct {
	BaseURL     string `yaml:"base_url"`
	ChatModel   string `yaml:"chat_model"`
	WorkerModel string `yaml:"worker_model"`
}

// ClaudeConfig はClaude API設定
type ClaudeConfig struct {
	APIKey string `yaml:"api_key"` // 環境変数から読み込み推奨
	Model  string `yaml:"model"`
}

// DeepSeekConfig はDeepSeek API設定
type DeepSeekConfig struct {
	APIKey string `yaml:"api_key"` // 環境変数から読み込み推奨
	Model  string `yaml:"model"`
}

// OpenAIConfig はOpenAI API設定
type OpenAIConfig struct {
	APIKey string `yaml:"api_key"` // 環境変数から読み込み推奨
	Model  string `yaml:"model"`
}

// SessionConfig はセッション設定
type SessionConfig struct {
	StorageDir string `yaml:"storage_dir"`
}

// LogConfig はログ設定
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// LoadConfig は設定ファイルを読み込む
func LoadConfig(path string) (*Config, error) {
	// ファイル読み込み
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// YAMLパース
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// デフォルト値設定
	cfg.setDefaults()

	// 環境変数から機密情報を読み込み
	cfg.loadFromEnv()

	// バリデーション
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

// setDefaults はデフォルト値を設定
func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}

	if c.Ollama.ChatModel == "" {
		c.Ollama.ChatModel = "chat-v1"
	}

	if c.Ollama.WorkerModel == "" {
		c.Ollama.WorkerModel = "worker-v1"
	}

	if c.Claude.Model == "" {
		c.Claude.Model = "claude-sonnet-4-20250514"
	}

	if c.DeepSeek.Model == "" {
		c.DeepSeek.Model = "deepseek-chat"
	}

	if c.OpenAI.Model == "" {
		c.OpenAI.Model = "gpt-4o-mini"
	}

	if c.Log.Level == "" {
		c.Log.Level = "info"
	}

	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
}

// loadFromEnv は環境変数から設定を読み込み
func (c *Config) loadFromEnv() {
	// API キーは環境変数から読み込み（ファイルに平文保存しない）
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		c.Claude.APIKey = apiKey
	}

	if apiKey := os.Getenv("DEEPSEEK_API_KEY"); apiKey != "" {
		c.DeepSeek.APIKey = apiKey
	}

	if apiKey := os.Getenv("OPENAI_API_KEY"); apiKey != "" {
		c.OpenAI.APIKey = apiKey
	}
}

// Validate は設定の妥当性を検証
func (c *Config) Validate() error {
	// サーバー設定検証
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d (must be 1-65535)", c.Server.Port)
	}

	// Ollama設定検証
	if c.Ollama.BaseURL == "" {
		return fmt.Errorf("ollama base_url is required")
	}

	if c.Ollama.ChatModel == "" {
		return fmt.Errorf("ollama chat_model is required")
	}

	if c.Ollama.WorkerModel == "" {
		return fmt.Errorf("ollama worker_model is required")
	}

	// セッション設定検証
	if c.Session.StorageDir == "" {
		return fmt.Errorf("session storage_dir is required")
	}

	return nil
}
