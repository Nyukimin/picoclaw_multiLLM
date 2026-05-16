package main

import (
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	capdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func buildOrchestratorRuntime(
	cfg *config.Config,
	deps *Dependencies,
	sessionRepo orchestrator.SessionRepository,
	agents agentRuntime,
	llmRuntime llmRuntimeProviders,
	workerExecutionService service.WorkerExecutionService,
	nodeCaps capdomain.NodeCapabilities,
	centralMemory *domainsession.CentralMemory,
	ttsBridge orchestrator.TTSBridge,
	vtuberBridge orchestrator.VTuberBridge,
	bridges viewerBridgeFactories,
) {
	if cfg.Distributed.Enabled {
		log.Println("=== v4 Distributed Mode ===")
		deps.buildDistributedMode(
			cfg,
			sessionRepo,
			agents.Mio,
			agents.Shiro,
			agents.Heavy,
			agents.Wild,
			llmRuntime.Coder1,
			llmRuntime.Coder2,
			llmRuntime.Coder3,
			llmRuntime.Coder4,
			workerExecutionService,
			llmRuntime.Chat,
			centralMemory,
			ttsBridge,
			vtuberBridge,
		)
		deps.viewerSend = bridges.ViewerSendFromOrch(deps.distOrch)
		deps.entryHandler = bridges.EntryFromOrch(deps.distOrch)
		deps.chromeBridge, deps.chromeBridgeStatus, deps.chromeBridgeEvents = bridges.ChromeBridgeFromOrch(deps.distOrch)
		return
	}

	log.Println("=== v3 Local Mode ===")
	orch := orchestrator.NewMessageOrchestrator(
		sessionRepo,
		agents.Mio,
		agents.Shiro,
		llmRuntime.Coder1,
		llmRuntime.Coder2,
		llmRuntime.Coder3,
		llmRuntime.Coder4,
		workerExecutionService,
	)
	if coderCaps := buildCoderCapabilities(nodeCaps, cfg); coderCaps != nil {
		orch.SetCoderCapabilities(coderCaps)
		log.Printf("Dynamic coder selection enabled (%d coders)", len(coderCaps))
	}
	orch.SetEventListener(deps.eventRelay)
	if deps.reportStore != nil {
		orch.SetReportStore(deps.reportStore)
	}
	orch.SetMaxRepair(cfg.Worker.MaxRepair)
	orch.SetWildAgent(agents.Wild)
	orch.SetHeavyAgent(agents.Heavy)
	orch.SetTTSBridge(ttsBridge)
	orch.SetVTuberBridge(vtuberBridge)
	if deps.idleChatOrch != nil {
		orch.SetIdleNotifier(deps.idleChatOrch)
		log.Printf("IdleChat integrated with MessageOrchestrator")
	}
	buildChannelRuntimeHandlers(cfg, deps, orch)
	deps.viewerSend = bridges.ViewerSendFromOrch(orch)
	deps.entryHandler = bridges.EntryFromOrch(orch)
	deps.chromeBridge, deps.chromeBridgeStatus, deps.chromeBridgeEvents = bridges.ChromeBridgeFromOrch(orch)
}
