package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	discordadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/discord"
	slackadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/slack"
	telegramadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/telegram"
	chromeadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/chrome"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	entryadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/entry"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	attachmentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/attachment"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/heartbeat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	subagentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/subagent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	capdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintool "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	glossary "github.com/Nyukimin/picoclaw_multiLLM/internal/glossary"
	capinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	executionpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
	memorypersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/memory"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
	toolregistry "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/toolregistry"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persona"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
	securityinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/security"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

// Dependencies はアプリケーション依存関係
type Dependencies struct {
	lineHandler          http.Handler
	telegramHandler      http.Handler
	discordHandler       http.Handler
	slackHandler         http.Handler
	eventHub             *viewer.EventHub                       // live viewer
	monitorStore         *viewer.MonitorStore                   // viewer monitor snapshots
	eventLogStore        *viewer.EventLogStore                  // persisted orchestrator event log
	eventLogGC           *viewer.EventLogGCService              // persisted event log GC
	reportStore          *executionpersistence.JSONLReportStore // execution evidence store
	eventRelay           *idleAwareEventListener                // viewer + idlechat stop relay
	viewerStatus         http.HandlerFunc                       // viewer status API
	viewerAgents         http.HandlerFunc                       // viewer agents API
	viewerAgentDetail    http.HandlerFunc                       // viewer agent detail API
	viewerJobs           http.HandlerFunc                       // viewer jobs API
	viewerLogs           http.HandlerFunc                       // viewer logs API
	viewerAuditSummary   http.HandlerFunc                       // viewer audit summary API
	viewerJobDetail      http.HandlerFunc                       // viewer job detail API
	viewerSend           http.HandlerFunc                       // viewer message sender
	evidenceHandler      http.HandlerFunc                       // viewer evidence API
	evidenceDetail       http.HandlerFunc                       // viewer evidence detail API
	evidenceSummary      http.HandlerFunc                       // viewer evidence summary API
	glossaryRecent       http.HandlerFunc                       // viewer glossary API
	viewerMemorySnapshot http.HandlerFunc                       // viewer memory/news/recall API
	viewerMemoryLayers   http.HandlerFunc                       // viewer memory layer API
	viewerMemoryEvents   http.HandlerFunc                       // viewer L1 event/search cache API
	viewerMemoryState    http.HandlerFunc                       // viewer memory state API
	viewerMemoryPromote  http.HandlerFunc                       // viewer memory promote API
	viewerRecallTraces   http.HandlerFunc                       // viewer recall trace API
	viewerSourceRegistry http.HandlerFunc                       // viewer source registry API
	entryHandler         http.HandlerFunc                       // unified entry endpoint
	chromeBridge         http.HandlerFunc                       // chrome bridge endpoint
	chromeBridgeStatus   http.HandlerFunc                       // chrome bridge status endpoint
	chromeBridgeEvents   http.HandlerFunc                       // chrome bridge SSE endpoint
	distOrch             *orchestrator.DistributedOrchestrator  // v4 distributed orchestrator
	router               *transport.MessageRouter               // v4 distributed mode
	localTransports      map[string]*transport.LocalTransport   // v4 local transports
	idleChatOrch         *idlechat.IdleChatOrchestrator         // v4 idle chat
	idleChatStartGate    idleChatStartGate                      // IdleChat 起動前の LLM Ops ガード
	sshTransports        map[string]domaintransport.Transport   // v4 SSH transports
	heartbeatSvc         *heartbeat.HeartbeatService            // heartbeat service
	toolRegistry         capdomain.ToolRegistry                 // Phase 4: Shiro ツール共有用 ToolRegistry
}

type idleChatStartGate interface {
	PrepareIdleChatStart(context.Context) error
}

// Shutdown はリソースを解放
func (d *Dependencies) Shutdown() {
	if d.eventLogGC != nil {
		d.eventLogGC.Stop()
	}
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
	for name, t := range d.localTransports {
		if err := t.Close(); err != nil {
			log.Printf("Failed to close Local transport for %s: %v", name, err)
		}
	}
	if d.router != nil {
		d.router.Stop()
	}
	if d.toolRegistry != nil {
		if err := d.toolRegistry.Close(); err != nil {
			log.Printf("Failed to close ToolRegistry: %v", err)
		}
	}
	log.Println("Shutdown complete")
}

