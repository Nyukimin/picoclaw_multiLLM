package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
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
	// 設定ファイルパス
	configPath := getConfigPath()

	// 設定読み込み
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded config from: %s", configPath)

	// 依存関係構築
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

	server := &http.Server{
		Addr:    addr,
		Handler: dependencies.lineHandler,
		ConnState: func(conn net.Conn, state http.ConnState) {
			log.Printf("[ConnState] %s -> %s (remote: %s)", state.String(), conn.LocalAddr(), conn.RemoteAddr())
		},
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// Dependencies はアプリケーション依存関係
type Dependencies struct {
	lineHandler     http.Handler
	router          *transport.MessageRouter                       // v4 distributed mode
	idleChatOrch    *idlechat.IdleChatOrchestrator                 // v4 idle chat
	sshTransports   map[string]domaintransport.Transport           // v4 SSH transports
}

// Shutdown はリソースを解放
func (d *Dependencies) Shutdown() {
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
		domainCoder := agent.NewCoderAgent(deepseekProvider, nil, nil)
		coder1Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("DeepSeek (Coder1) enabled with model: %s", cfg.DeepSeek.Model)
	}

	// OpenAI (Coder2) - API キーがある場合のみ
	if cfg.OpenAI.APIKey != "" {
		openaiProvider := openai.NewOpenAIProvider(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
		domainCoder := agent.NewCoderAgent(openaiProvider, nil, nil)
		coder2Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("OpenAI (Coder2) enabled with model: %s", cfg.OpenAI.Model)
	}

	// Claude (Coder3) - API キーがある場合のみ
	if cfg.Claude.APIKey != "" {
		claudeProvider := claude.NewClaudeProvider(cfg.Claude.APIKey, cfg.Claude.Model)
		domainCoder := agent.NewCoderAgent(claudeProvider, nil, nil)
		coder3Adapter = &coderAdapter{domainCoder: domainCoder}
		log.Printf("Claude (Coder3) enabled with model: %s", cfg.Claude.Model)
	}

	// 2. Routing Components
	classifier := routing.NewLLMClassifier(ollamaProvider)
	ruleDictionary := routing.NewRuleDictionary()

	// 3. Tool Runner（Chat用とWorker用で分離）
	chatToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:       os.Getenv("GOOGLE_API_KEY_CHAT"),
		GoogleSearchEngineID: os.Getenv("GOOGLE_SEARCH_ENGINE_ID_CHAT"),
	}
	workerToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:       os.Getenv("GOOGLE_API_KEY_WORKER"),
		GoogleSearchEngineID: os.Getenv("GOOGLE_SEARCH_ENGINE_ID_WORKER"),
	}

	chatToolRunner := tools.NewToolRunner(chatToolRunnerCfg)
	workerToolRunner := tools.NewToolRunner(workerToolRunnerCfg)
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

	// 5. Agents
	mioAgent := agent.NewMioAgent(ollamaProvider, classifier, ruleDictionary, chatToolRunner, mcpClient)
	shiroAgent := agent.NewShiroAgent(ollamaProvider, workerToolRunner, mcpClient)

	// 6. Session Repository
	sessionRepo := session.NewJSONSessionRepository(cfg.Session.StorageDir)

	// セッションディレクトリ作成
	if err := os.MkdirAll(cfg.Session.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create session directory: %v", err)
	}

	// 7. Worker Execution Service
	workerExecutionService := service.NewWorkerExecutionService(cfg.Worker)
	log.Printf("WorkerExecutionService initialized (Workspace: %s, Parallel: %v)",
		cfg.Worker.Workspace, cfg.Worker.ParallelExecution)

	deps := &Dependencies{}

	// 8. v3/v4 モード分岐
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

	log.Println("Dependency injection complete")
	return deps
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
		)
		idleChatOrch.Start()
		d.idleChatOrch = idleChatOrch
		log.Printf("IdleChat enabled (participants=%v)", cfg.IdleChat.Participants)
	}
}

// getConfigPath は設定ファイルパスを取得
func getConfigPath() string {
	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		return path
	}
	return "./config.yaml"
}

// mustGetToolList はツールリストを取得（エラーは無視）
func mustGetToolList(runner *tools.ToolRunner) []string {
	tools, _ := runner.List(context.Background())
	return tools
}
