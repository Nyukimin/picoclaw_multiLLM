package config

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

	// === MLX / local OpenAI-compatible LLM runtime ===
	LocalLLM LocalLLMConfig `yaml:"local_llm"`

	// === v5.1 プロンプト外部ファイル ===
	PromptsDir   string         `yaml:"prompts_dir"`   // プロンプトファイルのベースディレクトリ（デフォルト）
	WorkspaceDir string         `yaml:"workspace_dir"` // ユーザーカスタマイズ領域（オーバーライド）
	Prompts      *LoadedPrompts `yaml:"-"`             // 読み込み済みプロンプト（YAML非対象）

	// === Heartbeat ===
	Heartbeat HeartbeatConfig `yaml:"heartbeat"`

	// === Glossary / Recent Topics ===
	Glossary GlossaryConfig `yaml:"glossary"`

	// === Google Search API ===
	GoogleSearchChat   GoogleSearchConfig `yaml:"google_search_chat"`
	GoogleSearchWorker GoogleSearchConfig `yaml:"google_search_worker"`

	// === Subagent ===
	Subagent SubagentConfig `yaml:"subagent"`

	// === Capability Detection (v4.1) ===
	Capability CapabilityConfig `yaml:"capability"`

	// === Security / Execution Audit ===
	Security SecurityConfig `yaml:"security"`

	// === TTS / OpenClaw parity ===
	TTS TTSConfig `yaml:"tts"`

	// === STT / HTTPS Viewer voice input ===
	STT STTConfig `yaml:"stt"`

	// === VTuber / VTube Studio integration ===
	VTuber VTuberConfig `yaml:"vtuber"`

	// === Coder4 AudioRouter ===
	AudioRouter AudioRouterConfig `yaml:"audio_router"`

	// === Viewer persisted JSON operation log ===
	ViewerLog ViewerLogConfig `yaml:"viewer_log"`

	// === Viewer → MLX 管理デーモン プロキシ（stop / restart / status）===
	// トークンは環境変数 LLM_OPS_TOKEN のみ（YAML に平文保存しないこと）。
	LLMOps LLMOpsConfig `yaml:"llm_ops"`

	// === Agent Persona files (v4.2) ===
	MioPersonaFile string `yaml:"mio_persona_file"` // workspace_dir からの相対パス

	// === Coder スロット（v4.1: 4体化 + Agent Persona） ===
	Coder1 CoderConfig `yaml:"coder1"`
	Coder2 CoderConfig `yaml:"coder2"`
	Coder3 CoderConfig `yaml:"coder3"`
	Coder4 CoderConfig `yaml:"coder4"` // 新規追加
}

// ServerConfig はサーバー設定
type ServerConfig struct {
	Port int       `yaml:"port"`
	Host string    `yaml:"host"`
	TLS  TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
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

// LocalLLMConfig is the primary local inference runtime for Chat / Worker / Heavy / Wild.
// It is intended for OpenAI-compatible local servers such as MLX.
type LocalLLMConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Provider          string `yaml:"provider"` // local_openai (default) or ollama
	BaseURL           string `yaml:"base_url"`
	ChatBaseURL       string `yaml:"chat_base_url"`
	WorkerBaseURL     string `yaml:"worker_base_url"`
	HeavyBaseURL      string `yaml:"heavy_base_url"`
	WildBaseURL       string `yaml:"wild_base_url"`
	APIKey            string `yaml:"api_key"`
	ChatModel         string `yaml:"chat_model"`
	WorkerModel       string `yaml:"worker_model"`
	HeavyModel        string `yaml:"heavy_model"`
	WildModel         string `yaml:"wild_model"`
	TimeoutSec        int    `yaml:"timeout_sec"`
	Warmup            *bool  `yaml:"warmup"`
	GlobalConcurrency int    `yaml:"global_concurrency"`
	ModelConcurrency  int    `yaml:"model_concurrency"`
}

