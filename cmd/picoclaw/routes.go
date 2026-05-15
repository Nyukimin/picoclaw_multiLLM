package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
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
	mux.HandleFunc("/viewer", viewer.HandlePage)
	mux.HandleFunc("/viewer/assets/", viewer.HandleAsset)
	mux.HandleFunc("/viewer/runtime-config", viewer.HandleRuntimeConfig(debugSystemOpts))
	mux.HandleFunc("/viewer/logo.png", viewer.HandleLogo)
	mux.HandleFunc("/viewer/mio-lipsync-closed.svg", viewer.HandleMioLipSyncClosed)
	mux.HandleFunc("/viewer/mio-lipsync-open.svg", viewer.HandleMioLipSyncOpen)
	mux.HandleFunc("/viewer/shiro-lipsync-closed.svg", viewer.HandleShiroLipSyncClosed)
	mux.HandleFunc("/viewer/shiro-lipsync-open.svg", viewer.HandleShiroLipSyncOpen)
	mux.HandleFunc("/viewer/tts/audio", handleLocalTTSAudio(cfg.TTS.OutputDir))
	mux.HandleFunc("/viewer/events", dependencies.eventHub.HandleSSE)
	mux.HandleFunc("/viewer/debug/system", viewer.HandleDebugSystemSnapshot(debugSystemOpts))
	mux.HandleFunc("/viewer/assets-git/status", viewer.HandleAssetsGitStatus(defaultAssetsGitRepoPath()))
}

func registerLLMOpsRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies, debugSystemOpts *viewer.DebugSystemOptions) {
	if debugSystemOpts == nil || !debugSystemOpts.LLMOpsEnabled {
		return
	}
	llmOpsOpts := viewer.LLMOpsProxyOptions{
		BaseURL: cfg.LLMOps.BaseURL,
		Token:   strings.TrimSpace(os.Getenv("LLM_OPS_TOKEN")),
	}
	dependencies.idleChatStartGate = viewer.NewLLMOpsIdleChatGate(llmOpsOpts)
	mux.HandleFunc("/viewer/llm-ops/health", viewer.HandleLLMOpsHealth(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/status", viewer.HandleLLMOpsStatus(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/start", viewer.HandleLLMOpsStart(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/stop", viewer.HandleLLMOpsStop(llmOpsOpts))
	mux.HandleFunc("/viewer/llm-ops/restart", viewer.HandleLLMOpsRestart(llmOpsOpts))
	log.Printf("Viewer: MLX llm-ops proxy -> %s", strings.TrimRight(strings.TrimSpace(cfg.LLMOps.BaseURL), "/"))
}

func registerSTTAndAudioRoutes(mux *http.ServeMux, sttRuntime sttRuntime, dependencies *Dependencies) {
	mux.HandleFunc("/viewer/stt/log", viewer.HandleSTTClientLogSave("tmp/client_stt_log.txt"))
	mux.HandleFunc("/viewer/stt/wav", viewer.HandleSTTInputWAVSave("tmp/client_stt_input_latest.wav", "tmp/stt_inputs"))
	mux.HandleFunc("/viewer/stt/autotest", viewer.HandleSTTAutoTest("scripts/stt_e2e_probe.py", "tmp/client_stt_input_latest.wav", "tmp/stt_e2e_from_mic_latest.json"))
	registerSTTRuntimeRoutes(mux, sttRuntime)
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
	if dependencies.viewerRecallTraces != nil {
		mux.HandleFunc("/viewer/recall/traces", dependencies.viewerRecallTraces)
	}
	if dependencies.viewerSourceRegistry != nil {
		mux.HandleFunc("/viewer/source-registry", dependencies.viewerSourceRegistry)
	}
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
