package main

import (
	"context"
	"log"
	"net/http"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/heartbeat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	capdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	executionpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
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
	verificationRecent   http.HandlerFunc                       // viewer verification recent API
	verificationDetail   http.HandlerFunc                       // viewer verification detail API
	verificationSummary  http.HandlerFunc                       // viewer verification summary API
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
	runtimeToolRegistry := buildRuntimeToolRegistry(cfg)
	nodeCaps := buildCapabilityRuntime(cfg, runtimeToolRegistry)
	llmRuntime := buildLLMRuntimeProviders(cfg)
	classifier := routing.NewLLMClassifier(llmRuntime.Chat, cfg.Prompts.Classifier)
	ruleDictionary := routing.NewRuleDictionary()
	toolRuntime := buildToolRuntime(cfg, llmRuntime.WorkerToolProvider, runtimeToolRegistry)
	mcpClient := mcp.NewMCPClient()
	log.Printf("MCPClient initialized with %d servers", len(mcpClient.ListServers()))
	conversationRuntime := buildConversationRuntime(cfg, llmRuntime.Primary, toolRuntime.ChatRunnerV2, toolRuntime.WorkerRunnerV2)
	glossaryRuntime := buildGlossaryRuntime(cfg)
	agents := buildAgentRuntime(
		cfg,
		llmRuntime.Chat,
		llmRuntime.Worker,
		llmRuntime.Heavy,
		llmRuntime.Wild,
		classifier,
		ruleDictionary,
		toolRuntime.ChatLegacy,
		toolRuntime.WorkerLegacy,
		mcpClient,
		conversationRuntime.Engine,
		glossaryRuntime.RecentContext,
		conversationRuntime.Manager,
		toolRuntime.SubagentMgr,
	)
	sessionRuntime := buildSessionRuntime(cfg)
	workerExecutionService := service.NewWorkerExecutionService(cfg.Worker)
	log.Printf("WorkerExecutionService initialized (Workspace: %s, Parallel: %v)",
		cfg.Worker.Workspace, cfg.Worker.ParallelExecution)

	deps := &Dependencies{}
	deps.glossaryRecent = glossaryRuntime.RecentHandler
	deps.toolRegistry = runtimeToolRegistry
	reportPath := defaultExecutionReportPath(cfg.WorkspaceDir)
	buildViewerRuntimeHandlers(cfg, deps, conversationRuntime.L1Store, conversationRuntime.Manager, reportPath)
	verificationRuntime := buildVerificationRuntime(cfg, deps, conversationRuntime.L1Store)

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

	// NI-003: ToolRegistry エラーを SSE でユーザーに通知する
	if toolRuntime.SubagentMgr != nil && deps.eventRelay != nil {
		toolRuntime.SubagentMgr.SetRegistryErrorHandler(func(err error) {
			deps.eventRelay.OnEvent(orchestrator.NewEvent(
				"registry.error", "system", "subagent", err.Error(),
				"", "", "system", "system", "system",
			))
		})
	}

	bridges := buildViewerBridgeHandlers(cfg, deps, reportPath, ttsRuntime)
	buildIdleChatRuntime(
		cfg,
		deps,
		llmRuntime.Chat,
		llmRuntime.Worker,
		llmRuntime.Heavy,
		llmRuntime.Wild,
		sessionRuntime.CentralMemory,
		llmRuntime.Coder2,
		glossaryRuntime.RecentTopics,
		ttsBridge,
	)
	buildOrchestratorRuntime(
		cfg,
		deps,
		sessionRuntime.SessionRepo,
		agents,
		llmRuntime,
		workerExecutionService,
		nodeCaps,
		sessionRuntime.CentralMemory,
		ttsBridge,
		vtuberBridge,
		bridges,
		verificationRuntime,
	)
	buildHeartbeatRuntime(cfg, deps, agents.Mio, sessionRuntime.MemoryStore)

	log.Println("Dependency injection complete")
	return deps
}
