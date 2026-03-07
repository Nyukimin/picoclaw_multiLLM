package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	healthadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	healthapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/heartbeat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	subagentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/subagent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domaintool "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	infrahealth "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	memorypersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/memory"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

// Version 情報（go build -ldflags で注入）
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// coderAdapter はdomain CoderAgentをorchestrator CoderAgentに適応
type coderAdapter struct {
	domainCoder *agent.CoderAgent
}

func (a *coderAdapter) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return a.domainCoder.GenerateWithPrompt(ctx, t, systemPrompt)
}

func (a *coderAdapter) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
	return a.domainCoder.GenerateProposal(ctx, t)
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		cmdRun()
	case "agent":
		cmdAgent()
	case "version":
		cmdVersion()
	case "health":
		cmdHealth()
	case "status":
		cmdStatus()
	case "help", "-h", "--help":
		cmdHelp()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		cmdHelp()
		os.Exit(1)
	}
}

// cmdRun はHTTPサーバーを起動する（デフォルトコマンド）
func cmdRun() {
	configPath := getConfigPath()

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("PicoClaw %s (commit: %s, built: %s)", Version, Commit, BuildDate)
	log.Printf("Loaded config from: %s", configPath)

	dependencies := buildDependencies(cfg)

	// Graceful shutdown用シグナル
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		log.Printf("Received signal: %v, shutting down...", sig)
		dependencies.Shutdown()
		os.Exit(0)
	}()

	// HTTPサーバー起動
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting PicoClaw server on %s", addr)

	mux := http.NewServeMux()
	mux.Handle("/webhook", dependencies.lineHandler)

	healthHandler := dependencies.buildHealthHandler(cfg)
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	mux.HandleFunc("/ready", healthHandler.HandleReady)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		ConnState: func(conn net.Conn, state http.ConnState) {
			log.Printf("[ConnState] %s -> %s (remote: %s)", state.String(), conn.LocalAddr(), conn.RemoteAddr())
		},
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// cmdVersion はバージョン情報を表示
func cmdVersion() {
	fmt.Printf("picoclaw %s\ncommit: %s\nbuilt:  %s\n", Version, Commit, BuildDate)
}

// cmdHealth はヘルスチェックを実行してJSON出力
func cmdHealth() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	svc := buildHealthService(cfg)
	report := svc.RunChecks(context.Background())

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(report)

	if report.Status == domainhealth.StatusDown {
		os.Exit(1)
	}
}

// cmdStatus はシステム状態の概要を表示
func cmdStatus() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	fmt.Printf("PicoClaw %s\n", Version)
	fmt.Printf("Ollama: %s (model: %s)\n", cfg.Ollama.BaseURL, cfg.Ollama.Model)
	fmt.Printf("Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Println()

	svc := buildHealthService(cfg)
	report := svc.RunChecks(context.Background())

	for _, c := range report.Checks {
		fmt.Printf("  [%s] %s: %s (%dms)\n", c.Status, c.Name, c.Message, c.Duration.Milliseconds())
	}
	fmt.Printf("\nOverall: %s\n", report.Status)

	if report.Status == domainhealth.StatusDown {
		os.Exit(1)
	}
}

// cmdHelp はヘルプメッセージを表示
func cmdHelp() {
	fmt.Printf(`PicoClaw %s - Multi-LLM AI Assistant

Usage: picoclaw [command]

Commands:
  run                Start the HTTP server (default)
  agent <type>       Run in agent mode (worker, coder1, coder2, coder3)
  version            Show version information
  health             Run health checks and output JSON
  status             Show system status overview
  help               Show this help message

Agent Mode:
  picoclaw agent worker   - Worker agent (stdin/stdout JSON)
  picoclaw agent coder1   - Coder1 agent (DeepSeek)
  picoclaw agent coder2   - Coder2 agent (OpenAI)
  picoclaw agent coder3   - Coder3 agent (Claude)
`, Version)
}