// LLMOpsConfig は MLX 運用デーモン（8079 番管理 API）への Viewer 経由プロキシ用。
// Bearer は LLM_OPS_TOKEN 環境変数から読む。
type LLMOpsConfig struct {
	Enabled bool   `yaml:"enabled"`
	BaseURL string `yaml:"base_url"` // 例: http://192.168.1.31:8079
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

	// === v4.2: Agent Persona ===
	PersonaFile string `yaml:"persona_file"` // workspace_dir からの相対パス
	Tone        string `yaml:"tone"`         // 口調ヒント（TTS 連携用）

	// === v4.1: Autonomous ===
	MaxRepair int `yaml:"max_repair"` // 自律実行のリペア上限（0以下は1とみなす、デフォルト: 1）
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
	Enabled         bool                       `yaml:"enabled"`
	Transports      map[string]TransportConfig `yaml:"transports"`
	CoderTimeoutSec int                        `yaml:"coder_timeout_sec"` // Coder SSH タイムアウト秒数（0以下は360とみなす）
	CoderRetryMax   int                        `yaml:"coder_retry_max"`   // Coder リトライ上限（0以下は2とみなす）
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
	Enabled      bool     `yaml:"enabled"`        // 雑談モードの有効化（デフォルト: false）
	Participants []string `yaml:"participants"`   // 参加Agent名（デフォルト: ["mio", "shiro"]）
	IntervalMin  int      `yaml:"interval_min"`   // 雑談開始までのアイドル時間・分（デフォルト: 5）
	IntervalSec  int      `yaml:"interval_sec"`   // 雑談開始までのアイドル時間・秒（指定時は interval_min より優先）
	MaxTurns     int      `yaml:"max_turns"`      // 1回の雑談の最大ターン数（デフォルト: 10）
	Temperature  float64  `yaml:"temperature"`    // 雑談時の温度（デフォルト: 0.8）
	StoryDataDir string   `yaml:"story_data_dir"` // 物語データJSONディレクトリ（デフォルト: "data/story"）
}

// ConversationConfig は会話LLMの設定
type ConversationConfig struct {
	Enabled      bool   `yaml:"enabled"`        // 会話LLM機能の有効化（デフォルト: false）
	RedisURL     string `yaml:"redis_url"`      // Redis接続先（例: "redis://localhost:6379"）
	L1SQLitePath string `yaml:"l1_sqlite_path"` // L1 hot store SQLite path（任意）
	DuckDBPath   string `yaml:"duckdb_path"`    // DuckDBファイルパス（例: "/var/lib/picoclaw/memory.duckdb"）
	VectorDBURL  string `yaml:"vectordb_url"`   // VectorDB gRPC接続先（例: "localhost:6334" for Qdrant）
	EmbedModel   string `yaml:"embed_model"`    // Embedding用モデル（例: "nomic-embed-text"）。空の場合はembedding無効
	SummaryModel string `yaml:"summary_model"`  // 要約用モデル（例: "chat-v1"）。空の場合はOllama chatモデルを使用
}

// HeartbeatConfig はハートビート（定期タスク）の設定
type HeartbeatConfig struct {
	Enabled  bool   `yaml:"enabled"`  // ハートビートの有効化（デフォルト: false）
	Interval int    `yaml:"interval"` // チェック間隔（分）、最小5分（デフォルト: 30）
	Channel  string `yaml:"channel"`  // 通知先チャネル（line, telegram, discord, slack）
	ChatID   string `yaml:"chat_id"`  // 通知先ID（LINE user ID / Telegram chat ID / Discord channel ID / Slack channel ID）
}

type GlossaryConfig struct {
	Enabled           bool     `yaml:"enabled"`
	DBPath            string   `yaml:"db_path"`
	RefreshIntervalHr int      `yaml:"refresh_interval_hr"`
	MaxEntries        int      `yaml:"max_entries"`
	FeedURLs          []string `yaml:"feed_urls"`
}

// SubagentConfig はサブエージェントシステムの設定
type SubagentConfig struct {
	Enabled       bool   `yaml:"enabled"`            // サブエージェント有効化（デフォルト: false）
	MaxIterations int    `yaml:"max_iterations"`     // ReActループ最大反復回数（デフォルト: 10）
	Provider      string `yaml:"provider,omitempty"` // LLMプロバイダー: "ollama"(default), "claude", "openai", "deepseek"
	Model         string `yaml:"model,omitempty"`    // 使用モデル（空=各プロバイダーのデフォルトモデルを使用）
}

