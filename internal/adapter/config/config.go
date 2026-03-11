package config

import (
	"fmt"
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション全体の設定
// v3既存フィールドをそのまま維持し、v4.0で Distributed, IdleChat を追加
type Config struct {
	// === v3.0 既存フィールド ===
	Server   ServerConfig   `yaml:"server"`
	Ollama   OllamaConfig   `yaml:"ollama"`
	Claude   ClaudeConfig   `yaml:"claude"`
	DeepSeek DeepSeekConfig `yaml:"deepseek"`
	OpenAI   OpenAIConfig   `yaml:"openai"`
	Session  SessionConfig  `yaml:"session"`
	Worker   WorkerConfig   `yaml:"worker"`
	Line     LineConfig     `yaml:"line"`
	Telegram TelegramConfig `yaml:"telegram"`
	Discord  DiscordConfig  `yaml:"discord"`
	Slack    SlackConfig    `yaml:"slack"`
	Log      LogConfig      `yaml:"log"`

	// === v4.0 追加フィールド ===
	Distributed DistributedConfig `yaml:"distributed"`
	IdleChat    IdleChatConfig    `yaml:"idle_chat"`

	// === v5.0 追加フィールド ===
	Conversation ConversationConfig `yaml:"conversation"`

	// === v5.1 プロンプト外部ファイル ===
	PromptsDir   string         `yaml:"prompts_dir"`   // プロンプトファイルのベースディレクトリ（デフォルト）
	WorkspaceDir string         `yaml:"workspace_dir"` // ユーザーカスタマイズ領域（オーバーライド）
	Prompts      *LoadedPrompts `yaml:"-"`             // 読み込み済みプロンプト（YAML非対象）

	// === Heartbeat ===
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// === Google Search API ===
	GoogleSearchChat   GoogleSearchConfig `yaml:"google_search_chat"`
	GoogleSearchWorker GoogleSearchConfig `yaml:"google_search_worker"`

	// === Subagent ===
	Subagent SubagentConfig `yaml:"subagent"`

	// === Security / Execution Audit ===
	Security SecurityConfig `yaml:"security"`

	// === TTS / OpenClaw parity ===
	TTS TTSConfig `yaml:"tts"`
}

// ServerConfig はサーバー設定
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// OllamaConfig はOllama設定
// v4.0で chat_model/worker_model を統合し、単一の Model に変更
// 全Agent（mio/shiro/IdleChat参加Agent）が同一モデルを共用する
type OllamaConfig struct {
	BaseURL    string `yaml:"base_url"`
	Model      string `yaml:"model"`       // v4: 共通モデル（例: "picoclaw-v1"）
	MaxContext int    `yaml:"max_context"` // 常駐モデルの最大コンテキスト長（超過はNG）

	// v3後方互換（deprecated: Model に統合済み）
	ChatModel   string `yaml:"chat_model,omitempty"`
	WorkerModel string `yaml:"worker_model,omitempty"`
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

// WorkerConfig はWorker実行設定
type WorkerConfig struct {
	// === v3.0 既存フィールド ===
	AutoCommit           bool     `yaml:"auto_commit"`
	CommitMessagePrefix  string   `yaml:"commit_message_prefix"`
	CommandTimeout       int      `yaml:"command_timeout"` // 秒
	GitTimeout           int      `yaml:"git_timeout"`     // 秒
	StopOnError          bool     `yaml:"stop_on_error"`
	Workspace            string   `yaml:"workspace"`
	ProtectedPatterns    []string `yaml:"protected_patterns"`
	ActionOnProtected    string   `yaml:"action_on_protected"` // "error", "skip", "log"
	ShowExecutionSummary bool     `yaml:"show_execution_summary"`

	// === v4.0 追加フィールド ===
	ParallelExecution bool `yaml:"parallel_execution"` // true で並列実行（デフォルト: false）
	MaxParallelism    int  `yaml:"max_parallelism"`    // 並列度上限（デフォルト: 4）
}

// LineConfig はLINE Messaging API設定
type LineConfig struct {
	ChannelSecret string `yaml:"channel_secret"` // 環境変数 LINE_CHANNEL_SECRET 推奨
	AccessToken   string `yaml:"access_token"`   // 環境変数 LINE_CHANNEL_TOKEN 推奨
}

type TelegramConfig struct {
	BotToken      string `yaml:"bot_token"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type DiscordConfig struct {
	BotToken  string `yaml:"bot_token"`
	PublicKey string `yaml:"public_key"` // Interaction署名検証用(HEX)
}

type SlackConfig struct {
	AppToken      string `yaml:"app_token"` // Socket Mode 用（将来利用）
	BotToken      string `yaml:"bot_token"` // chat.postMessage 用
	SigningSecret string `yaml:"signing_secret"`
}

// LogConfig はログ設定
type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// DistributedConfig は分散実行設定
// YAML に distributed セクションがない場合、ゼロ値（Enabled=false）でv3互換動作
type DistributedConfig struct {
	Enabled    bool                       `yaml:"enabled"`
	Transports map[string]TransportConfig `yaml:"transports"`
}

// TransportConfig はAgent別のTransport設定
type TransportConfig struct {
	Type             string `yaml:"type"`               // "local" or "ssh"
	RemoteHost       string `yaml:"remote_host"`        // SSH接続先（例: "192.168.1.100:22"）
	RemoteUser       string `yaml:"remote_user"`        // SSHユーザー名
	SSHKeyPath       string `yaml:"ssh_key_path"`       // SSH秘密鍵パス
	StrictHostKey    bool   `yaml:"strict_host_key"`    // true: known_hosts必須（本番用）、false: Insecureフォールバック許可
	RemoteAgentPath  string `yaml:"remote_agent_path"`  // リモートのpicoclaw-agentパス（例: "C:/Users/nyuki/picoclaw-agent.exe"）
	RemoteConfigPath string `yaml:"remote_config_path"` // リモートのconfig.yamlパス（例: "C:/Users/nyuki/.picoclaw/config.yaml"）
}

// IdleChatConfig はAgent間雑談モードの設定
type IdleChatConfig struct {
	Enabled      bool     `yaml:"enabled"`      // 雑談モードの有効化（デフォルト: false）
	Participants []string `yaml:"participants"` // 参加Agent名（デフォルト: ["mio", "shiro"]）
	IntervalMin  int      `yaml:"interval_min"` // 雑談開始までのアイドル時間・分（デフォルト: 5）
	MaxTurns     int      `yaml:"max_turns"`    // 1回の雑談の最大ターン数（デフォルト: 10）
	Temperature  float64  `yaml:"temperature"`  // 雑談時の温度（デフォルト: 0.8）
}

// ConversationConfig は会話LLMの設定
type ConversationConfig struct {
	Enabled      bool   `yaml:"enabled"`       // 会話LLM機能の有効化（デフォルト: false）
	RedisURL     string `yaml:"redis_url"`     // Redis接続先（例: "redis://localhost:6379"）
	DuckDBPath   string `yaml:"duckdb_path"`   // DuckDBファイルパス（例: "/var/lib/picoclaw/memory.duckdb"）
	VectorDBURL  string `yaml:"vectordb_url"`  // VectorDB gRPC接続先（例: "localhost:6334" for Qdrant）
	EmbedModel   string `yaml:"embed_model"`   // Embedding用モデル（例: "nomic-embed-text"）。空の場合はembedding無効
	SummaryModel string `yaml:"summary_model"` // 要約用モデル（例: "chat-v1"）。空の場合はOllama chatモデルを使用
}

// HeartbeatConfig はハートビート（定期タスク）の設定
type HeartbeatConfig struct {
	Enabled  bool   `yaml:"enabled"`  // ハートビートの有効化（デフォルト: false）
	Interval int    `yaml:"interval"` // チェック間隔（分）、最小5分（デフォルト: 30）
	ChatID   string `yaml:"chat_id"`  // LINE Push通知先のユーザーID
}

// SubagentConfig はサブエージェントシステムの設定
type SubagentConfig struct {
	Enabled       bool   `yaml:"enabled"`            // サブエージェント有効化（デフォルト: false）
	MaxIterations int    `yaml:"max_iterations"`     // ReActループ最大反復回数（デフォルト: 10）
	Provider      string `yaml:"provider,omitempty"` // LLMプロバイダー: "ollama"(default), "claude", "openai", "deepseek"
	Model         string `yaml:"model,omitempty"`    // 使用モデル（空=各プロバイダーのデフォルトモデルを使用）
}

// SecurityConfig は実行ポリシーと監査設定
type SecurityConfig struct {
	Enabled            bool                `yaml:"enabled"`
	PolicyMode         string              `yaml:"policy_mode"`          // strict|balanced|dev
	ApprovalMode       string              `yaml:"approval_mode"`        // never|on_demand|always
	ApprovalTTLMinutes int                 `yaml:"approval_ttl_minutes"` // 承認待ちTTL
	NetworkScope       string              `yaml:"network_scope"`        // blocked|allowlist|full (optional: fallback to profile)
	NetworkAllowlist   []string            `yaml:"network_allowlist"`    // host allowlist when network_scope=allowlist
	DenyCommands       []string            `yaml:"deny_commands"`
	WorkspaceEnforced  bool                `yaml:"workspace_enforced"`
	Audit              SecurityAuditConfig `yaml:"audit"`
}

// SecurityAuditConfig は監査ログ出力設定
type SecurityAuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Backend string `yaml:"backend"` // jsonl|sqlite
	Path    string `yaml:"path"`
}

// TTSConfig configures provider fallback and playback verification.
type TTSConfig struct {
	Enabled           bool                `yaml:"enabled"`
	OutputDir         string              `yaml:"output_dir"`
	AudioPathRoot     string              `yaml:"audio_path_root"`
	HTTPBaseURL       string              `yaml:"http_base_url"`
	WSURL             string              `yaml:"ws_url"`
	ConnectTimeoutMS  int                 `yaml:"connect_timeout_ms"`
	ReceiveTimeoutMS  int                 `yaml:"receive_timeout_ms"`
	ChunkGapTimeoutMS int                 `yaml:"chunk_gap_timeout_ms"`
	VoiceID           string              `yaml:"voice_id"`
	SpeechMode        string              `yaml:"speech_mode"`
	ProviderPriority  []string            `yaml:"provider_priority"` // e.g. sbv2,azure,eleven
	PlaybackCommands  []TTSCommandConfig  `yaml:"playback_commands"`
	SBV2              TTSSBV2Config       `yaml:"sbv2"`
	Azure             TTSAzureConfig      `yaml:"azure"`
	Eleven            TTSElevenLabsConfig `yaml:"eleven"`
}

type TTSCommandConfig struct {
	Name string   `yaml:"name"`
	Args []string `yaml:"args"`
}

type TTSSBV2Config struct {
	Enabled    bool   `yaml:"enabled"`
	BaseURL    string `yaml:"base_url"`
	VoiceID    string `yaml:"voice_id"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

type TTSAzureConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Endpoint   string `yaml:"endpoint"`
	APIKey     string `yaml:"api_key"`
	VoiceName  string `yaml:"voice_name"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

type TTSElevenLabsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	APIKey     string `yaml:"api_key"`
	VoiceID    string `yaml:"voice_id"`
	ModelID    string `yaml:"model_id"`
	TimeoutSec int    `yaml:"timeout_sec"`
}

// GoogleSearchConfig はGoogle Search API設定
type GoogleSearchConfig struct {
	APIKey         string `yaml:"api_key"`          // 環境変数から読み込み推奨
	SearchEngineID string `yaml:"search_engine_id"` // カスタム検索エンジンID
}

// LoadConfig は設定ファイルを読み込む
func LoadConfig(path string) (*Config, error) {
	// ファイル読み込み
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// ${ENV_VAR} を環境変数で展開してから YAML パース
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	// デフォルト値設定
	cfg.setDefaults()

	// バリデーション
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	// プロンプトファイル読み込み（prompts/ → workspace/ の順でオーバーライド）
	cfg.Prompts = LoadPrompts(cfg.PromptsDir, cfg.WorkspaceDir)

	return &cfg, nil
}

// setDefaults はデフォルト値を設定
func (c *Config) setDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}

	// v3後方互換: chat_model/worker_model が設定されている場合は Model にマッピング
	if c.Ollama.Model == "" {
		if c.Ollama.ChatModel != "" {
			log.Printf("WARN: ollama.chat_model is deprecated, use ollama.model instead")
			c.Ollama.Model = c.Ollama.ChatModel
		} else {
			c.Ollama.Model = "picoclaw-v1"
		}
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

	// Worker設定デフォルト
	if c.Worker.CommitMessagePrefix == "" {
		c.Worker.CommitMessagePrefix = "[Worker Auto-Commit]"
	}

	if c.Worker.CommandTimeout == 0 {
		c.Worker.CommandTimeout = 300 // 5分
	}

	if c.Worker.GitTimeout == 0 {
		c.Worker.GitTimeout = 30 // 30秒
	}

	if len(c.Worker.ProtectedPatterns) == 0 {
		c.Worker.ProtectedPatterns = []string{".env*", "*credentials*", "*.key", "*.pem"}
	}

	if c.Worker.ActionOnProtected == "" {
		c.Worker.ActionOnProtected = "error"
	}

	if c.Worker.Workspace == "" {
		c.Worker.Workspace = "." // カレントディレクトリ
	}

	// v4.0 Worker並列実行デフォルト
	if c.Worker.MaxParallelism == 0 {
		c.Worker.MaxParallelism = 4
	}

	// v4.0 IdleChat デフォルト
	if c.IdleChat.Enabled {
		if len(c.IdleChat.Participants) == 0 {
			c.IdleChat.Participants = []string{"mio", "shiro"}
		}
		if c.IdleChat.IntervalMin == 0 {
			c.IdleChat.IntervalMin = 5
		}
		if c.IdleChat.MaxTurns == 0 {
			c.IdleChat.MaxTurns = 10
		}
		if c.IdleChat.Temperature == 0 {
			c.IdleChat.Temperature = 0.8
		}
	}

	// v5.0 Conversation デフォルト
	// enabled: false がデフォルト（明示的に有効化が必要）
	if c.Conversation.RedisURL == "" {
		c.Conversation.RedisURL = "redis://localhost:6379"
	}
	if c.Conversation.DuckDBPath == "" {
		c.Conversation.DuckDBPath = "/var/lib/picoclaw/memory.duckdb"
	}
	if c.Conversation.VectorDBURL == "" {
		c.Conversation.VectorDBURL = "localhost:6334"
	}

	// Heartbeat デフォルト
	if c.Heartbeat.Interval == 0 {
		c.Heartbeat.Interval = 30
	}

	// Subagent デフォルト
	if c.Subagent.MaxIterations == 0 {
		c.Subagent.MaxIterations = 10
	}

	if c.Security.PolicyMode == "" {
		c.Security.PolicyMode = "balanced"
	}
	if c.Security.ApprovalMode == "" {
		c.Security.ApprovalMode = "never"
	}
	if c.Security.ApprovalTTLMinutes == 0 {
		c.Security.ApprovalTTLMinutes = 10
	}
	if len(c.Security.DenyCommands) == 0 {
		c.Security.DenyCommands = []string{"rm -rf", "git reset --hard"}
	}
	if c.Security.Audit.Backend == "" {
		c.Security.Audit.Backend = "jsonl"
	}
	if c.Security.Audit.Path == "" {
		c.Security.Audit.Path = "logs/execution_audit.jsonl"
	}

	// v5.1 プロンプト/workspace デフォルト
	if c.PromptsDir == "" {
		c.PromptsDir = "./prompts"
	}
	if c.WorkspaceDir == "" {
		c.WorkspaceDir = "./workspace"
	}
	if len(c.TTS.ProviderPriority) == 0 {
		c.TTS.ProviderPriority = []string{"sbv2", "azure", "eleven"}
	}
	if c.TTS.OutputDir == "" {
		c.TTS.OutputDir = "./workspace/tts"
	}
	if c.TTS.HTTPBaseURL == "" {
		c.TTS.HTTPBaseURL = "http://127.0.0.1:8765"
	}
	if c.TTS.WSURL == "" {
		c.TTS.WSURL = "ws://127.0.0.1:8765/sessions"
	}
	if c.TTS.ConnectTimeoutMS <= 0 {
		c.TTS.ConnectTimeoutMS = 3000
	}
	if c.TTS.ReceiveTimeoutMS <= 0 {
		c.TTS.ReceiveTimeoutMS = 15000
	}
	if c.TTS.ChunkGapTimeoutMS <= 0 {
		c.TTS.ChunkGapTimeoutMS = 3000
	}
	if c.TTS.VoiceID == "" {
		c.TTS.VoiceID = "female_01"
	}
	if c.TTS.SpeechMode == "" {
		c.TTS.SpeechMode = "conversational"
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

	if c.Ollama.Model == "" {
		return fmt.Errorf("ollama model is required")
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
		if c.IdleChat.IntervalMin < 1 {
			return fmt.Errorf("idle_chat.interval_min must be >= 1")
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
		if c.Security.ApprovalMode != "never" && c.Security.ApprovalMode != "on_demand" && c.Security.ApprovalMode != "always" {
			return fmt.Errorf("security.approval_mode must be 'never', 'on_demand', or 'always'")
		}
		if c.Security.NetworkScope != "" &&
			c.Security.NetworkScope != "blocked" &&
			c.Security.NetworkScope != "allowlist" &&
			c.Security.NetworkScope != "full" {
			return fmt.Errorf("security.network_scope must be 'blocked', 'allowlist', or 'full'")
		}
		if c.Security.ApprovalTTLMinutes < 1 {
			return fmt.Errorf("security.approval_ttl_minutes must be >= 1")
		}
		if c.Security.Audit.Backend != "jsonl" && c.Security.Audit.Backend != "sqlite" {
			return fmt.Errorf("security.audit.backend must be 'jsonl' or 'sqlite'")
		}
	}

	return nil
}