// buildHealthService は HealthService を構築（CLI コマンドで共用）
func buildHealthService(cfg *config.Config) *healthapp.HealthService {
	checks := []domainhealth.Check{
		infrahealth.NewOllamaCheck(cfg.Ollama.BaseURL),
		infrahealth.NewOllamaModelCheck(cfg.Ollama.BaseURL, cfg.Ollama.Model),
	}

	// 常駐モデルのコンテキスト長チェック（max_context が設定されている場合のみ）
	if cfg.Ollama.MaxContext > 0 {
		checks = append(checks, infrahealth.NewOllamaModelsCheck(
			cfg.Ollama.BaseURL,
			[]infrahealth.ModelRequirement{
				{Name: cfg.Ollama.Model, MaxContext: cfg.Ollama.MaxContext},
			},
		))
	}

	return healthapp.NewHealthService(checks...)
}

// Dependencies はアプリケーション依存関係
type Dependencies struct {
	lineHandler   http.Handler
	router        *transport.MessageRouter             // v4 distributed mode
	idleChatOrch  *idlechat.IdleChatOrchestrator       // v4 idle chat
	sshTransports map[string]domaintransport.Transport // v4 SSH transports
	heartbeatSvc  *heartbeat.HeartbeatService          // heartbeat service
}

// Shutdown はリソースを解放
func (d *Dependencies) Shutdown() {
	if d.heartbeatSvc != nil {
		d.heartbeatSvc.Stop()
	}
	if d.idleChatOrch != nil {
		d.idleChatOrch.Stop()
	}
	for name, t := range d.sshTransports {
		if err := t.Close(); err != nil {
			log.Printf("Failed to close SSH transport for %s: %v", name, err)
		}
	}
	if d.router != nil {
		d.router.Stop()
	}
	log.Println("Shutdown complete")
}

