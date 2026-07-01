package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	viewerfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/viewer"
	modulestt "github.com/Nyukimin/picoclaw_multiLLM/modules/stt"
)

func registerChannelRoutes(mux *http.ServeMux, dependencies *Dependencies) {
	mux.Handle("/webhook", dependencies.lineHandler)
	if dependencies.telegramHandler != nil {
		mux.Handle("/webhook/telegram", dependencies.telegramHandler)
	}
	if dependencies.discordHandler != nil {
		mux.Handle("/webhook/discord", dependencies.discordHandler)
	}
	if dependencies.slackHandler != nil {
		mux.Handle("/webhook/slack", dependencies.slackHandler)
	}
}

func registerViewerBaseRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies, debugSystemOpts viewer.DebugSystemOptions) {
	if dependencies.backlogStore == nil {
		dependencies.backlogStore = viewer.NewBacklogStore(filepath.Join(cfg.WorkspaceDir, "logs", "backlog.jsonl"))
	}
	viewerfeature.RegisterBaseRoutes(mux, viewerfeature.Dependencies{Base: viewerfeature.BaseRoutes{
		Page:                         viewer.HandlePage,
		Asset:                        viewer.HandleAsset,
		RuntimeConfig:                viewer.HandleRuntimeConfig(debugSystemOpts),
		Logo:                         viewer.HandleLogo,
		MioLipSyncClosed:             viewer.HandleMioLipSyncClosed,
		MioLipSyncOpen:               viewer.HandleMioLipSyncOpen,
		MioPortrait:                  viewer.HandleMioPortrait,
		ShiroPortrait:                viewer.HandleShiroPortrait,
		ShiroLipSyncClosed:           viewer.HandleShiroLipSyncClosed,
		ShiroLipSyncOpen:             viewer.HandleShiroLipSyncOpen,
		CharacterState:               viewer.HandleCharacterState,
		CharacterManifest:            viewer.HandleCharacterManifest,
		LayeredCharacterState:        viewer.HandleLayeredCharacterState,
		LayeredCharacterMouth:        viewer.HandleLayeredCharacterMouth,
		LayeredCharacterManifest:     viewer.HandleLayeredCharacterManifest,
		Live2DCharacter:              viewer.HandleLive2DCharacter,
		Live2DCharacterEmbed:         viewer.HandleLive2DCharacterEmbed,
		Live2DAsset:                  viewer.HandleLive2DAsset,
		Live2DChat:                   viewer.HandleLive2DChat,
		Live2DEmotionControl:         viewer.HandleLive2DEmotionControl,
		Live2DChatAPI:                viewer.HandleLive2DChatAPIWithResponder(newLive2DOrchestratorResponder(dependencies)),
		TTSAudio:                     handleTTSAudio(cfg.TTS.OutputDir, cfg.TTS.HTTPBaseURL),
		TTSPlaybackAck:               handleTTSPlaybackAck(),
		ActiveControl:                handleViewerActiveClaim(dependencies.eventHub.OnEvent),
		Events:                       dependencies.eventHub.HandleSSE,
		DebugSystem:                  viewer.HandleDebugSystemSnapshot(debugSystemOpts),
		DocsSearch:                   viewer.HandleDocsSearch(),
		DocsDetail:                   viewer.HandleDocsDetail(),
		HistoryRepairJSONL:           dependencies.historyRepairJSONL,
		PackageValidation:            dependencies.packageValidation,
		CharacterRuntime:             dependencies.characterRuntime,
		ExtensionHealth:              dependencies.extensionHealth,
		OTELExport:                   dependencies.otelExport,
		ArtifactCleanup:              dependencies.artifactCleanup,
		RepairRun:                    viewer.HandleRepairRunWithRunner(dependencies.eventRelay, dependencies.repairRunner),
		Backlog:                      viewer.HandleBacklog(dependencies.backlogStore),
		Scheduler:                    dependencies.schedulerStatus,
		AssetsGitStatus:              viewer.HandleAssetsGitStatus(defaultAssetsGitRepoPath()),
		MovieCatalog:                 viewer.HandleMovieCatalog(viewer.MovieCatalogOptions{}),
		MovieCatalogFetch:            viewer.HandleMovieCatalogFetch(viewer.MovieCatalogOptions{}),
		MovieCatalogPreference:       viewer.HandleMovieCatalogPreference(viewer.MovieCatalogOptions{}),
		MovieTopicCandidatesGenerate: viewer.HandleMovieTopicCandidatesGenerate(viewer.MovieCatalogOptions{}),
		HobbyGraph:                   viewer.HandleHobbyGraph(viewer.HobbyGraphOptions{}),
		HobbyGraphBootstrap:          viewer.HandleHobbyGraphBootstrap(viewer.HobbyGraphOptions{}),
		HobbyGraphInteraction:        viewer.HandleHobbyGraphInteraction(viewer.HobbyGraphOptions{}),
		HobbyGraphRelation:           viewer.HandleHobbyGraphRelation(viewer.HobbyGraphOptions{}),
		HobbyTopicCandidatesGenerate: viewer.HandleHobbyTopicCandidatesGenerate(viewer.HobbyGraphOptions{}),
		InvestmentStatus:             viewer.HandleInvestmentStatus(defaultInvestmentDBPath()),
		InvestmentNotify:             viewer.HandleInvestmentNotify(dependencies.eventHub),
	}})
}

func registerLLMOpsRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies, debugSystemOpts *viewer.DebugSystemOptions) {
	llmOpsOpts := viewer.LLMOpsProxyOptions{
		BaseURL: cfg.LLMOps.BaseURL,
		Token:   strings.TrimSpace(os.Getenv("LLM_OPS_TOKEN")),
	}
	if debugSystemOpts != nil {
		dependencies.aiWorkflowHeavyRuntime = viewer.HandleAIWorkflowHeavyWorkerRuntimeDiagnostics(viewer.HeavyWorkerRuntimeDiagnosticsOptions{
			LocalLLMEnabled:  debugSystemOpts.LocalLLM.Enabled,
			Provider:         debugSystemOpts.LocalLLM.Provider,
			EffectiveBaseURL: debugSystemOpts.LocalLLM.HeavyBaseURL,
			EffectiveModel:   debugSystemOpts.LocalLLM.HeavyModel,
			TimeoutSec:       debugSystemOpts.LocalLLM.TimeoutSec,
			LLMOpsConfigured: debugSystemOpts.LLMOpsConfigured,
			LLMOpsEnabled:    debugSystemOpts.LLMOpsEnabled,
			LLMOpsBaseURL:    debugSystemOpts.LLMOpsBaseURL,
			LLMOps:           llmOpsOpts,
		})
	}
	if debugSystemOpts == nil || !debugSystemOpts.LLMOpsEnabled {
		return
	}
	dependencies.idleChatStartGate = viewer.NewLLMOpsIdleChatGate(llmOpsOpts)
	mux.HandleFunc("/viewer/llm-ops/health", viewer.HandleLLMOpsHealth(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/status", viewer.HandleLLMOpsStatus(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/start", viewer.HandleLLMOpsStart(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/stop", viewer.HandleLLMOpsStop(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/restart", viewer.HandleLLMOpsRestart(llmOpsOpts))
	log.Printf("Viewer: MLX llm-ops proxy -> %s", strings.TrimRight(strings.TrimSpace(cfg.LLMOps.BaseURL), "/"))
}

func registerSTTAndAudioRoutes(mux *http.ServeMux, sttRuntime sttRuntime, voiceChatRuntime voiceChatRuntime, dependencies *Dependencies) {
	mux.HandleFunc("/viewer/stt/log", viewer.HandleSTTClientLogSave(modulestt.DefaultViewerClientLogPath))
	mux.HandleFunc("/viewer/stt/wav", viewer.HandleSTTInputWAVSave(modulestt.DefaultViewerLatestWAVPath, modulestt.DefaultViewerArchiveDir))
	mux.HandleFunc("/viewer/stt/wav/raw", viewer.HandleSTTInputRawWAVSave(modulestt.DefaultViewerLatestRawWAVPath, modulestt.DefaultViewerArchiveDir))
	mux.HandleFunc("/viewer/stt/autotest", viewer.HandleSTTAutoTest(modulestt.DefaultViewerAutoTestScriptPath, modulestt.DefaultViewerLatestWAVPath, modulestt.DefaultViewerAutoTestOutputPath))
	mux.HandleFunc("/viewer/stt/admin/restart", viewer.HandleSTTRestart(viewer.STTAdminOptions{BaseURL: sttRuntime.DebugOptions.STTBaseURL}))
	dependencies.moduleSTTViewerInput = newSTTViewerInputObserver(sttRuntime)
	registerSTTRuntimeRoutes(mux, sttRuntime)
	registerVoiceChatRuntimeRoutes(mux, voiceChatRuntime)
	registerModuleRoutes(mux, dependencies, sttRuntime)
	mux.HandleFunc("/audio-router/events", viewer.HandleAudioRouterSSE(dependencies.eventHub))
}

func registerViewerDynamicRoutes(mux *http.ServeMux, dependencies *Dependencies) {
	if dependencies.viewerStatus != nil {
		mux.HandleFunc("/viewer/status", dependencies.viewerStatus)
	}
	if dependencies.viewerAgents != nil {
		mux.HandleFunc("/viewer/agents", dependencies.viewerAgents)
	}
	if dependencies.viewerAgentDetail != nil {
		mux.HandleFunc("/viewer/agent/detail", dependencies.viewerAgentDetail)
	}
	if dependencies.viewerJobs != nil {
		mux.HandleFunc("/viewer/jobs", dependencies.viewerJobs)
	}
	if dependencies.parallelJobs != nil {
		mux.HandleFunc("/viewer/parallel-jobs", dependencies.parallelJobs)
	}
	if dependencies.parallelJobDetail != nil {
		mux.HandleFunc("/viewer/parallel-job/detail", dependencies.parallelJobDetail)
	}
	if dependencies.jobNotifications != nil {
		mux.HandleFunc("/viewer/job-notifications", dependencies.jobNotifications)
	}
	if dependencies.viewerLogs != nil {
		mux.HandleFunc("/viewer/logs", dependencies.viewerLogs)
	}
	if dependencies.viewerAuditSummary != nil {
		mux.HandleFunc("/viewer/audit/summary", dependencies.viewerAuditSummary)
	}
	if dependencies.viewerJobDetail != nil {
		mux.HandleFunc("/viewer/job/detail", dependencies.viewerJobDetail)
	}
	if dependencies.viewerSend != nil {
		mux.HandleFunc("/viewer/send", dependencies.viewerSend)
	}
	if dependencies.evidenceHandler != nil {
		mux.HandleFunc("/viewer/evidence/recent", dependencies.evidenceHandler)
	}
	if dependencies.evidenceDetail != nil {
		mux.HandleFunc("/viewer/evidence/detail", dependencies.evidenceDetail)
	}
	if dependencies.evidenceSummary != nil {
		mux.HandleFunc("/viewer/evidence/summary", dependencies.evidenceSummary)
	}
	if dependencies.glossaryRecent != nil {
		mux.HandleFunc("/viewer/glossary/recent", dependencies.glossaryRecent)
	}
	if dependencies.viewerMemorySnapshot != nil {
		mux.HandleFunc("/viewer/memory/snapshot", dependencies.viewerMemorySnapshot)
	}
	if dependencies.viewerMemoryLayers != nil {
		mux.HandleFunc("/viewer/memory/layers", dependencies.viewerMemoryLayers)
	}
	if dependencies.viewerMemoryEvents != nil {
		mux.HandleFunc("/viewer/memory/events", dependencies.viewerMemoryEvents)
	}
	if dependencies.viewerMemoryState != nil {
		mux.HandleFunc("/viewer/memory/state", dependencies.viewerMemoryState)
	}
	if dependencies.viewerMemoryPromote != nil {
		mux.HandleFunc("/viewer/memory/promote", dependencies.viewerMemoryPromote)
	}
	if dependencies.viewerMemoryUser != nil {
		mux.HandleFunc("/viewer/memory/user", dependencies.viewerMemoryUser)
	}
	if dependencies.viewerMemoryUserState != nil {
		mux.HandleFunc("/viewer/memory/user/state", dependencies.viewerMemoryUserState)
	}
	if dependencies.viewerMemoryUserForget != nil {
		mux.HandleFunc("/viewer/memory/user/forget", dependencies.viewerMemoryUserForget)
	}
	if dependencies.viewerMemoryUserSupersede != nil {
		mux.HandleFunc("/viewer/memory/user/supersede", dependencies.viewerMemoryUserSupersede)
	}
	if dependencies.viewerMemoryRecallPack != nil {
		mux.HandleFunc("/viewer/memory/recall-pack", dependencies.viewerMemoryRecallPack)
	}
	if dependencies.viewerRecallTraces != nil {
		mux.HandleFunc("/viewer/recall/traces", dependencies.viewerRecallTraces)
	}
	if dependencies.viewerSourceRegistry != nil {
		mux.HandleFunc("/viewer/source-registry", dependencies.viewerSourceRegistry)
	}
	if dependencies.viewerDomainGraphAssertions != nil {
		mux.HandleFunc("/viewer/domain-graph/assertions", dependencies.viewerDomainGraphAssertions)
	}
	if dependencies.viewerMovieDomainGraphSync != nil {
		mux.HandleFunc("/viewer/movie-catalog/domain-graph-sync", dependencies.viewerMovieDomainGraphSync)
	}
	if dependencies.viewerHobbyDomainGraphSync != nil {
		mux.HandleFunc("/viewer/hobby-graph/domain-graph-sync", dependencies.viewerHobbyDomainGraphSync)
	}
	if dependencies.verificationRecent != nil {
		mux.HandleFunc("/viewer/verification/recent", dependencies.verificationRecent)
	}
	if dependencies.verificationDetail != nil {
		mux.HandleFunc("/viewer/verification/detail", dependencies.verificationDetail)
	}
	if dependencies.verificationSummary != nil {
		mux.HandleFunc("/viewer/verification/summary", dependencies.verificationSummary)
	}
	if dependencies.toolHarnessRecent != nil {
		mux.HandleFunc("/viewer/tool-harness/recent", dependencies.toolHarnessRecent)
	}
	if dependencies.dciRecent != nil {
		mux.HandleFunc("/viewer/dci/recent", dependencies.dciRecent)
	}
	if dependencies.dciSearch != nil {
		mux.HandleFunc("/viewer/dci/search", dependencies.dciSearch)
	}
	if dependencies.sandboxStatus != nil {
		mux.HandleFunc("/viewer/sandbox", dependencies.sandboxStatus)
	}
	if dependencies.sandboxPromotion != nil {
		mux.HandleFunc("/viewer/sandbox/promotions", dependencies.sandboxPromotion)
	}
	if dependencies.sandboxPromotionApply != nil {
		mux.HandleFunc("/viewer/sandbox/promotions/apply", dependencies.sandboxPromotionApply)
	}
	if dependencies.sandboxPromotionRollback != nil {
		mux.HandleFunc("/viewer/sandbox/promotions/rollback", dependencies.sandboxPromotionRollback)
	}
	if dependencies.sandboxPromotionPreview != nil {
		mux.HandleFunc("/viewer/sandbox/promotions/preview", dependencies.sandboxPromotionPreview)
	}
	if dependencies.sandboxPromotionManualReview != nil {
		mux.HandleFunc("/viewer/sandbox/promotions/manual-review", dependencies.sandboxPromotionManualReview)
	}
	if dependencies.sandboxWorktreeCreate != nil {
		mux.HandleFunc("/viewer/sandbox/worktrees/create", dependencies.sandboxWorktreeCreate)
	}
	if dependencies.sandboxWorktreeClose != nil {
		mux.HandleFunc("/viewer/sandbox/worktrees/close", dependencies.sandboxWorktreeClose)
	}
	if dependencies.skillGovernanceRecent != nil {
		mux.HandleFunc("/viewer/skill-governance/recent", dependencies.skillGovernanceRecent)
	}
	if dependencies.skillGovernanceBoot != nil {
		mux.HandleFunc("/viewer/skill-governance/bootstrap", dependencies.skillGovernanceBoot)
	}
	if dependencies.skillContributionGate != nil {
		mux.HandleFunc("/viewer/skill-governance/contribution-gate", dependencies.skillContributionGate)
	}
	if dependencies.skillChangeGate != nil {
		mux.HandleFunc("/viewer/skill-governance/skill-changes", dependencies.skillChangeGate)
	}
	if dependencies.skillChangeEval != nil {
		mux.HandleFunc("/viewer/skill-governance/skill-change-evals", dependencies.skillChangeEval)
	}
	if dependencies.skillExternalPRSubmit != nil {
		mux.HandleFunc("/viewer/skill-governance/external-pr-submit", dependencies.skillExternalPRSubmit)
	}
	if dependencies.workstreamStatus != nil {
		mux.HandleFunc("/viewer/workstreams", dependencies.workstreamStatus)
	}
	if dependencies.workstreamGoal != nil {
		mux.HandleFunc("/viewer/workstreams/goals", dependencies.workstreamGoal)
	}
	if dependencies.workstreamArtifact != nil {
		mux.HandleFunc("/viewer/workstreams/artifacts", dependencies.workstreamArtifact)
	}
	if dependencies.workstreamAnnotation != nil {
		mux.HandleFunc("/viewer/workstreams/annotations", dependencies.workstreamAnnotation)
	}
	if dependencies.workstreamSteering != nil {
		mux.HandleFunc("/viewer/workstreams/steering", dependencies.workstreamSteering)
	}
	if dependencies.workstreamHeartbeat != nil {
		mux.HandleFunc("/viewer/workstreams/heartbeats", dependencies.workstreamHeartbeat)
	}
	if dependencies.workstreamVaultUpdate != nil {
		mux.HandleFunc("/viewer/workstreams/vault-updates", dependencies.workstreamVaultUpdate)
	}
	if dependencies.workstreamVaultReview != nil {
		mux.HandleFunc("/viewer/workstreams/vault-updates/review", dependencies.workstreamVaultReview)
	}
	if dependencies.workstreamVaultPreview != nil {
		mux.HandleFunc("/viewer/workstreams/vault-updates/preview", dependencies.workstreamVaultPreview)
	}
	if dependencies.revenueStatus != nil {
		mux.HandleFunc("/viewer/revenue", dependencies.revenueStatus)
	}
	if dependencies.revenueMarket != nil {
		mux.HandleFunc("/viewer/revenue/market-research", dependencies.revenueMarket)
	}
	if dependencies.revenueSNSPost != nil {
		mux.HandleFunc("/viewer/revenue/sns-posts", dependencies.revenueSNSPost)
	}
	if dependencies.revenueProduct != nil {
		mux.HandleFunc("/viewer/revenue/products", dependencies.revenueProduct)
	}
	if dependencies.revenueCustomerVoice != nil {
		mux.HandleFunc("/viewer/revenue/customer-voices", dependencies.revenueCustomerVoice)
	}
	if dependencies.revenueEvent != nil {
		mux.HandleFunc("/viewer/revenue/events", dependencies.revenueEvent)
	}
	if dependencies.revenueHumanDecisionGate != nil {
		mux.HandleFunc("/viewer/revenue/human-decision-gate", dependencies.revenueHumanDecisionGate)
	}
	if dependencies.revenueHumanDecisionReview != nil {
		mux.HandleFunc("/viewer/revenue/human-decision-gate/review", dependencies.revenueHumanDecisionReview)
	}
	if dependencies.revenueDailyRoutine != nil {
		mux.HandleFunc("/viewer/revenue/daily-routine", dependencies.revenueDailyRoutine)
	}
	if dependencies.revenueChannelDraft != nil {
		mux.HandleFunc("/viewer/revenue/channel-drafts", dependencies.revenueChannelDraft)
	}
	if dependencies.revenueExternalSendApply != nil {
		mux.HandleFunc("/viewer/revenue/channel-drafts/external-send-apply", dependencies.revenueExternalSendApply)
	}
	if dependencies.personaObservation != nil {
		mux.HandleFunc("/viewer/persona-observation", dependencies.personaObservation)
	}
	if dependencies.personaDiscomfort != nil {
		mux.HandleFunc("/viewer/persona-observation/discomforts", dependencies.personaDiscomfort)
	}
	if dependencies.personaTrigger != nil {
		mux.HandleFunc("/viewer/persona-observation/triggers", dependencies.personaTrigger)
	}
	if dependencies.personaCanonical != nil {
		mux.HandleFunc("/viewer/persona-observation/canonical-responses", dependencies.personaCanonical)
	}
	if dependencies.personaObservationLog != nil {
		mux.HandleFunc("/viewer/persona-observation/observations", dependencies.personaObservationLog)
	}
	if dependencies.personaObservationAggregate != nil {
		mux.HandleFunc("/viewer/persona-observation/aggregate", dependencies.personaObservationAggregate)
	}
	if dependencies.personaMetaUpdate != nil {
		mux.HandleFunc("/viewer/persona-observation/meta-updates", dependencies.personaMetaUpdate)
	}
	if dependencies.personaMetaUpdateReview != nil {
		mux.HandleFunc("/viewer/persona-observation/meta-updates/review", dependencies.personaMetaUpdateReview)
	}
	if dependencies.personaSession != nil {
		mux.HandleFunc("/viewer/persona-observation/sessions", dependencies.personaSession)
	}
	if dependencies.browserTraceAPIStatus != nil {
		mux.HandleFunc("/viewer/browser-trace-api", dependencies.browserTraceAPIStatus)
	}
	if dependencies.browserTraceAPIDiscover != nil {
		mux.HandleFunc("/viewer/browser-trace-api/discover", dependencies.browserTraceAPIDiscover)
	}
	if dependencies.browserTraceAPIValidation != nil {
		mux.HandleFunc("/viewer/browser-trace-api/validations", dependencies.browserTraceAPIValidation)
	}
	if dependencies.browserTraceAPIFetcherProposal != nil {
		mux.HandleFunc("/viewer/browser-trace-api/fetcher-proposals", dependencies.browserTraceAPIFetcherProposal)
	}
	if dependencies.complexityHotspotStatus != nil {
		mux.HandleFunc("/viewer/complexity-hotspots", dependencies.complexityHotspotStatus)
	}
	if dependencies.complexityHotspotScan != nil {
		mux.HandleFunc("/viewer/complexity-hotspots/scan", dependencies.complexityHotspotScan)
	}
	if dependencies.complexityHotspotProposal != nil {
		mux.HandleFunc("/viewer/complexity-hotspots/proposals", dependencies.complexityHotspotProposal)
	}
	if dependencies.complexityHotspotConcreteDiff != nil {
		mux.HandleFunc("/viewer/complexity-hotspots/concrete-diffs", dependencies.complexityHotspotConcreteDiff)
	}
	if dependencies.complexityHotspotCoderDiff != nil {
		mux.HandleFunc("/viewer/complexity-hotspots/coder-diffs", dependencies.complexityHotspotCoderDiff)
	}
	if dependencies.superAgentStatus != nil {
		mux.HandleFunc("/viewer/superagent", dependencies.superAgentStatus)
	}
	if dependencies.superAgentRun != nil {
		mux.HandleFunc("/viewer/superagent/runs", dependencies.superAgentRun)
	}
	if dependencies.superAgentRunPause != nil {
		mux.HandleFunc("/viewer/superagent/runs/pause", dependencies.superAgentRunPause)
	}
	if dependencies.superAgentRunResume != nil {
		mux.HandleFunc("/viewer/superagent/runs/resume", dependencies.superAgentRunResume)
	}
	if dependencies.superAgentRunQueue != nil {
		mux.HandleFunc("/viewer/superagent/run-queue", dependencies.superAgentRunQueue)
	}
	if dependencies.superAgentRunQueueClaim != nil {
		mux.HandleFunc("/viewer/superagent/run-queue/claim", dependencies.superAgentRunQueueClaim)
	}
	if dependencies.superAgentRunQueueComplete != nil {
		mux.HandleFunc("/viewer/superagent/run-queue/complete", dependencies.superAgentRunQueueComplete)
	}
	if dependencies.superAgentSubagentTask != nil {
		mux.HandleFunc("/viewer/superagent/subagent-tasks", dependencies.superAgentSubagentTask)
	}
	if dependencies.superAgentContextPack != nil {
		mux.HandleFunc("/viewer/superagent/context-packs", dependencies.superAgentContextPack)
	}
	if dependencies.superAgentMessageChannel != nil {
		mux.HandleFunc("/viewer/superagent/message-channels", dependencies.superAgentMessageChannel)
	}
	if dependencies.superAgentTraceEvent != nil {
		mux.HandleFunc("/viewer/superagent/trace-events", dependencies.superAgentTraceEvent)
	}
	if dependencies.aiWorkflowStatus != nil {
		mux.HandleFunc("/viewer/ai-workflow", dependencies.aiWorkflowStatus)
	}
	if dependencies.aiWorkflowEvent != nil {
		mux.HandleFunc("/viewer/ai-workflow/events", dependencies.aiWorkflowEvent)
	}
	if dependencies.aiWorkflowProjectMemory != nil {
		mux.HandleFunc("/viewer/ai-workflow/project-memory", dependencies.aiWorkflowProjectMemory)
	}
	if dependencies.aiWorkflowWorktree != nil {
		mux.HandleFunc("/viewer/ai-workflow/worktrees", dependencies.aiWorkflowWorktree)
	}
	if dependencies.aiWorkflowCommand != nil {
		mux.HandleFunc("/viewer/ai-workflow/commands", dependencies.aiWorkflowCommand)
	}
	if dependencies.aiWorkflowCommandRun != nil {
		mux.HandleFunc("/viewer/ai-workflow/commands/run", dependencies.aiWorkflowCommandRun)
	}
	if dependencies.aiWorkflowContextUsage != nil {
		mux.HandleFunc("/viewer/ai-workflow/context-usages", dependencies.aiWorkflowContextUsage)
	}
	if dependencies.aiWorkflowContextBudget != nil {
		mux.HandleFunc("/viewer/ai-workflow/context-budget/check", dependencies.aiWorkflowContextBudget)
	}
	if dependencies.aiWorkflowExternalControl != nil {
		mux.HandleFunc("/viewer/ai-workflow/external-control/check", dependencies.aiWorkflowExternalControl)
	}
	if dependencies.aiWorkflowHeavyWorker != nil {
		mux.HandleFunc("/viewer/ai-workflow/heavy-worker/evaluate", dependencies.aiWorkflowHeavyWorker)
	}
	if dependencies.aiWorkflowHeavyRuntime != nil {
		mux.HandleFunc("/viewer/ai-workflow/heavy-worker/runtime-diagnostics", dependencies.aiWorkflowHeavyRuntime)
	}
	if dependencies.aiWorkflowProjectInit != nil {
		mux.HandleFunc("/viewer/ai-workflow/project-init", dependencies.aiWorkflowProjectInit)
	}
	if dependencies.aiWorkflowWorktreeCreate != nil {
		mux.HandleFunc("/viewer/ai-workflow/worktrees/create", dependencies.aiWorkflowWorktreeCreate)
	}
	if dependencies.aiWorkflowWorktreeClose != nil {
		mux.HandleFunc("/viewer/ai-workflow/worktrees/close", dependencies.aiWorkflowWorktreeClose)
	}
	if dependencies.knowledgeMemoryStatus != nil {
		mux.HandleFunc("/viewer/knowledge-memory", dependencies.knowledgeMemoryStatus)
	}
	if dependencies.personalArchiveCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/personal-archive", dependencies.personalArchiveCreate)
	}
	if dependencies.creativeKnowledgeCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/creative-knowledge", dependencies.creativeKnowledgeCreate)
	}
	if dependencies.newsKnowledgeCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/news-knowledge", dependencies.newsKnowledgeCreate)
	}
	if dependencies.dailyIntakeRuleCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/daily-intake-rules", dependencies.dailyIntakeRuleCreate)
	}
	if dependencies.temporalMemoryCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/temporal-markers", dependencies.temporalMemoryCreate)
	}
	if dependencies.knowledgeMemoryReview != nil {
		mux.HandleFunc("/viewer/knowledge-memory/review", dependencies.knowledgeMemoryReview)
	}
	if dependencies.dreamConsolidationCreate != nil {
		mux.HandleFunc("/viewer/knowledge-memory/dream-runs", dependencies.dreamConsolidationCreate)
	}
	if dependencies.dreamConsolidationProposal != nil {
		mux.HandleFunc("/viewer/knowledge-memory/dream-runs/propose", dependencies.dreamConsolidationProposal)
	}
	if dependencies.dreamConsolidationReview != nil {
		mux.HandleFunc("/viewer/knowledge-memory/dream-runs/review", dependencies.dreamConsolidationReview)
	}
}

