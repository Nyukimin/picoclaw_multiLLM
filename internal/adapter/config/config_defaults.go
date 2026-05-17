package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

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

	if c.LocalLLM.Provider == "" {
		c.LocalLLM.Provider = "local_openai"
	}
	if c.LocalLLM.ChatModel == "" {
		c.LocalLLM.ChatModel = "Chat"
	}
	if c.LocalLLM.WorkerModel == "" {
		c.LocalLLM.WorkerModel = "Worker"
	}
	if c.LocalLLM.HeavyModel == "" {
		c.LocalLLM.HeavyModel = "Heavy"
	}
	if c.LocalLLM.WildModel == "" {
		c.LocalLLM.WildModel = "Wild"
	}
	if c.LocalLLM.TimeoutSec <= 0 {
		c.LocalLLM.TimeoutSec = 120
	}
	if c.LocalLLM.Warmup == nil {
		v := true
		c.LocalLLM.Warmup = &v
	}
	if c.LocalLLM.GlobalConcurrency <= 0 {
		c.LocalLLM.GlobalConcurrency = 2
	}
	if c.LocalLLM.ModelConcurrency <= 0 {
		c.LocalLLM.ModelConcurrency = 1
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
		if c.IdleChat.IntervalSec == 0 {
			c.IdleChat.IntervalSec = c.IdleChat.IntervalMin * 60
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

	if c.Glossary.DBPath == "" {
		c.Glossary.DBPath = "./workspace/glossary.db"
	}
	if c.Glossary.RefreshIntervalHr == 0 {
		c.Glossary.RefreshIntervalHr = 6
	}
	if c.Glossary.MaxEntries == 0 {
		c.Glossary.MaxEntries = 8
	}
	if len(c.Glossary.FeedURLs) == 0 {
		c.Glossary.FeedURLs = []string{
			"https://www3.nhk.or.jp/rss/news/cat0.xml",
			"https://feeds.bbci.co.uk/news/world/rss.xml",
			"https://feeds.bbci.co.uk/news/technology/rss.xml",
		}
	}

	// Subagent デフォルト
	if c.Subagent.MaxIterations == 0 {
		c.Subagent.MaxIterations = 10
	}

	if c.Security.PolicyMode == "" {
		c.Security.PolicyMode = "balanced"
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
	if c.ViewerLog.Path == "" {
		c.ViewerLog.Path = "./workspace/orchestrator_event_log.jsonl"
	}
	if c.ViewerLog.RetentionDays <= 0 {
		c.ViewerLog.RetentionDays = 14
	}
	if c.ViewerLog.GCIntervalMinutes <= 0 {
		c.ViewerLog.GCIntervalMinutes = 60
	}
	if c.Verification.Mode == "" {
		c.Verification.Mode = "dry_run"
	}
	if c.Verification.DefaultLevel == "" {
		c.Verification.DefaultLevel = "low"
	}

	// v5.1 プロンプト/workspace デフォルト
	if c.PromptsDir == "" {
		c.PromptsDir = "./prompts"
	}
	if c.WorkspaceDir == "" {
		c.WorkspaceDir = "./workspace"
	}
	if c.OperationMemoryDir == "" {
		c.OperationMemoryDir = DefaultOperationMemoryDir()
	}
	if c.Verification.ReportPath == "" {
		c.Verification.ReportPath = c.WorkspaceDir + "/verification_report.jsonl"
	}
	if !c.ViewerLog.Enabled {
		c.ViewerLog.Enabled = true
	}
	if len(c.TTS.ProviderPriority) == 0 {
		c.TTS.ProviderPriority = []string{"irodori"}
	}
	if c.TTS.OutputDir == "" {
		c.TTS.OutputDir = "./workspace/tts"
	}
	if c.TTS.HTTPBaseURL == "" {
		c.TTS.HTTPBaseURL = "https://127.0.0.1:8770"
		c.TTS.TLSSkipVerify = true
	}
	if shouldEnableLocalTLSSkipVerify(c.TTS.HTTPBaseURL) {
		c.TTS.TLSSkipVerify = true
	}
	if c.TTS.TimeoutMS <= 0 {
		c.TTS.TimeoutMS = 15000
	}
	if c.TTS.VoiceID == "" {
		c.TTS.VoiceID = "mio"
	}
	if c.TTS.Irodori.VoiceID == "" {
		c.TTS.Irodori.VoiceID = c.TTS.VoiceID
	}
	if c.TTS.Irodori.VoiceName == "" && (strings.EqualFold(c.TTS.Irodori.VoiceID, "mio") || strings.EqualFold(c.TTS.Irodori.VoiceID, "female_01")) {
		c.TTS.Irodori.VoiceName = "Mio"
	}
	if c.TTS.Irodori.VoiceName == "" && (strings.EqualFold(c.TTS.Irodori.VoiceID, "shiro") || strings.EqualFold(c.TTS.Irodori.VoiceID, "male_01")) {
		c.TTS.Irodori.VoiceName = "Shiro"
	}
	if c.TTS.Irodori.EndpointPath == "" {
		c.TTS.Irodori.EndpointPath = "/api/tts"
	}
	if c.TTS.Irodori.TimeoutSec <= 0 {
		c.TTS.Irodori.TimeoutSec = 120
	}
	if c.TTS.Irodori.Checkpoint == "" {
		c.TTS.Irodori.Checkpoint = "Aratako/Irodori-TTS-500M-v2"
	}
	if c.TTS.Irodori.ModelDevice == "" {
		c.TTS.Irodori.ModelDevice = "mps"
	}
	if c.TTS.Irodori.ModelPrecision == "" {
		c.TTS.Irodori.ModelPrecision = "fp32"
	}
	if c.TTS.Irodori.CodecDevice == "" {
		c.TTS.Irodori.CodecDevice = "mps"
	}
	if c.TTS.Irodori.CodecPrecision == "" {
		c.TTS.Irodori.CodecPrecision = "fp32"
	}
	if c.TTS.Irodori.NumSteps <= 0 {
		c.TTS.Irodori.NumSteps = 16
	}
	if c.TTS.Irodori.NumCandidates <= 0 {
		c.TTS.Irodori.NumCandidates = 1
	}
	if c.TTS.Irodori.CFGGuidanceMode == "" {
		c.TTS.Irodori.CFGGuidanceMode = "independent"
	}
	if c.TTS.Irodori.CFGScaleText == 0 {
		c.TTS.Irodori.CFGScaleText = 3.0
	}
	if c.TTS.Irodori.CFGScaleSpeaker == 0 {
		c.TTS.Irodori.CFGScaleSpeaker = 5.0
	}
	if c.TTS.Irodori.CFGMinT == 0 {
		c.TTS.Irodori.CFGMinT = 0.5
	}
	if c.TTS.Irodori.CFGMaxT == 0 {
		c.TTS.Irodori.CFGMaxT = 1.0
	}
	if !c.TTS.Irodori.ContextKVCache {
		c.TTS.Irodori.ContextKVCache = true
	}
	if c.TTS.ProviderParams == nil {
		c.TTS.ProviderParams = map[string]any{}
	}
	if c.STT.Provider == "" {
		c.STT.Provider = "external_http"
	}
	if c.STT.Language == "" {
		c.STT.Language = "ja"
	}
	if c.STT.TimeoutMS <= 0 {
		c.STT.TimeoutMS = 8000
	}
	if c.STT.BusyPolicy == "" {
		c.STT.BusyPolicy = "queue_latest"
	}
	if c.STT.EndpointPath == "" {
		c.STT.EndpointPath = "/stt"
	}
	if c.STT.ProviderParams == nil {
		c.STT.ProviderParams = map[string]any{}
	}
	if envURL := strings.TrimSpace(os.Getenv("STT_PROVIDER_URL")); envURL != "" && c.STT.ProviderURL == "" && c.STT.ExternalHTTP.URL == "" {
		c.STT.Provider = "external_http"
		c.STT.ProviderURL = envURL
	}
	if c.STT.ProviderURL == "" {
		c.STT.ProviderURL = c.STT.ExternalHTTP.URL
	}
	if c.STT.StreamURL == "" {
		c.STT.StreamURL = c.STT.ExternalHTTP.StreamURL
	}
	if c.VTuber.TickIntervalMS <= 0 {
		c.VTuber.TickIntervalMS = 100
	}
	if c.VTuber.ConnectTimeout <= 0 {
		c.VTuber.ConnectTimeout = 3000
	}
	if c.VTuber.WriteTimeout <= 0 {
		c.VTuber.WriteTimeout = 2000
	}
	if c.AudioRouter.ConnectTimeoutMS <= 0 {
		c.AudioRouter.ConnectTimeoutMS = 5000
	}
	if c.AudioRouter.DownloadTimeoutMS <= 0 {
		c.AudioRouter.DownloadTimeoutMS = 15000
	}
	if c.AudioRouter.RetryDelayMS <= 0 {
		c.AudioRouter.RetryDelayMS = 2000
	}
	if c.AudioRouter.BufferMS <= 0 {
		c.AudioRouter.BufferMS = 120
	}

	// Coder スロットのデフォルト値（v4.1）
	if c.Coder1.Provider == "" {
		c.Coder1.Provider = "deepseek"
	}
	if c.Coder1.Model == "" {
		c.Coder1.Model = "deepseek-coder"
	}
	if c.Coder1.Name == "" {
		c.Coder1.Name = "aka"
	}
	if c.Coder1.DisplayName == "" {
		c.Coder1.DisplayName = "赤"
	}
	if c.Coder1.LightMemory.MaxTurns == 0 {
		c.Coder1.LightMemory.MaxTurns = 3
	}

	if c.Coder2.Provider == "" {
		c.Coder2.Provider = "openai"
	}
	if c.Coder2.Model == "" {
		c.Coder2.Model = "gpt-4-turbo"
	}
	if c.Coder2.Name == "" {
		c.Coder2.Name = "ao"
	}
	if c.Coder2.DisplayName == "" {
		c.Coder2.DisplayName = "青"
	}
	if c.Coder2.LightMemory.MaxTurns == 0 {
		c.Coder2.LightMemory.MaxTurns = 3
	}

	if c.Coder3.Provider == "" {
		c.Coder3.Provider = "claude"
	}
	if c.Coder3.Model == "" {
		c.Coder3.Model = "claude-sonnet-4"
	}
	if c.Coder3.Name == "" {
		c.Coder3.Name = "gin"
	}
	if c.Coder3.DisplayName == "" {
		c.Coder3.DisplayName = "銀"
	}
	if c.Coder3.LightMemory.MaxTurns == 0 {
		c.Coder3.LightMemory.MaxTurns = 3
	}

	if c.Coder4.Provider == "" {
		c.Coder4.Provider = "gemini"
	}
	if c.Coder4.Model == "" {
		c.Coder4.Model = "gemini-2.0-flash-exp"
	}
	if c.Coder4.Name == "" {
		c.Coder4.Name = "kin"
	}
	if c.Coder4.DisplayName == "" {
		c.Coder4.DisplayName = "金"
	}
	if c.Coder4.LightMemory.MaxTurns == 0 {
		c.Coder4.LightMemory.MaxTurns = 3
	}
}

// DefaultOperationMemoryDir returns the runtime-owned operation memory directory.
func DefaultOperationMemoryDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return filepath.Join(".picoclaw", "rencrow", "memory")
	}
	return filepath.Join(homeDir, ".picoclaw", "rencrow", "memory")
}