// buildDependencies は依存関係を構築
func buildDependencies(cfg *config.Config) *Dependencies {
	// 1. LLM Provider (v4: 単一共通モデル)
	ollamaProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, cfg.Ollama.Model)

	var coder1Adapter, coder2Adapter, coder3Adapter *coderAdapter

	// DeepSeek (Coder1) - API キーがある場合のみ
	if cfg.DeepSeek.APIKey != "" {
		deepseekProvider := deepseek.NewDeepSeekProvider(cfg.DeepSeek.APIKey, cfg.DeepSeek.Model)
		domainCoder := agent.NewCoderAgent(deepseekProvider, nil, nil, cfg.Prompts.CoderProposal)
		coder1Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("DeepSeek (Coder1) enabled with model: %s", cfg.DeepSeek.Model)
	}

	// OpenAI (Coder2) - API キーがある場合のみ
	if cfg.OpenAI.APIKey != "" {
		openaiProvider := openai.NewOpenAIProvider(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
		domainCoder := agent.NewCoderAgent(openaiProvider, nil, nil, cfg.Prompts.CoderProposal)
		coder2Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("OpenAI (Coder2) enabled with model: %s", cfg.OpenAI.Model)
	}

	// Claude (Coder3) - API キーがある場合のみ
	if cfg.Claude.APIKey != "" {
		claudeProvider := claude.NewClaudeProvider(cfg.Claude.APIKey, cfg.Claude.Model)
		domainCoder := agent.NewCoderAgent(claudeProvider, nil, nil, cfg.Prompts.CoderProposal)
		coder3Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("Claude (Coder3) enabled with model: %s", cfg.Claude.Model)
	}

	// 2. Routing Components
	classifier := routing.NewLLMClassifier(ollamaProvider, cfg.Prompts.Classifier)
	ruleDictionary := routing.NewRuleDictionary()

	// 3. Tool Runner（Chat用とWorker用で分離）
	chatToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:         cfg.GoogleSearchChat.APIKey,
		GoogleSearchEngineID: cfg.GoogleSearchChat.SearchEngineID,
	}
	workerToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:         cfg.GoogleSearchWorker.APIKey,
		GoogleSearchEngineID: cfg.GoogleSearchWorker.SearchEngineID,
	}

	chatToolRunnerV2 := tools.NewToolRunner(chatToolRunnerCfg)
	workerToolRunnerV2 := tools.NewToolRunner(workerToolRunnerCfg)

	// Subagent配線（2段階構築: ToolRunner作成後にManagerを注入）
	var subagentMgr *subagentapp.Manager
	if cfg.Subagent.Enabled {
		subagentProvider := resolveSubagentProvider(cfg, ollamaProvider)
		toolDefs := workerToolRunnerV2.ToolDefinitions()

		subagentMgr = subagentapp.NewManager(
			subagentProvider,
			workerToolRunnerV2,
			toolDefs,
			toolloop.Config{MaxIterations: cfg.Subagent.MaxIterations},
		)

		workerToolRunnerV2.RegisterSubagent("worker", tools.NewSubagentFuncFromManager(subagentMgr))
		log.Printf("Subagent enabled (provider: %s, max_iterations: %d)",
			subagentProvider.Name(), cfg.Subagent.MaxIterations)
	} else {
		log.Printf("Subagent disabled")
	}

	// LegacyRunner アダプター（V2 → V1 ブリッジ）で agents に注入
	chatToolRunner := domaintool.NewLegacyRunner(chatToolRunnerV2)
	workerToolRunner := domaintool.NewLegacyRunner(workerToolRunnerV2)
	log.Printf("ToolRunner initialized: Chat=%d tools, Worker=%d tools",
		len(mustGetToolList(chatToolRunner)), len(mustGetToolList(workerToolRunner)))

	if chatToolRunnerCfg.GoogleAPIKey != "" && chatToolRunnerCfg.GoogleSearchEngineID != "" {
		log.Printf("Google Search API (Chat) configured")
	}
	if workerToolRunnerCfg.GoogleAPIKey != "" && workerToolRunnerCfg.GoogleSearchEngineID != "" {
		log.Printf("Google Search API (Worker) configured")
	}

	// 4. MCP Client
	mcpClient := mcp.NewMCPClient()
	log.Printf("MCPClient initialized with %d servers", len(mcpClient.ListServers()))

	// 4.5. v5.1 ConversationEngine初期化
	var convEngine conversation.ConversationEngine
	var realMgr *conversationpersistence.RealConversationManager // Phase 4.2: KB自動保存用
	if cfg.Conversation.Enabled {
		// ConversationManager（3層記憶）
		var err error
		realMgr, err = conversationpersistence.NewRealConversationManager(
			cfg.Conversation.RedisURL,
			cfg.Conversation.DuckDBPath,
			cfg.Conversation.VectorDBURL,
		)
		if err != nil {
			log.Fatalf("Failed to initialize conversation manager: %v", err)
		}

		// Embedder注入（embed_model が設定されている場合）
		if cfg.Conversation.EmbedModel != "" {
			embedder := ollama.NewOllamaEmbedder(cfg.Ollama.BaseURL, cfg.Conversation.EmbedModel)
			realMgr.WithEmbedder(embedder)
			log.Printf("  Embedder: %s (model: %s)", cfg.Ollama.BaseURL, cfg.Conversation.EmbedModel)
		}

		// Summarizer注入（summary_model が設定されている場合、なければ chat model）
		summaryModel := cfg.Conversation.SummaryModel
		if summaryModel == "" {
			summaryModel = cfg.Ollama.Model
		}
		if summaryModel != "" {
			summaryProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, summaryModel)
			summarizer := conversationpersistence.NewLLMSummarizer(summaryProvider)
			realMgr.WithSummarizer(summarizer)
			log.Printf("  Summarizer: %s (model: %s)", cfg.Ollama.BaseURL, summaryModel)
		}

		// スレッド境界検出器（Embedder があれば類似度チェックも有効化）
		var embedderForDetector conversation.EmbeddingProvider
		if cfg.Conversation.EmbedModel != "" {
			embedderForDetector = ollama.NewOllamaEmbedder(cfg.Ollama.BaseURL, cfg.Conversation.EmbedModel)
		}
		detector := conversationpersistence.NewThreadBoundaryDetector(embedderForDetector)

		// ProfileExtractor（summary_model を再利用）
		var profileExtractor conversation.ProfileExtractor
		if summaryModel != "" {
			profileProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, summaryModel)
			profileExtractor = conversationpersistence.NewLLMProfileExtractor(profileProvider)
			log.Printf("  ProfileExtractor: %s (model: %s)", cfg.Ollama.BaseURL, summaryModel)
		}

		// ConversationEngine（RecallPack生成 + ペルソナ + スレッド自動検出 + プロファイル抽出）
		engine := conversationpersistence.NewRealConversationEngine(
			realMgr,
			conversation.NewMioPersona(cfg.Prompts.MioPersona),
		).WithDetector(detector)
		if profileExtractor != nil {
			engine = engine.WithProfileExtractor(profileExtractor)
		}
		convEngine = engine

		log.Printf("ConversationEngine v5.1 enabled (RecallPack + Persona + ProfileExtractor)")
		log.Printf("  Redis: %s", cfg.Conversation.RedisURL)
		log.Printf("  DuckDB: %s", cfg.Conversation.DuckDBPath)
		log.Printf("  VectorDB: %s", cfg.Conversation.VectorDBURL)
	} else {
		convEngine = nil
		log.Printf("Conversation LLM disabled (v3/v4 mode)")
	}

	// 5. Memory Store（HeartbeatService用。Mio会話メモリはConversationEngine v5.1が担当）
	memStore := memorypersistence.NewFileStore(cfg.WorkspaceDir)
	log.Printf("MemoryStore initialized (workspace: %s)", cfg.WorkspaceDir)

	// 6. Agents
	mioAgent := agent.NewMioAgent(ollamaProvider, classifier, ruleDictionary, chatToolRunner, mcpClient, convEngine)
	if realMgr != nil {
		mioAgent = mioAgent.WithConversationManager(realMgr)
		log.Printf("Mio: ConversationManager injected (KB autosave enabled)")
	}
	shiroAgent := agent.NewShiroAgent(ollamaProvider, workerToolRunner, mcpClient, cfg.Prompts.Worker, subagentMgr)

	// 7. Session Repository
	sessionRepo := session.NewJSONSessionRepository(cfg.Session.StorageDir)

	// セッションディレクトリ作成
	if err := os.MkdirAll(cfg.Session.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create session directory: %v", err)
	}

	// 8. Worker Execution Service
	workerExecutionService := service.NewWorkerExecutionService(cfg.Worker)
	log.Printf("WorkerExecutionService initialized (Workspace: %s, Parallel: %v)",
		cfg.Worker.Workspace, cfg.Worker.ParallelExecution)

	deps := &Dependencies{}

	// 9. v3/v4 モード分岐
	if cfg.Distributed.Enabled {
		log.Println("=== v4 Distributed Mode ===")
		deps.buildDistributedMode(cfg, sessionRepo, mioAgent, ollamaProvider)
	} else {
		log.Println("=== v3 Local Mode ===")
		// 既存v3ロジック
		orch := orchestrator.NewMessageOrchestrator(
			sessionRepo,
			mioAgent,
			shiroAgent,
			coder1Adapter,
			coder2Adapter,
			coder3Adapter,
			workerExecutionService,
		)
		deps.lineHandler = line.NewHandler(orch, cfg.Line.ChannelSecret, cfg.Line.AccessToken)
	}

	// 10. Heartbeat Service
	if cfg.Heartbeat.Enabled {
		// LINE Push通知用のNotificationSender
		var sender heartbeat.NotificationSender
		if cfg.Line.AccessToken != "" {
			sender = &lineNotificationSender{
				lineSender: line.NewMessageSender(cfg.Line.AccessToken),
				chatID:     cfg.Heartbeat.ChatID,
			}
		}

		heartbeatSvc := heartbeat.NewHeartbeatService(
			mioAgent,
			sender,
			cfg.WorkspaceDir,
			cfg.Heartbeat.Interval,
		)
		heartbeatSvc.WithMemoryStore(memStore)
		heartbeatSvc.Start()
		deps.heartbeatSvc = heartbeatSvc
		log.Printf("HeartbeatService enabled (interval: %dm, workspace: %s)", cfg.Heartbeat.Interval, cfg.WorkspaceDir)
	}

	log.Println("Dependency injection complete")
	return deps
}