// CapabilityConfig はケイパビリティ適応システムの設定（v4.1）
type CapabilityConfig struct {
	// ProbeLLMs: true の場合、起動時に各 LLM に疎通確認を実施する
	// false の場合は config に記載された情報だけでケイパビリティを決定する
	ProbeLLMs bool `yaml:"probe_llms"`

	// ToolRegistryDB: ToolRegistry の DuckDB ファイルパス（空の場合は ToolRegistry 無効）
	ToolRegistryDB string `yaml:"tool_registry_db"`

	// LLMQualityOverrides: モデル名 → 品質ランク（1〜5）の上書き設定
	LLMQualityOverrides map[string]int `yaml:"llm_quality_overrides"`
}

// SecurityConfig は実行ポリシーと監査設定
type SecurityConfig struct {
	Enabled           bool                `yaml:"enabled"`
	PolicyMode        string              `yaml:"policy_mode"`       // strict|balanced|dev
	NetworkScope      string              `yaml:"network_scope"`     // blocked|allowlist|full (optional: fallback to profile)
	NetworkAllowlist  []string            `yaml:"network_allowlist"` // host allowlist when network_scope=allowlist
	DenyCommands      []string            `yaml:"deny_commands"`
	WorkspaceEnforced bool                `yaml:"workspace_enforced"`
	Audit             SecurityAuditConfig `yaml:"audit"`
}

// SecurityAuditConfig は監査ログ出力設定
type SecurityAuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Backend string `yaml:"backend"` // jsonl|sqlite
	Path    string `yaml:"path"`
}

type ViewerLogConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Path              string `yaml:"path"`
	RetentionDays     int    `yaml:"retention_days"`
	GCIntervalMinutes int    `yaml:"gc_interval_minutes"`
}

// TTSConfig configures provider fallback and playback verification.
type TTSConfig struct {
	Enabled          bool                `yaml:"enabled"`
	OutputDir        string              `yaml:"output_dir"`
	AudioPathRoot    string              `yaml:"audio_path_root"`
	HTTPBaseURL      string              `yaml:"http_base_url"`
	TLSSkipVerify    bool                `yaml:"tls_skip_verify"`
	TimeoutMS        int                 `yaml:"timeout_ms"`
	VoiceID          string              `yaml:"voice_id"`
	ProviderParams   map[string]any      `yaml:"provider_params"`
	ProviderPriority []string            `yaml:"provider_priority"` // e.g. irodori
	PlaybackCommands []TTSCommandConfig  `yaml:"playback_commands"`
	SBV2             TTSSBV2Config       `yaml:"sbv2"`
	Irodori          TTSIrodoriConfig    `yaml:"irodori"`
	Azure            TTSAzureConfig      `yaml:"azure"`
	Eleven           TTSElevenLabsConfig `yaml:"eleven"`
}

type STTConfig struct {
	Enabled        bool              `yaml:"enabled"`
	Provider       string            `yaml:"provider"`
	Language       string            `yaml:"language"`
	Model          string            `yaml:"model"`
	TimeoutMS      int               `yaml:"timeout_ms"`
	VAD            bool              `yaml:"vad"`
	EndpointPath   string            `yaml:"endpoint_path"`
	ProviderURL    string            `yaml:"provider_url"`
	StreamURL      string            `yaml:"stream_url"`
	ProviderParams map[string]any    `yaml:"provider_params"`
	Debug          STTDebugConfig    `yaml:"debug"`
	ExternalHTTP   STTExternalConfig `yaml:"external_http"`
}

type STTDebugConfig struct {
	SaveAudio      bool `yaml:"save_audio"`
	SaveTranscript bool `yaml:"save_transcript"`
}

