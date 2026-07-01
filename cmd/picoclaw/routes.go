package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	idlechatfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/idlechat"
	opsfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/ops"
	sttfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/stt"
	ttsfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/tts"
	viewerfeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/viewer"
	voicefeature "github.com/Nyukimin/picoclaw_multiLLM/internal/features/voice"
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

func registerOpsRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies) {
	if dependencies.backlogStore == nil {
		dependencies.backlogStore = viewer.NewBacklogStore(filepath.Join(cfg.WorkspaceDir, "logs", "backlog.jsonl"))
	}
	opsfeature.RegisterRoutes(mux, opsfeature.Dependencies{Routes: opsfeature.Routes{
		Status:                 dependencies.viewerStatus,
		Agents:                 dependencies.viewerAgents,
		AgentDetail:            dependencies.viewerAgentDetail,
		Jobs:                   dependencies.viewerJobs,
		ParallelJobs:           dependencies.parallelJobs,
		ParallelJobDetail:      dependencies.parallelJobDetail,
		JobNotifications:       dependencies.jobNotifications,
		Logs:                   dependencies.viewerLogs,
		AuditSummary:           dependencies.viewerAuditSummary,
		JobDetail:              dependencies.viewerJobDetail,
		RepairRun:              viewer.HandleRepairRunWithRunner(dependencies.eventRelay, dependencies.repairRunner),
		Backlog:                viewer.HandleBacklog(dependencies.backlogStore),
		Scheduler:              dependencies.schedulerStatus,
		Workstreams:            dependencies.workstreamStatus,
		WorkstreamGoals:        dependencies.workstreamGoal,
		WorkstreamArtifacts:    dependencies.workstreamArtifact,
		WorkstreamAnnotations:  dependencies.workstreamAnnotation,
		WorkstreamSteering:     dependencies.workstreamSteering,
		WorkstreamHeartbeats:   dependencies.workstreamHeartbeat,
		WorkstreamVaultUpdates: dependencies.workstreamVaultUpdate,
		WorkstreamVaultReview:  dependencies.workstreamVaultReview,
		WorkstreamVaultPreview: dependencies.workstreamVaultPreview,
		Revenue:                dependencies.revenueStatus,
		RevenueMarketResearch:  dependencies.revenueMarket,
		RevenueSNSPosts:        dependencies.revenueSNSPost,
		RevenueProducts:        dependencies.revenueProduct,
		RevenueCustomerVoices:  dependencies.revenueCustomerVoice,
		RevenueEvents:          dependencies.revenueEvent,
		RevenueDecisionGate:    dependencies.revenueHumanDecisionGate,
		RevenueDecisionReview:  dependencies.revenueHumanDecisionReview,
		RevenueDailyRoutine:    dependencies.revenueDailyRoutine,
		RevenueChannelDrafts:   dependencies.revenueChannelDraft,
		RevenueExternalSend:    dependencies.revenueExternalSendApply,
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

func registerSTTAndAudioRoutes(mux *http.ServeMux, cfg *config.Config, sttRuntime sttRuntime, voiceChatRuntime voiceChatRuntime, dependencies *Dependencies) {
	sttRoutes := sttRuntimeRoutes(sttRuntime)
	sttRoutes.ClientLog = viewer.HandleSTTClientLogSave(modulestt.DefaultViewerClientLogPath)
	sttRoutes.WAV = viewer.HandleSTTInputWAVSave(modulestt.DefaultViewerLatestWAVPath, modulestt.DefaultViewerArchiveDir)
	sttRoutes.RawWAV = viewer.HandleSTTInputRawWAVSave(modulestt.DefaultViewerLatestRawWAVPath, modulestt.DefaultViewerArchiveDir)
	sttRoutes.AutoTest = viewer.HandleSTTAutoTest(modulestt.DefaultViewerAutoTestScriptPath, modulestt.DefaultViewerLatestWAVPath, modulestt.DefaultViewerAutoTestOutputPath)
	sttRoutes.AdminRestart = viewer.HandleSTTRestart(viewer.STTAdminOptions{BaseURL: sttRuntime.DebugOptions.STTBaseURL})
	dependencies.moduleSTTViewerInput = newSTTViewerInputObserver(sttRuntime)
	voicefeature.RegisterRoutes(mux, voicefeature.Dependencies{
		Routes: voicefeature.Routes{
			VoiceChat:         voiceChatRuntime.WSHandler,
			AudioRouterEvents: viewer.HandleAudioRouterSSE(dependencies.eventHub),
			ActiveControl:     handleViewerActiveClaim(dependencies.eventHub.OnEvent),
		},
		STT: sttfeature.Dependencies{Routes: sttRoutes},
		TTS: ttsfeature.Dependencies{Routes: ttsfeature.Routes{
			Audio:       handleTTSAudio(cfg.TTS.OutputDir, cfg.TTS.HTTPBaseURL),
			PlaybackAck: handleTTSPlaybackAck(),
		}},
	})
	registerModuleRoutes(mux, dependencies, sttRuntime)
}

func registerViewerDynamicRoutes(mux *http.ServeMux, dependencies *Dependencies) {
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
	idlechatfeature.RegisterRoutes(mux, idlechatfeature.Dependencies{Routes: idlechatfeature.Routes{
		Start:       dependencies.handleIdleChatStart(),
		Stop:        dependencies.handleIdleChatStop(),
		Interrupt:   dependencies.handleIdleChatInterrupt(),
		Status:      dependencies.handleIdleChatStatus(),
		Logs:        dependencies.handleIdleChatLogs(),
		Forecast:    dependencies.handleIdleChatForecast(),
		Story:       dependencies.handleIdleChatStory(),
		StorySimple: dependencies.handleIdleChatStorySimple(),
	}})
}

func registerHealthRoutes(mux *http.ServeMux, dependencies *Dependencies, cfg *config.Config) {
	healthHandler := dependencies.buildHealthHandler(cfg)
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	mux.HandleFunc("/ready", healthHandler.HandleReady)
}