// lineNotificationSender はLINE Push APIを使ったNotificationSender実装
type lineNotificationSender struct {
	lineSender *line.MessageSender
	chatID     string
}

func (s *lineNotificationSender) SendNotification(ctx context.Context, message string) error {
	if s.chatID == "" {
		log.Printf("[Heartbeat] notification skipped: PICOCLAW_HEARTBEAT_CHAT_ID not set")
		return nil
	}
	return s.lineSender.SendPushMessage(ctx, s.chatID, message)
}

// buildDistributedMode はv4分散モードの依存関係を構築
func (d *Dependencies) buildDistributedMode(
	cfg *config.Config,
	sessionRepo orchestrator.SessionRepository,
	mioAgent *agent.MioAgent,
	ollamaProvider *ollama.OllamaProvider,
) {
	// Transport Factory でAgent別Transport生成
	factory := transport.NewTransportFactory()
	transports, err := factory.CreateTransports(cfg.Distributed)
	if err != nil {
		log.Fatalf("Failed to create transports: %v", err)
	}

	// MessageRouter 構築（LocalTransport専用）
	router := transport.NewMessageRouter()
	sshTransports := make(map[string]domaintransport.Transport)

	for agentName, t := range transports {
		switch v := t.(type) {
		case *transport.LocalTransport:
			router.RegisterAgent(agentName, v)
			log.Printf("Registered LocalTransport for agent '%s'", agentName)
		case *transport.SSHTransport:
			// SSH接続を確立
			if err := v.Connect(); err != nil {
				log.Fatalf("Failed to connect SSH transport for agent '%s': %v", agentName, err)
			}
			sshTransports[agentName] = v
			log.Printf("Connected SSHTransport for agent '%s'", agentName)
		}
	}
	d.router = router
	d.sshTransports = sshTransports

	// CentralMemory
	centralMemory := domainsession.NewCentralMemory()

	// DistributedOrchestrator（Local + SSH transports）
	distOrch := orchestrator.NewDistributedOrchestrator(
		sessionRepo,
		mioAgent,
		router,
		centralMemory,
		sshTransports,
	)
	d.lineHandler = line.NewHandler(distOrch, cfg.Line.ChannelSecret, cfg.Line.AccessToken)

	// IdleChat（有効な場合）
	if cfg.IdleChat.Enabled {
		idleChatOrch := idlechat.NewIdleChatOrchestrator(
			ollamaProvider,
			centralMemory,
			cfg.IdleChat.Participants,
			cfg.IdleChat.IntervalMin,
			cfg.IdleChat.MaxTurns,
			cfg.IdleChat.Temperature,
			cfg.Prompts.IdleChatAgents,
		)
		idleChatOrch.Start()
		d.idleChatOrch = idleChatOrch
		log.Printf("IdleChat enabled (participants=%v)", cfg.IdleChat.Participants)
	}
}