type STTExternalConfig struct {
	URL       string `yaml:"url"`
	StreamURL string `yaml:"stream_url"`
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

type TTSIrodoriConfig struct {
	Enabled               bool    `yaml:"enabled"`
	BaseURL               string  `yaml:"base_url"`
	EndpointPath          string  `yaml:"endpoint_path"`
	VoiceID               string  `yaml:"voice_id"`
	VoiceName             string  `yaml:"voice_name"`
	ReferenceAudio        string  `yaml:"reference_audio"`
	ReferenceAudioURL     string  `yaml:"reference_audio_url"`
	TimeoutSec            int     `yaml:"timeout_sec"`
	Checkpoint            string  `yaml:"checkpoint"`
	ModelDevice           string  `yaml:"model_device"`
	ModelPrecision        string  `yaml:"model_precision"`
	CodecDevice           string  `yaml:"codec_device"`
	CodecPrecision        string  `yaml:"codec_precision"`
	EnableWatermark       bool    `yaml:"enable_watermark"`
	NumSteps              int     `yaml:"num_steps"`
	NumCandidates         int     `yaml:"num_candidates"`
	SeedRaw               string  `yaml:"seed_raw"`
	CFGGuidanceMode       string  `yaml:"cfg_guidance_mode"`
	CFGScaleText          float64 `yaml:"cfg_scale_text"`
	CFGScaleSpeaker       float64 `yaml:"cfg_scale_speaker"`
	CFGScaleRaw           string  `yaml:"cfg_scale_raw"`
	CFGMinT               float64 `yaml:"cfg_min_t"`
	CFGMaxT               float64 `yaml:"cfg_max_t"`
	ContextKVCache        bool    `yaml:"context_kv_cache"`
	TruncationFactorRaw   string  `yaml:"truncation_factor_raw"`
	RescaleKRaw           string  `yaml:"rescale_k_raw"`
	RescaleSigmaRaw       string  `yaml:"rescale_sigma_raw"`
	SpeakerKVScaleRaw     string  `yaml:"speaker_kv_scale_raw"`
	SpeakerKVMinTRaw      string  `yaml:"speaker_kv_min_t_raw"`
	SpeakerKVMaxLayersRaw string  `yaml:"speaker_kv_max_layers_raw"`
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

// VTuberConfig configures VTube Studio emotion event delivery.
type VTuberConfig struct {
	Enabled        bool                             `yaml:"enabled"`
	TickIntervalMS int                              `yaml:"tick_interval_ms"`
	ConnectTimeout int                              `yaml:"connect_timeout_ms"`
	WriteTimeout   int                              `yaml:"write_timeout_ms"`
	Characters     map[string]VTuberCharacterConfig `yaml:"characters"`
}

type VTuberCharacterConfig struct {
	AudioOutput   string            `yaml:"audio_output"`
	VTSHost       string            `yaml:"vts_host"`
	VTSPort       int               `yaml:"vts_port"`
	ExpressionMap map[string]string `yaml:"expression_map"`
}

// AudioRouterConfig configures Coder4-side audio routing.
type AudioRouterConfig struct {
	Enabled           bool                               `yaml:"enabled"`
	SSEURL            string                             `yaml:"sse_url"`
	ConnectTimeoutMS  int                                `yaml:"connect_timeout_ms"`
	DownloadTimeoutMS int                                `yaml:"download_timeout_ms"`
	RetryDelayMS      int                                `yaml:"retry_delay_ms"`
	BufferMS          int                                `yaml:"buffer_ms"`
	DeviceMap         map[string]AudioRouterDeviceConfig `yaml:"device_map"`
}

type AudioRouterDeviceConfig struct {
	DeviceID    string `yaml:"device_id"`
	DisplayName string `yaml:"display_name"`
}

// GoogleSearchConfig はGoogle Search API設定
type GoogleSearchConfig struct {
	APIKey         string `yaml:"api_key"`          // 環境変数から読み込み推奨
	SearchEngineID string `yaml:"search_engine_id"` // カスタム検索エンジンID
}

// CoderConfig は Coder 個別設定（v4.1: 4体化 + Agent Persona）
type CoderConfig struct {
	Name        string            `yaml:"name"`         // 任意の名前（aka, ao, gin, kin 等）
	DisplayName string            `yaml:"display_name"` // 表示名（赤, 青, 銀, 金 等）
	Provider    string            `yaml:"provider"`     // deepseek/openai/claude/gemini
	Model       string            `yaml:"model"`
	APIKey      string            `yaml:"api_key"`      // 環境変数参照（${...}）
	BaseURL     string            `yaml:"base_url"`     // オプション（DeepSeek 等）
	PersonaFile string            `yaml:"persona_file"` // ペルソナファイル（workspace_dir からの相対パス）
	Personality string            `yaml:"personality"`  // インラインペルソナ（persona_file がなければ使用）
	Tone        string            `yaml:"tone"`         // 口調（TTS 連携用）
	LightMemory LightMemoryConfig `yaml:"light_memory"`
	Enabled     bool              `yaml:"enabled"`
}

// LightMemoryConfig は短期記憶設定
type LightMemoryConfig struct {
	Enabled  bool `yaml:"enabled"`
	MaxTurns int  `yaml:"max_turns"` // 保持ターン数（推奨: 3〜5）
}