func defaultInvestmentDBPath() string {
	if env := strings.TrimSpace(os.Getenv("RENCROW_DATA_DB")); env != "" {
		return env
	}
	return filepath.Join("rencrow-data", "data", "rencrow.db")
}

func registerEntryAndChromeRoutes(mux *http.ServeMux, dependencies *Dependencies) {
	if dependencies.entryHandler != nil {
		mux.HandleFunc("/entry", dependencies.entryHandler)
	}
	if dependencies.chromeBridge != nil {
		mux.HandleFunc("/chrome/bridge", dependencies.chromeBridge)
	}
	if dependencies.chromeBridgeStatus != nil {
		mux.HandleFunc("/chrome/bridge/status", dependencies.chromeBridgeStatus)
	}
	if dependencies.chromeBridgeEvents != nil {
		mux.HandleFunc("/chrome/bridge/events", dependencies.chromeBridgeEvents)
	}
}

func registerIdleChatRoutes(mux *http.ServeMux, dependencies *Dependencies) {
	if dependencies.idleChatOrch == nil {
		return
	}
	mux.HandleFunc("/viewer/idlechat/start", dependencies.handleIdleChatStart())
	mux.HandleFunc("/viewer/idlechat/stop", dependencies.handleIdleChatStop())
	mux.HandleFunc("/viewer/idlechat/interrupt", dependencies.handleIdleChatInterrupt())
	mux.HandleFunc("/viewer/idlechat/status", dependencies.handleIdleChatStatus())
	mux.HandleFunc("/viewer/idlechat/logs", dependencies.handleIdleChatLogs())
	mux.HandleFunc("/viewer/idlechat/forecast", dependencies.handleIdleChatForecast())
	mux.HandleFunc("/viewer/idlechat/story", dependencies.handleIdleChatStory())
	mux.HandleFunc("/viewer/idlechat/story-simple", dependencies.handleIdleChatStorySimple())
}

func registerHealthRoutes(mux *http.ServeMux, dependencies *Dependencies, cfg *config.Config) {
	healthHandler := dependencies.buildHealthHandler(cfg)
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	mux.HandleFunc("/ready", healthHandler.HandleReady)
}