// buildDependencies は依存関係を構築
func buildDependencies(cfg *config.Config) *Dependencies {
	// 0. ケイパビリティ検出（v4.1）
	var nodeCaps capdomain.NodeCapabilities

	// ToolRegistry 初期化（ProbeLLMs に関係なくランタイムで使用）
	var runtimeToolRegistry capdomain.ToolRegistry
	if cfg.Capability.ToolRegistryDB != "" {
		tr, err := toolregistry.NewDuckDBToolRegistryStore(cfg.Capability.ToolRegistryDB)
		if err != nil {
			log.Printf("WARN: ToolRegistry init failed (%s): %v", cfg.Capability.ToolRegistryDB, err)
		} else {
			runtimeToolRegistry = tr
			log.Printf("ToolRegistry initialized: %s", cfg.Capability.ToolRegistryDB)
		}
	}

	if cfg.Capability.ProbeLLMs {
		detector := capinfra.NewCapabilityDetector(cfg)
		if runtimeToolRegistry != nil {
			detector = detector.WithToolRegistry(runtimeToolRegistry)
		}
		caps, err := detector.Detect(context.Background())
		if err != nil {
			log.Printf("WARN: capability detection failed: %v", err)
		} else {
			nodeCaps = caps
			profile := capdomain.DetermineProfile(caps)
			log.Printf("Node capabilities: profile=%s llms=%d tools=%d memory=%dMB/%dMB os=%s/%s",
				profile, len(caps.LLMs), len(caps.Tools),
				caps.Memory.AvailableMB, caps.Memory.TotalMB,
				caps.Platform.OS, caps.Platform.Arch)
			for _, l := range caps.LLMs {
				log.Printf("  LLM: provider=%s model=%s available=%v quality=%d",
					l.ProviderName, l.ModelName, l.Available, l.Quality)
			}
		}
	}

	// 1. LLM Provider
	primaryProviders := buildPrimaryLLMProviders(cfg)
	chatProvider := primaryProviders.Chat
	workerProvider := primaryProviders.Worker
	heavyProvider := primaryProviders.Heavy
	wildProvider := primaryProviders.Wild
	workerToolProvider, ok := workerProvider.(llm.ToolCallingProvider)
	if !ok {
		log.Fatalf("worker provider %s does not support tool calling", workerProvider.Name())
	}

	// v4.1: Unified Coder setup using LLM Factory and Agent Persona
	coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter := setupCoders(cfg)

	// 2. Routing Components
	classifier := routing.NewLLMClassifier(chatProvider, cfg.Prompts.Classifier)
	ruleDictionary := routing.NewRuleDictionary()

	// 3. Tool Runner（Chat用とWorker用で分離）
	// 全エージェントのペルソナファイルを Chat から編集可能にする
	personaWritePaths := []string{
		filepath.Join(cfg.WorkspaceDir, "persona", "mio.md"),
		filepath.Join(cfg.WorkspaceDir, "persona", "shiro.md"),
		filepath.Join(cfg.WorkspaceDir, "persona", "aka.md"),
		filepath.Join(cfg.WorkspaceDir, "persona", "ao.md"),
		filepath.Join(cfg.WorkspaceDir, "persona", "gin.md"),
		filepath.Join(cfg.WorkspaceDir, "persona", "kin.md"),
	}
	chatToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:         cfg.GoogleSearchChat.APIKey,
		GoogleSearchEngineID: cfg.GoogleSearchChat.SearchEngineID,
		AllowedWritePaths:    personaWritePaths,
	}
	workerToolRunnerCfg := tools.ToolRunnerConfig{
		GoogleAPIKey:         cfg.GoogleSearchWorker.APIKey,
		GoogleSearchEngineID: cfg.GoogleSearchWorker.SearchEngineID,
		ToolRegistry:         runtimeToolRegistry,
		WorkspaceDir:         cfg.WorkspaceDir,
	}

	chatToolRunnerV2 := tools.NewToolRunner(chatToolRunnerCfg)
	workerToolRunnerV2 := tools.NewToolRunner(workerToolRunnerCfg)

	// Subagent配線（2段階構築: ToolRunner作成後にManagerを注入）
	var subagentMgr *subagentapp.Manager
	if cfg.Subagent.Enabled {
		subagentProvider := resolveSubagentProvider(cfg, workerToolProvider)
		toolDefs := workerToolRunnerV2.ToolDefinitions()

		subagentOpts := []subagentapp.ManagerOption{}
		if runtimeToolRegistry != nil {
			subagentOpts = append(subagentOpts, subagentapp.WithToolRegistry(runtimeToolRegistry))
		}
		subagentMgr = subagentapp.NewManager(
			subagentProvider,
			workerToolRunnerV2,
			toolDefs,
			toolloop.Config{MaxIterations: cfg.Subagent.MaxIterations},
			subagentOpts...,
		)

		workerToolRunnerV2.RegisterSubagent("worker", tools.NewSubagentFuncFromManager(subagentMgr))
		log.Printf("Subagent enabled (provider: %s, max_iterations: %d)",
			subagentProvider.Name(), cfg.Subagent.MaxIterations)
	} else {
		log.Printf("Subagent disabled")
	}

	// Security policy wrapper（enabled 時のみ）
	var chatRunnerV2 domaintool.RunnerV2 = chatToolRunnerV2
	var workerRunnerV2 domaintool.RunnerV2 = workerToolRunnerV2
	if cfg.Security.Enabled {
		var execRepo domainexecution.Repository
		if cfg.Security.Audit.Enabled && cfg.Security.Audit.Backend == "jsonl" {
			repo, err := executionpersistence.NewJSONLRepository(cfg.Security.Audit.Path)
			if err != nil {
				log.Fatalf("Failed to initialize execution audit repository: %v", err)
			}
			execRepo = repo
		}

		policy := securityinfra.NewPolicyEngine(securityinfra.PolicyConfig{
			Mode:              cfg.Security.PolicyMode,
			NetworkScope:      cfg.Security.NetworkScope,
			NetworkAllowed:    cfg.Security.NetworkAllowlist,
			DenyCommands:      cfg.Security.DenyCommands,
			Workspace:         cfg.WorkspaceDir,
			WorkspaceEnforced: cfg.Security.WorkspaceEnforced,
		})

		securedChatRunner, err := securityinfra.NewPolicyRunner(chatToolRunnerV2, policy, execRepo, "chat")
		if err != nil {
			log.Fatalf("Failed to create chat policy runner: %v", err)
		}
		securedWorkerRunner, err := securityinfra.NewPolicyRunner(workerToolRunnerV2, policy, execRepo, "worker")
		if err != nil {
			log.Fatalf("Failed to create worker policy runner: %v", err)
		}
		chatRunnerV2 = securedChatRunner
		workerRunnerV2 = securedWorkerRunner
		log.Printf("Security policy runner enabled (mode=%s)", cfg.Security.PolicyMode)
	}

	// Phase 4: CompositeRunnerV2（ToolRegistry フォールバック）
	if runtimeToolRegistry != nil {
		workerRunnerV2 = tools.NewCompositeRunnerV2(workerRunnerV2, runtimeToolRegistry, cfg.WorkspaceDir)
		log.Printf("CompositeRunnerV2 enabled (ToolRegistry fallback for worker)")
	}

	// LegacyRunner アダプター（V2 → V1 ブリッジ）で agents に注入
	chatToolRunner := domaintool.NewLegacyRunner(chatRunnerV2)
	workerToolRunner := domaintool.NewLegacyRunner(workerRunnerV2)
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
	var l1Store *conversationpersistence.L1SQLiteStore
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
		if cfg.Conversation.L1SQLitePath != "" {
			if err := os.MkdirAll(filepath.Dir(cfg.Conversation.L1SQLitePath), 0755); err != nil {
				log.Fatalf("Failed to create L1 SQLite directory: %v", err)
			}
			l1Store, err = conversationpersistence.NewL1SQLiteStore(cfg.Conversation.L1SQLitePath)
			if err != nil {
				log.Fatalf("Failed to initialize L1 SQLite store: %v", err)
			}
			realMgr.WithL1Store(l1Store)
			log.Printf("  L1 SQLite: %s", cfg.Conversation.L1SQLitePath)
		}

		// Embedder注入（embed_model が設定されている場合）
		embedder, embedderLabel := buildConversationEmbedder(cfg)
		if embedder != nil {
			realMgr.WithEmbedder(embedder)
			log.Printf("  Embedder: %s", embedderLabel)
		}

		// Summarizer注入（local_llm有効時はWorker provider、従来構成ではOllama summary_model）
		summaryProvider, summaryProviderLabel := buildConversationTextProvider(cfg, primaryProviders)
		if summaryProvider != nil {
			summarizer := conversationpersistence.NewLLMSummarizer(summaryProvider)
			realMgr.WithSummarizer(summarizer)
			if l1Store != nil {
				l1Store.WithDailyDigestSummarizer(conversationpersistence.NewLLMDailyDigestSummarizer(summaryProvider))
			}
			log.Printf("  Summarizer: %s", summaryProviderLabel)
		}

		// スレッド境界検出器（Embedder があれば類似度チェックも有効化）
		var embedderForDetector conversation.EmbeddingProvider
		embedderForDetector = embedder
		detector := conversationpersistence.NewThreadBoundaryDetector(embedderForDetector)

		// ProfileExtractor（summary_model を再利用）
		var profileExtractor conversation.ProfileExtractor
		if summaryProvider != nil {
			profileExtractor = conversationpersistence.NewLLMProfileExtractor(summaryProvider)
			log.Printf("  ProfileExtractor: %s", summaryProviderLabel)
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
	if realMgr != nil {
		webSearchCache := newConversationWebSearchCacheAdapter(realMgr)
		chatToolRunnerV2.WithWebSearchCache(webSearchCache)
		workerToolRunnerV2.WithWebSearchCache(webSearchCache)
		log.Printf("ToolRunner web_search cache enabled via Conversation L1")
	}
	if l1Store != nil {
		startSourceRegistrySweeper(l1Store)
	}
	if realMgr != nil {
		startParquetExportJob(realMgr)
	}

	// 5. Memory Store（HeartbeatService用。Mio会話メモリはConversationEngine v5.1が担当）
	memStore := memorypersistence.NewFileStore(cfg.WorkspaceDir)
	log.Printf("MemoryStore initialized (workspace: %s)", cfg.WorkspaceDir)

	var recentGlossaryContext func(context.Context, int) (string, error)
	var recentGlossaryTopics func(context.Context, int) ([]string, error)
	var glossaryRecentHandler http.HandlerFunc
	if cfg.Glossary.Enabled {
		dbPath := cfg.Glossary.DBPath
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			log.Printf("WARN: glossary directory create failed: %v", err)
		} else if glossaryModule, err := glossary.NewGlossaryModule(dbPath); err != nil {
			log.Printf("WARN: glossary disabled: %v", err)
		} else {
			syncGlossary := func() {
				count, err := glossaryModule.SyncFeeds(context.Background(), cfg.Glossary.FeedURLs)
				if err != nil {
					log.Printf("WARN: glossary sync failed: %v", err)
					return
				}
				log.Printf("Glossary sync complete: %d items", count)
			}
			syncGlossary()
			if cfg.Glossary.RefreshIntervalHr > 0 {
				go func(interval time.Duration) {
					ticker := time.NewTicker(interval)
					defer ticker.Stop()
					for range ticker.C {
						syncGlossary()
					}
				}(time.Duration(cfg.Glossary.RefreshIntervalHr) * time.Hour)
			}
			recentGlossaryContext = glossaryModule.MioAdapter.GetRecentContext
			recentGlossaryTopics = glossaryModule.MioAdapter.GetRecentTopics
			glossaryRecentHandler = viewer.HandleGlossaryRecent(glossaryModule.Service)
			log.Printf("Glossary enabled: db=%s feeds=%d", dbPath, len(cfg.Glossary.FeedURLs))
		}
	}

	// 6. Agents
	mioAgent := agent.NewMioAgent(chatProvider, classifier, ruleDictionary, chatToolRunner, mcpClient, convEngine).
		WithSystemPrompt(cfg.Prompts.MioPersona)
	if recentGlossaryContext != nil {
		mioAgent = mioAgent.WithRecentContextProvider(recentGlossaryContext)
		log.Printf("Mio: Glossary context injected")
	}
	if realMgr != nil {
		mioAgent = mioAgent.WithKBManager(realMgr)
		log.Printf("Mio: KBManager injected (KB autosave enabled)")
	}
	mioPersonaFile := filepath.Join(cfg.WorkspaceDir, "persona", "mio.md")
	if cfg.MioPersonaFile != "" {
		mioPersonaFile = filepath.Join(cfg.WorkspaceDir, cfg.MioPersonaFile)
	}
	personaEditor := persona.NewFilePersonaEditor(mioPersonaFile)
	mioAgent = mioAgent.WithPersonaEditor(personaEditor)
	log.Printf("Mio: PersonaEditor injected (file: %s)", mioPersonaFile)
	// 設計契約: subagent が無効なときは interface へ型付き nil を渡さない
	var shiroSubagentManager agent.SubagentManager
	if subagentMgr != nil {
		shiroSubagentManager = subagentMgr
	}
	shiroAgent := agent.NewShiroAgent(workerProvider, workerToolRunner, mcpClient, cfg.Prompts.Worker, shiroSubagentManager)
	heavyAgent := agent.NewHeavyAgent(heavyProvider, cfg.Prompts.Heavy)
	wildAgent := agent.NewWildAgent(wildProvider, cfg.Prompts.Wild)
	if convEngine != nil {
		shiroAgent.WithConversationEngine(convEngine)
		heavyAgent.WithConversationEngine(convEngine)
		wildAgent.WithConversationEngine(convEngine)
	}
	if cfg.Worker.PersonaFile != "" {
		if content, ok := config.LoadPersonaFile(cfg.WorkspaceDir, cfg.Worker.PersonaFile); ok {
			shiroPersona := agent.AgentPersona{
				Name:        "Shiro",
				Personality: content,
				Tone:        cfg.Worker.Tone,
			}
			shiroAgent.WithPersona(shiroPersona)
			log.Printf("Shiro: persona loaded from %s", cfg.Worker.PersonaFile)
		}
	}

	// 7. Session Repository
	sessionRepo := session.NewJSONSessionRepository(cfg.Session.StorageDir)
	centralMemory := domainsession.NewCentralMemory()

	// セッションディレクトリ作成
	if err := os.MkdirAll(cfg.Session.StorageDir, 0755); err != nil {
		log.Fatalf("Failed to create session directory: %v", err)
	}

	// 8. Worker Execution Service
	workerExecutionService := service.NewWorkerExecutionService(cfg.Worker)
	log.Printf("WorkerExecutionService initialized (Workspace: %s, Parallel: %v)",
		cfg.Worker.Workspace, cfg.Worker.ParallelExecution)

	deps := &Dependencies{}
	deps.glossaryRecent = glossaryRecentHandler
	if l1Store != nil {
		deps.viewerMemorySnapshot = viewer.HandleMemorySnapshot(l1Store)
		deps.viewerMemoryLayers = viewer.HandleMemoryLayers(l1Store, realMgr)
		deps.viewerMemoryEvents = viewer.HandleMemoryEvents(l1Store)
		deps.viewerMemoryState = viewer.HandleMemoryState(l1Store)
		deps.viewerMemoryPromote = viewer.HandleMemoryPromote(l1Store)
		deps.viewerRecallTraces = viewer.HandleRecallTraces(l1Store)
		deps.viewerSourceRegistry = viewer.HandleSourceRegistry(l1Store)
	}
	deps.toolRegistry = runtimeToolRegistry

	// EventHub (Live Viewer)
	hub := viewer.NewEventHub(200)
	deps.eventHub = hub
	if cfg.ViewerLog.Enabled {
		eventLogPath := cfg.ViewerLog.Path
		if eventLogStore, err := viewer.NewEventLogStore(eventLogPath); err != nil {
			log.Printf("WARN: viewer event log disabled: %v", err)
		} else {
			deps.eventLogStore = eventLogStore
			log.Printf("Viewer event log enabled: %s", eventLogPath)
			gcPath := filepath.Join(filepath.Dir(eventLogPath), "orchestrator_event_gc.jsonl")
			if gcSvc, err := viewer.NewEventLogGCService(eventLogStore, gcPath, cfg.ViewerLog.RetentionDays, cfg.ViewerLog.GCIntervalMinutes); err != nil {
				log.Printf("WARN: viewer event log GC disabled: %v", err)
			} else {
				deps.eventLogGC = gcSvc
				deps.eventLogGC.Start()
				log.Printf("Viewer event log GC enabled: %s", gcPath)
			}
		}
	}
	reportPath := defaultExecutionReportPath(cfg.WorkspaceDir)
	ttsRuntime := buildTTSEntryRuntime(cfg)
	vtuberBridge := buildVTuberBridge(cfg)
	lipSync := newTTSVTuberLipSync(vtuberBridge)
	ttsBridge := buildTTSClientBridge(
		cfg,
		func(ev orchestrator.OrchestratorEvent) {
			if deps.eventRelay != nil {
				deps.eventRelay.OnEvent(ev)
			}
		},
		func(sessionID, characterID, text string) {
			if lipSync != nil {
				lipSync.OnChunkReady(sessionID, characterID, text)
			}
		},
		func(sessionID, characterID string) {
			if lipSync != nil {
				lipSync.OnSessionCompleted(sessionID, characterID)
			}
		},
	)
	if reportStore, err := executionpersistence.NewJSONLReportStore(reportPath); err != nil {
		deps.monitorStore = viewer.NewMonitorStore(nil, deps.eventLogStore)
		deps.eventRelay = &idleAwareEventListener{hub: hub, monitor: deps.monitorStore, archive: deps.eventLogStore}
		deps.viewerStatus = viewer.HandleMonitorStatus(deps.monitorStore)
		deps.viewerAgents = viewer.HandleMonitorAgents(deps.monitorStore)
		deps.viewerAgentDetail = viewer.HandleMonitorAgentDetail(deps.monitorStore)
		deps.viewerJobs = viewer.HandleMonitorJobs(deps.monitorStore)
		deps.viewerLogs = viewer.HandleMonitorLogs(deps.monitorStore)
		deps.viewerAuditSummary = viewer.HandleMonitorAuditSummary(deps.monitorStore)
		deps.viewerJobDetail = viewer.HandleMonitorJobDetail(deps.monitorStore)
		log.Printf("WARN: evidence API disabled: %v", err)
	} else {
		deps.reportStore = reportStore
		deps.monitorStore = viewer.NewMonitorStore(reportStore, deps.eventLogStore)
		deps.eventRelay = &idleAwareEventListener{hub: hub, monitor: deps.monitorStore, archive: deps.eventLogStore}
		deps.viewerStatus = viewer.HandleMonitorStatus(deps.monitorStore)
		deps.viewerAgents = viewer.HandleMonitorAgents(deps.monitorStore)
		deps.viewerAgentDetail = viewer.HandleMonitorAgentDetail(deps.monitorStore)
		deps.viewerJobs = viewer.HandleMonitorJobs(deps.monitorStore)
		deps.viewerLogs = viewer.HandleMonitorLogs(deps.monitorStore)
		deps.viewerAuditSummary = viewer.HandleMonitorAuditSummary(deps.monitorStore)
		deps.viewerJobDetail = viewer.HandleMonitorJobDetail(deps.monitorStore)
		deps.evidenceHandler = viewer.HandleEvidenceRecent(reportStore)
		deps.evidenceDetail = viewer.HandleEvidenceDetail(reportStore)
		deps.evidenceSummary = viewer.HandleEvidenceSummary(reportStore)
		log.Printf("Viewer evidence API enabled: %s", reportPath)
	}

	// NI-003: ToolRegistry エラーを SSE でユーザーに通知する
	if subagentMgr != nil && deps.eventRelay != nil {
		subagentMgr.SetRegistryErrorHandler(func(err error) {
			deps.eventRelay.OnEvent(orchestrator.NewEvent(
				"registry.error", "system", "subagent", err.Error(),
				"", "", "system", "system", "system",
			))
		})
	}

	// viewerSendFromOrch はオーケストレーター共通のviewer送信ハンドラを生成
	viewerSendFromOrch := func(proc messageProcessor) http.HandlerFunc {
		attachmentStore := attachmentapp.NewStore(cfg.WorkspaceDir)
		return viewer.HandleSendWithAttachments(func(ctx context.Context, req viewer.SendRequest) (string, error) {
			log.Printf("[main] viewerSendFromOrch: calling ProcessMessage for viewer message: %q attachments=%d", req.Message, len(req.Attachments))
			resp, err := proc.ProcessMessage(ctx, orchestrator.ProcessMessageRequest{
				SessionID:   "viewer",
				Channel:     "viewer",
				ChatID:      "viewer-user",
				UserMessage: req.Message,
				Attachments: req.Attachments,
			})
			if err != nil {
				log.Printf("[main] viewerSendFromOrch: ProcessMessage error: %v", err)
				return "", err
			}
			log.Printf("[main] viewerSendFromOrch: ProcessMessage completed, route=%s jobID=%s", resp.Route, resp.JobID)
			return resp.Response, nil
		}, func(err error) {
			if deps.eventRelay != nil {
				deps.eventRelay.OnEvent(orchestrator.NewEvent(
					"viewer.error", "system", "viewer", err.Error(),
					"", "", "viewer", "viewer", "viewer-user",
				))
			}
		}, attachmentStore)
	}
	entryFromOrch := func(proc messageProcessor) http.HandlerFunc {
		return entryadapter.HandleWithObserver(
			func(ctx context.Context, req entryadapter.Request) (entryadapter.Result, error) {
				return processEntryRequestWithRuntime(ctx, proc, req, reportPath, ttsRuntime)
			},
			func(ctx context.Context, stage entryadapter.Stage, req entryadapter.Request, result *entryadapter.Result, err error) {
				route := ""
				jobID := ""
				if result != nil {
					route = result.Route
					jobID = result.JobID
				}
				if deps.eventRelay != nil {
					deps.eventRelay.OnEvent(orchestrator.NewEvent(
						"entry.stage",
						req.Platform,
						"system",
						string(stage),
						route,
						jobID,
						req.SessionID,
						req.Channel,
						req.UserID,
					))
				}
				switch stage {
				case entryadapter.StageReceived:
					log.Printf("[entry] stage=%s channel=%s user=%s session=%s", stage, req.Channel, req.UserID, req.SessionID)
				case entryadapter.StagePlanning:
					log.Printf("[entry] stage=%s session=%s", stage, req.SessionID)
				case entryadapter.StageCompleted:
					log.Printf("[entry] stage=%s session=%s route=%s job=%s", stage, req.SessionID, route, jobID)
				case entryadapter.StageFailed:
					log.Printf("[entry] stage=%s session=%s err=%v", stage, req.SessionID, err)
				default:
					log.Printf("[entry] stage=%s session=%s", stage, req.SessionID)
				}
			},
		)
	}
	chromeBridgeFromOrch := func(proc messageProcessor) (http.HandlerFunc, http.HandlerFunc, http.HandlerFunc) {
		bridge := chromeadapter.HandleBridge(func(ctx context.Context, req entryadapter.Request) (entryadapter.Result, error) {
			return processEntryRequestWithRuntime(ctx, proc, req, reportPath, ttsRuntime)
		})
		status := chromeadapter.HandleBridgeStatus(func() []orchestrator.OrchestratorEvent {
			if deps.eventHub == nil {
				return nil
			}
			return deps.eventHub.History()
		}, func() time.Time {
			return time.Now().UTC()
		})
		events := chromeadapter.HandleBridgeEvents(deps.eventHub)
		return bridge, status, events
	}

	// 9. IdleChat（有効な場合）
	if cfg.IdleChat.Enabled {
		idleChatOrch := idlechat.NewIdleChatOrchestrator(
			chatProvider,
			centralMemory,
			cfg.IdleChat.Participants,
			cfg.IdleChat.IntervalMin,
			cfg.IdleChat.MaxTurns,
			cfg.IdleChat.Temperature,
			config.BuildIdleChatAgentPrompts(cfg.Prompts),
			cfg.IdleChat.StoryDataDir,
		)
		idleChatOrch.SetIntervalSeconds(cfg.IdleChat.IntervalSec)
		idleChatOrch.SetSpeakerProviders(map[string]llm.LLMProvider{
			"mio":   chatProvider,
			"shiro": workerProvider,
			"kuro":  heavyProvider,
			"wild":  wildProvider,
		})
		// v4.1: OpenAI provider を coder2 から取得（Forecast用）
		if coder2Adapter != nil && cfg.Coder2.Provider == "openai" {
			// coder2Adapter.domainCoder から LLMProvider を取得
			// Note: CoderAgent は内部的に LLMProvider を持つが、直接アクセスできない
			// 代わりに、Config から再度 OpenAI Provider を作成
			if cfg.Coder2.APIKey != "" {
				openaiProvider := openai.NewOpenAIProvider(cfg.Coder2.APIKey, cfg.Coder2.Model)
				idleChatOrch.SetForecastProvider(openaiProvider)
				idleChatOrch.InitForecastTopicStock(filepath.Join(cfg.Session.StorageDir, "forecast_topic_stock.json"))
				log.Printf("IdleChat: Forecast provider set to OpenAI (Coder2: %s), topic stock filling", cfg.Coder2.Model)
			}
		}
		if recentGlossaryTopics != nil {
			idleChatOrch.SetRecentTopicProvider(recentGlossaryTopics)
			log.Printf("IdleChat: Glossary topics injected")
		}
		topicStorePath := filepath.Join(cfg.Session.StorageDir, "idlechat_topics.jsonl")
		if err := idleChatOrch.SetTopicStore(topicStorePath); err != nil {
			log.Printf("WARN: idleChat topic store disabled: %v", err)
		} else {
			log.Printf("IdleChat topic store enabled: %s", topicStorePath)
		}
		if deps.eventHub != nil {
			idleChatOrch.SetEventEmitter(func(ev idlechat.TimelineEvent) <-chan struct{} {
				// "idlechat.tts" は TTS 専用 — Viewer には送らない
				// "idlechat.viewer" は段落表示専用 — "idlechat.message" としてViewerに送る
				if ev.Type != "idlechat.tts" {
					viewerType := ev.Type
					if viewerType == "idlechat.viewer" {
						viewerType = "idlechat.message"
					}
					chatID := strings.TrimSpace(ev.SessionID)
					if chatID == "" {
						chatID = "idlechat"
					}
					viewerEvent := orchestrator.NewEvent(
						viewerType,
						ev.From,
						ev.To,
						ev.Content,
						"IDLECHAT",
						"",
						ev.SessionID,
						"idlechat",
						chatID,
					)
					viewerEvent.RawContent = ev.RawContent
					deps.eventHub.OnEvent(viewerEvent)
				}
				// "idlechat.viewer" は Viewer 専用 — TTS には送らない
				if ev.Type == "idlechat.viewer" {
					return nil
				}
				return emitIdleChatTTSAsync(ttsBridge, ev)
			})
		}
		if deps.eventRelay != nil {
			deps.eventRelay.SetIdleChat(idleChatOrch)
		}
		idleChatOrch.Start()
		deps.idleChatOrch = idleChatOrch
		log.Printf("IdleChat enabled (participants=%v)", cfg.IdleChat.Participants)
	}

	// 10. v3/v4 モード分岐
	if cfg.Distributed.Enabled {
		log.Println("=== v4 Distributed Mode ===")
		deps.buildDistributedMode(cfg, sessionRepo, mioAgent, shiroAgent, heavyAgent, wildAgent, coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter, workerExecutionService, chatProvider, centralMemory, ttsBridge, vtuberBridge)
		deps.viewerSend = viewerSendFromOrch(deps.distOrch)
		deps.entryHandler = entryFromOrch(deps.distOrch)
		deps.chromeBridge, deps.chromeBridgeStatus, deps.chromeBridgeEvents = chromeBridgeFromOrch(deps.distOrch)
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
			coder4Adapter,
			workerExecutionService,
		)
		// Phase 3: 動的コーダー選択を注入
		if coderCaps := buildCoderCapabilities(nodeCaps, cfg); coderCaps != nil {
			orch.SetCoderCapabilities(coderCaps)
			log.Printf("Dynamic coder selection enabled (%d coders)", len(coderCaps))
		}
		orch.SetEventListener(deps.eventRelay)
		if deps.reportStore != nil {
			orch.SetReportStore(deps.reportStore)
		}
		orch.SetMaxRepair(cfg.Worker.MaxRepair)
		orch.SetWildAgent(wildAgent)
		orch.SetHeavyAgent(heavyAgent)
		orch.SetTTSBridge(ttsBridge)
		orch.SetVTuberBridge(vtuberBridge)
		// IdleChat統合（有効な場合）
		if deps.idleChatOrch != nil {
			orch.SetIdleNotifier(deps.idleChatOrch)
			log.Printf("IdleChat integrated with MessageOrchestrator")
		}
		deps.lineHandler = line.NewHandler(orch, cfg.Line.ChannelSecret, cfg.Line.AccessToken)
		if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
			tg := telegramadapter.NewAdapter(cfg.Telegram.BotToken, orch)
			tg.SetWebhookSecret(cfg.Telegram.WebhookSecret)
			deps.telegramHandler = tg
		}
		if strings.TrimSpace(cfg.Discord.BotToken) != "" {
			dc := discordadapter.NewAdapter(cfg.Discord.BotToken, orch)
			dc.SetPublicKeyHex(cfg.Discord.PublicKey)
			deps.discordHandler = dc
		}
		if strings.TrimSpace(cfg.Slack.BotToken) != "" {
			deps.slackHandler = slackadapter.NewAdapter(cfg.Slack.BotToken, cfg.Slack.SigningSecret, orch)
		}
		deps.viewerSend = viewerSendFromOrch(orch)
		deps.entryHandler = entryFromOrch(orch)
		deps.chromeBridge, deps.chromeBridgeStatus, deps.chromeBridgeEvents = chromeBridgeFromOrch(orch)
	}

	// 10. Heartbeat Service
	if cfg.Heartbeat.Enabled {
		heartbeatSvc := heartbeat.NewHeartbeatService(
			mioAgent,
			buildHeartbeatNotificationSender(cfg),
			cfg.WorkspaceDir,
			cfg.Heartbeat.Interval,
		)
		heartbeatSvc.WithMemoryStore(memStore)
		heartbeatSvc.WithEventListener(deps.eventRelay)
		heartbeatSvc.Start()
		deps.heartbeatSvc = heartbeatSvc
		log.Printf("HeartbeatService enabled (interval: %dm, workspace: %s)", cfg.Heartbeat.Interval, cfg.WorkspaceDir)
	}

	log.Println("Dependency injection complete")
	return deps
}
