package main

import (
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	domainai "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/aiworkflow"
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
	verificationRuntime verificationRuntime,
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
		startSuperAgentRunQueueScheduler(cfg, deps.superAgentStore, deps.distOrch)
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
		log.Printf("Coder capability metadata loaded (%d coders); CODE uses only local coder1 unless an explicit CODE route is requested", len(coderCaps))
	}
	orch.SetExternalCoderPolicy(map[string]bool{
		"coder1": coderProviderIsExternal(cfg.Coder1),
		"coder2": coderProviderIsExternal(cfg.Coder2),
		"coder3": coderProviderIsExternal(cfg.Coder3),
		"coder4": coderProviderIsExternal(cfg.Coder4),
	})
	orch.SetEventListener(deps.eventRelay)
	if deps.reportStore != nil {
		orch.SetReportStore(deps.reportStore)
	}
	orch.SetMaxRepair(cfg.Worker.MaxRepair)
	orch.SetWildAgent(agents.Wild)
	orch.SetHeavyAgent(agents.Heavy)
	orch.SetHeavyWorkerPolicy(domainai.HeavyWorkerPolicy{
		Enabled:                 cfg.AIWorkflow.HeavyWorkerEnabled,
		RequireReason:           cfg.AIWorkflow.HeavyWorkerRequireReason,
		FileCountThreshold:      cfg.AIWorkflow.HeavyWorkerFileThreshold,
		SpecCountThreshold:      cfg.AIWorkflow.HeavyWorkerSpecThreshold,
		FailedAttemptsThreshold: cfg.AIWorkflow.HeavyWorkerRetryThreshold,
	})
	if deps.dciSearcher != nil {
		orch.SetDCISearcher(deps.dciSearcher)
		log.Println("DCI explicit trigger integrated with MessageOrchestrator")
	}
	if deps.recallTraceStore != nil {
		orch.SetRecallTraceStore(deps.recallTraceStore)
		log.Println("Recall trace store integrated with MessageOrchestrator")
	}
	if deps.skillBootstrap != nil {
		orch.SetSkillBootstrapRecorder(deps.skillBootstrap)
		log.Println("Skill bootstrap integrated with MessageOrchestrator routes")
	}
	if deps.coderProposalEvidence != nil {
		orch.SetCoderProposalEvidenceRecorder(deps.coderProposalEvidence)
		log.Println("Coder proposal evidence recorder integrated with MessageOrchestrator")
	}
	if deps.aiWorkflowStore != nil {
		orch.SetWorkflowEventRecorder(deps.aiWorkflowStore)
		orch.SetCommandRegistry(deps.aiWorkflowStore)
		log.Println("AI Workflow event recorder integrated with MessageOrchestrator")
	}
	if deps.superAgentStore != nil {
		orch.SetSuperAgentRuntimeRecorder(deps.superAgentStore)
		orch.SetSuperAgentRunController(deps.superAgentRunController)
		log.Println("SuperAgent runtime recorder integrated with MessageOrchestrator")
	}
	if deps.personaRuntimeStore != nil {
		orch.SetPersonaRuntimeRecorder(deps.personaRuntimeStore, deps.personaTriggerDefinitions)
		orch.SetPersonaCanonicalResponses(deps.personaCanonicalResponses)
		log.Printf("Persona runtime recorder integrated with MessageOrchestrator (%d trigger definitions, %d canonical responses)", len(deps.personaTriggerDefinitions), len(deps.personaCanonicalResponses))
	}
	orch.SetTTSBridge(ttsBridge)
	orch.SetVTuberBridge(vtuberBridge)
	if verificationRuntime.Pipeline != nil {
		orch.SetVerificationPipeline(verificationRuntime.Pipeline)
		log.Println("Verification pipeline integrated with MessageOrchestrator")
	}
	if deps.idleChatOrch != nil {
		orch.SetIdleNotifier(deps.idleChatOrch)
		log.Printf("IdleChat integrated with MessageOrchestrator")
	}
	buildChannelRuntimeHandlers(cfg, deps, orch)
	deps.viewerSend = bridges.ViewerSendFromOrch(orch)
	deps.entryHandler = bridges.EntryFromOrch(orch)
	deps.chromeBridge, deps.chromeBridgeStatus, deps.chromeBridgeEvents = bridges.ChromeBridgeFromOrch(orch)
	startSuperAgentRunQueueScheduler(cfg, deps.superAgentStore, orch)
}