// buildHealthHandler は Health Check HTTP ハンドラを構築
func (d *Dependencies) buildHealthHandler(cfg *config.Config) *healthadapter.Handler {
	return healthadapter.NewHandler(buildHealthService(cfg))
}

// getConfigPath は設定ファイルパスを取得
func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	return "./config.yaml"
}

// resolveSubagentProvider はサブエージェント用のToolCallingProviderを設定に基づいて選択する
func resolveSubagentProvider(cfg *config.Config, fallback *ollama.OllamaProvider) llm.ToolCallingProvider {
	switch cfg.Subagent.Provider {
	case "claude":
		if cfg.Claude.APIKey == "" {
			log.Fatalf("subagent.provider=claude but claude.api_key is not set")
		}
		model := cfg.Subagent.Model
		if model == "" {
			model = cfg.Claude.Model
		}
		return claude.NewClaudeProvider(cfg.Claude.APIKey, model)

	case "openai":
		if cfg.OpenAI.APIKey == "" {
			log.Fatalf("subagent.provider=openai but openai.api_key is not set")
		}
		model := cfg.Subagent.Model
		if model == "" {
			model = cfg.OpenAI.Model
		}
		return openai.NewOpenAIProvider(cfg.OpenAI.APIKey, model)

	case "deepseek":
		if cfg.DeepSeek.APIKey == "" {
			log.Fatalf("subagent.provider=deepseek but deepseek.api_key is not set")
		}
		model := cfg.Subagent.Model
		if model == "" {
			model = cfg.DeepSeek.Model
		}
		return deepseek.NewDeepSeekProvider(cfg.DeepSeek.APIKey, model)

	default: // "ollama" or empty
		return fallback
	}
}

// mustGetToolList はツールリストを取得（エラーは無視）
func mustGetToolList(runner agent.ToolRunner) []string {
	list, _ := runner.List(context.Background())
	return list
}
