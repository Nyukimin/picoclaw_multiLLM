package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	adapterchannels "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels"
	discordadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/discord"
	slackadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/slack"
	telegramadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/telegram"
	chromeadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/chrome"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	entryadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/entry"
	healthadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	healthapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/heartbeat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/sourcefetcher"
	subagentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/subagent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	capdomain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintool "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	glossary "github.com/Nyukimin/picoclaw_multiLLM/internal/glossary"
	capinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/capability"
	infrahealth "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/health"
	infrallm "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/claude"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/openai"
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
	"golang.org/x/net/websocket"
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

type primaryLLMProviders struct {
	Chat   llm.LLMProvider
	Worker llm.LLMProvider
	Wild   llm.LLMProvider
}

func buildPrimaryLLMProviders(cfg *config.Config) primaryLLMProviders {
	if cfg.LocalLLM.Enabled {
		timeout := time.Duration(cfg.LocalLLM.TimeoutSec) * time.Second
		global := make(chan struct{}, cfg.LocalLLM.GlobalConcurrency)
		chat := buildLocalAliasProvider(cfg, "Chat", cfg.LocalLLM.ChatModel, timeout, global)
		worker := buildLocalAliasProvider(cfg, "Worker", cfg.LocalLLM.WorkerModel, timeout, global)
		wild := buildLocalAliasProvider(cfg, "Wild", cfg.LocalLLM.WildModel, timeout, global)
		if cfg.LocalLLMWarmupEnabled() {
			go warmPrimaryLLMProviders(context.Background(), map[string]llm.LLMProvider{
				"Chat":   chat,
				"Worker": worker,
				"Wild":   wild,
			}, timeout)
		}
		return primaryLLMProviders{
			Chat:   infrallm.NewDateTimeProvider(chat),
			Worker: infrallm.NewDateTimeProvider(worker),
			Wild:   infrallm.NewDateTimeProvider(wild),
		}
	}

	chatRawProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, cfg.Ollama.Model, 32768)
	workerModel := strings.TrimSpace(cfg.Ollama.WorkerModel)
	if workerModel == "" {
		workerModel = cfg.Ollama.Model
	}
	workerRawProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, workerModel, 16384)
	return primaryLLMProviders{
		Chat:   infrallm.NewDateTimeProvider(chatRawProvider),
		Worker: infrallm.NewDateTimeProvider(workerRawProvider),
		Wild:   infrallm.NewDateTimeProvider(workerRawProvider),
	}
}

func buildConversationTextProvider(cfg *config.Config, providers primaryLLMProviders) (llm.LLMProvider, string) {
	if cfg.LocalLLM.Enabled && providers.Worker != nil {
		return providers.Worker, "local_llm Worker"
	}
	summaryModel := strings.TrimSpace(cfg.Conversation.SummaryModel)
	if summaryModel == "" {
		summaryModel = cfg.Ollama.Model
	}
	if summaryModel == "" {
		return nil, ""
	}
	return ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, summaryModel, 32768), fmt.Sprintf("%s (model: %s)", cfg.Ollama.BaseURL, summaryModel)
}

func buildConversationEmbedder(cfg *config.Config) (conversation.EmbeddingProvider, string) {
	model := strings.TrimSpace(cfg.Conversation.EmbedModel)
	if model == "" {
		return nil, ""
	}
	timeout := time.Duration(cfg.LocalLLM.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if cfg.LocalLLM.Enabled && cfg.LocalLLM.Provider != "ollama" {
		return openai.NewOpenAIEmbedderWithOptions(cfg.LocalLLM.APIKey, model, cfg.LocalLLM.BaseURL, timeout),
			fmt.Sprintf("local_llm embedding: %s (model: %s)", cfg.LocalLLM.BaseURL, model)
	}
	baseURL := cfg.Ollama.BaseURL
	if cfg.LocalLLM.Enabled && cfg.LocalLLM.Provider == "ollama" {
		baseURL = cfg.LocalLLM.BaseURL
	}
	return ollama.NewOllamaEmbedder(baseURL, model), fmt.Sprintf("%s (model: %s)", baseURL, model)
}

func startSourceRegistrySweeper(store *conversationpersistence.L1SQLiteStore) {
	sweep := func() {
		result, err := sourcefetcher.SweepDueSources(context.Background(), store, time.Now().UTC(), sourcefetcher.SweepOptions{
			LimitPerSource:    10,
			MinimumTrustScore: 0.5,
		})
		if err != nil {
			log.Printf("WARN: source registry sweep failed: %v", err)
			return
		}
		if result.Sources > 0 || result.Staged > 0 || result.Failed > 0 {
			log.Printf("Source registry sweep complete: sources=%d staged=%d validated=%d promoted_news=%d failed=%d",
				result.Sources, result.Staged, result.Validated, result.PromotedNews, result.Failed)
		}
	}
	go func() {
		sweep()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()
}

func buildLocalAliasProvider(cfg *config.Config, alias, model string, timeout time.Duration, global chan struct{}) llm.LLMProvider {
	var raw llm.LLMProvider
	switch cfg.LocalLLM.Provider {
	case "ollama":
		raw = ollama.NewOllamaProviderWithNumCtx(cfg.LocalLLM.BaseURL, model, 32768)
	default:
		raw = openai.NewOpenAIProviderWithOptions(cfg.LocalLLM.APIKey, model, cfg.LocalLLM.BaseURL, timeout)
	}
	modelSem := make(chan struct{}, cfg.LocalLLM.ModelConcurrency)
	return infrallm.NewLimitedProvider(raw, "local-"+alias+"-"+model, global, modelSem)
}

func warmPrimaryLLMProviders(parent context.Context, providers map[string]llm.LLMProvider, timeout time.Duration) {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	for alias, provider := range providers {
		ctx, cancel := context.WithTimeout(parent, timeout)
		_, err := provider.Generate(ctx, llm.GenerateRequest{
			Messages:  []llm.Message{{Role: "user", Content: "warmup"}},
			MaxTokens: 1,
		})
		cancel()
		if err != nil {
			log.Printf("WARN: local LLM warmup failed alias=%s provider=%s err=%v", alias, provider.Name(), err)
			continue
		}
		log.Printf("Local LLM warmup ok alias=%s provider=%s", alias, provider.Name())
	}
}

func main() {
	cmd := "run"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "run":
		cmdRun()
	case "version", "-v", "--version":
		cmdVersion()
	case "health":
		cmdHealth()
	case "status":
		cmdStatus()
	case "doctor":
		cmdDoctor()
	case "channels":
		cmdChannels()
	case "gateway":
		cmdGateway()
	case "ollama":
		cmdOllama()
	case "logs":
		cmdLogs()
	case "evidence":
		cmdEvidence()
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

	log.Printf("RenCrow %s (commit: %s, built: %s)", Version, Commit, BuildDate)
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
	log.Printf("Starting RenCrow server on %s", addr)

	mux := http.NewServeMux()
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

	// Live Viewer
	debugSystemOpts := viewer.DebugSystemOptions{
		TTSBaseURL: strings.TrimSpace(cfg.TTS.HTTPBaseURL),
		STTBaseURL: inferSTTBaseURL(cfg.TTS.HTTPBaseURL, os.Getenv("STT_PROVIDER_URL")),
	}
	sttProviderURL := inferSTTProviderURL(cfg.TTS.HTTPBaseURL, os.Getenv("STT_PROVIDER_URL"))
	mux.HandleFunc("/viewer", viewer.HandlePage)
	mux.HandleFunc("/viewer/logo.png", viewer.HandleLogo)
	mux.HandleFunc("/viewer/mio-lipsync-closed.svg", viewer.HandleMioLipSyncClosed)
	mux.HandleFunc("/viewer/mio-lipsync-open.svg", viewer.HandleMioLipSyncOpen)
	mux.HandleFunc("/viewer/shiro-lipsync-closed.svg", viewer.HandleShiroLipSyncClosed)
	mux.HandleFunc("/viewer/shiro-lipsync-open.svg", viewer.HandleShiroLipSyncOpen)
	mux.HandleFunc("/viewer/tts/audio", handleLocalTTSAudio(cfg.TTS.OutputDir))
	mux.HandleFunc("/viewer/events", dependencies.eventHub.HandleSSE)
	mux.HandleFunc("/viewer/debug/system", viewer.HandleDebugSystemSnapshot(debugSystemOpts))
	mux.HandleFunc("/viewer/stt/log", viewer.HandleSTTClientLogSave("tmp/client_stt_log.txt"))
	mux.HandleFunc("/viewer/stt/wav", viewer.HandleSTTInputWAVSave("tmp/client_stt_input_latest.wav", "tmp/stt_inputs"))
	mux.HandleFunc("/viewer/stt/autotest", viewer.HandleSTTAutoTest("scripts/stt_e2e_probe.py", "tmp/client_stt_input_latest.wav", "tmp/stt_e2e_from_mic_latest.json"))
	sttGatewayURL := inferSTTGatewayURL(os.Getenv("STT_GATEWAY_URL"), os.Getenv("RENCROW_STT_URL"))
	sttWSHandler := resolveSTTWebSocketHandler(sttProviderURL, sttGatewayURL)
	registerSTTRoutes(mux, sttWSHandler)
	mux.HandleFunc("/audio-router/events", viewer.HandleAudioRouterSSE(dependencies.eventHub))
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
	if dependencies.idleChatOrch != nil {
		mux.HandleFunc("/viewer/idlechat/start", dependencies.handleIdleChatStart())
		mux.HandleFunc("/viewer/idlechat/stop", dependencies.handleIdleChatStop())
		mux.HandleFunc("/viewer/idlechat/status", dependencies.handleIdleChatStatus())
		mux.HandleFunc("/viewer/idlechat/logs", dependencies.handleIdleChatLogs())
		mux.HandleFunc("/viewer/idlechat/forecast", dependencies.handleIdleChatForecast())
		mux.HandleFunc("/viewer/idlechat/story", dependencies.handleIdleChatStory())
		mux.HandleFunc("/viewer/idlechat/story-simple", dependencies.handleIdleChatStorySimple())
	}

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
	code := runHealthCommand(os.Args[2:], buildHealthService(cfg), os.Stdout, os.Stderr, func() time.Time { return time.Now().UTC() })
	if code != 0 {
		os.Exit(code)
	}
}

// cmdStatus はシステム状態の概要を表示
func cmdStatus() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	code := runStatusCommand(
		os.Args[2:],
		cfg,
		buildHealthService(cfg),
		loadExecutionStats,
		loadEvidenceSummary,
		os.Stdout,
		os.Stderr,
		func() time.Time { return time.Now().UTC() },
	)
	if code != 0 {
		os.Exit(code)
	}
}

type healthChecker interface {
	RunChecks(ctx context.Context) domainhealth.HealthReport
}

func runHealthCommand(args []string, checker healthChecker, out io.Writer, _ io.Writer, now func() time.Time) int {
	report := checker.RunChecks(context.Background())
	if hasFlag(args, "--json") {
		writeJSONCLI(out, map[string]any{
			"ok":        report.Status != domainhealth.StatusDown,
			"timestamp": now().Format(time.RFC3339),
			"component": "health",
			"status":    report.Status,
			"details": map[string]any{
				"checks": report.Checks,
			},
		}, true)
	} else {
		writeJSONCLI(out, report, true)
	}
	if report.Status == domainhealth.StatusDown {
		return 1
	}
	return 0
}

func runStatusCommand(
	args []string,
	cfg *config.Config,
	checker healthChecker,
	executionStatsLoader func(cfg *config.Config) (map[domainexecution.Status]int, error),
	evidenceSummaryLoader func(cfg *config.Config) (map[string]map[string]int, error),
	out io.Writer,
	_ io.Writer,
	now func() time.Time,
) int {
	report := checker.RunChecks(context.Background())
	deep := hasFlag(args, "--deep")
	usage := hasFlag(args, "--usage")
	jsonOut := hasFlag(args, "--json")

	stats, statsErr := executionStatsLoader(cfg)
	usageSummary, usageErr := map[string]map[string]int(nil), error(nil)
	if usage {
		usageSummary, usageErr = evidenceSummaryLoader(cfg)
	}

	if jsonOut {
		details := map[string]any{
			"server": map[string]any{
				"host": cfg.Server.Host,
				"port": cfg.Server.Port,
			},
			"ollama": map[string]any{
				"base_url": cfg.Ollama.BaseURL,
				"model":    cfg.Ollama.Model,
			},
		}
		if deep {
			details["checks"] = report.Checks
			if statsErr == nil {
				details["execution"] = map[string]int{
					"running": stats[domainexecution.StatusRunning],
					"denied":  stats[domainexecution.StatusDenied],
					"failed":  stats[domainexecution.StatusFailed],
				}
			} else {
				details["execution_error"] = statsErr.Error()
			}
		}
		if usage {
			if usageErr == nil {
				details["usage"] = usageSummary
			} else {
				details["usage_error"] = usageErr.Error()
			}
		}
		writeJSONCLI(out, map[string]any{
			"ok":        report.Status != domainhealth.StatusDown,
			"timestamp": now().Format(time.RFC3339),
			"component": "status",
			"status":    report.Status,
			"details":   details,
		}, true)
		if report.Status == domainhealth.StatusDown {
			return 1
		}
		return 0
	}
	fmt.Fprintf(out, "RenCrow %s\n", Version)
	fmt.Fprintf(out, "Ollama: %s (model: %s)\n", cfg.Ollama.BaseURL, cfg.Ollama.Model)
	fmt.Fprintf(out, "Server: %s:%d\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Fprintln(out)

	for _, c := range report.Checks {
		fmt.Fprintf(out, "  [%s] %s: %s (%dms)\n", c.Status, c.Name, c.Message, c.Duration.Milliseconds())
	}
	fmt.Fprintf(out, "\nOverall: %s\n", report.Status)

	if statsErr == nil {
		fmt.Fprintln(out, "\nExecution:")
		fmt.Fprintf(out, "  running: %d\n", stats[domainexecution.StatusRunning])
		fmt.Fprintf(out, "  denied: %d\n", stats[domainexecution.StatusDenied])
		fmt.Fprintf(out, "  failed: %d\n", stats[domainexecution.StatusFailed])
	} else {
		fmt.Fprintf(out, "\nExecution: unavailable (%v)\n", statsErr)
	}

	if deep {
		fmt.Fprintln(out, "\nDetails:")
		fmt.Fprintf(out, "  timestamp: %s\n", now().Format(time.RFC3339))
		fmt.Fprintf(out, "  security.enabled: %t\n", cfg.Security.Enabled)
	}
	if usage {
		fmt.Fprintln(out, "\nUsage:")
		if usageErr != nil {
			fmt.Fprintf(out, "  unavailable (%v)\n", usageErr)
		} else {
			writeJSONCLI(out, usageSummary, true)
		}
	}
	if report.Status == domainhealth.StatusDown {
		return 1
	}
	return 0
}

// cmdDoctor は設定の基本診断を実施
func cmdDoctor() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	code := runDoctorCommand(
		os.Args[2:],
		cfg,
		buildHealthService(cfg),
		lineWebhookConfigured(cfg),
		func(p string) error {
			_, err := os.Stat(p)
			return err
		},
		func(p string) error { return os.MkdirAll(p, 0755) },
		os.Stdout,
		os.Stderr,
		func() time.Time { return time.Now().UTC() },
	)
	if code != 0 {
		os.Exit(code)
	}
}

type doctorFinding struct {
	Level string `json:"level"`
	Msg   string `json:"msg"`
	Hint  string `json:"hint,omitempty"`
}

func runDoctorCommand(
	args []string,
	cfg *config.Config,
	checker healthChecker,
	lineConfigured bool,
	statPath func(path string) error,
	ensureDir func(path string) error,
	out io.Writer,
	_ io.Writer,
	now func() time.Time,
) int {
	findings := make([]doctorFinding, 0)

	if cfg.Security.Enabled {
		if cfg.Security.WorkspaceEnforced {
			if err := statPath(cfg.WorkspaceDir); err != nil {
				findings = append(findings, doctorFinding{
					Level: "ERROR",
					Msg:   "workspace_dir does not exist",
					Hint:  "create workspace_dir or set a valid path",
				})
			}
		}
		if cfg.Security.Audit.Enabled {
			auditDir := path.Dir(cfg.Security.Audit.Path)
			if strings.TrimSpace(auditDir) == "" {
				auditDir = "."
			}
			if err := ensureDir(auditDir); err != nil {
				findings = append(findings, doctorFinding{
					Level: "ERROR",
					Msg:   fmt.Sprintf("audit directory is not writable: %s", auditDir),
					Hint:  "set security.audit.path to a writable path",
				})
			}
		}
	}

	if report := checker.RunChecks(context.Background()); report.Status == domainhealth.StatusDown {
		findings = append(findings, doctorFinding{
			Level: "WARN",
			Msg:   "health checks report DOWN",
			Hint:  "verify ollama base_url/model settings",
		})
	}

	hasError := false
	hasWarn := false
	for _, f := range findings {
		switch f.Level {
		case "ERROR":
			hasError = true
		case "WARN":
			hasWarn = true
		}
	}

	if hasFlag(args, "--json") {
		status := "ok"
		if hasError {
			status = "down"
		} else if hasWarn {
			status = "degraded"
		}
		writeJSONCLI(out, map[string]any{
			"ok":        !hasError,
			"timestamp": now().Format(time.RFC3339),
			"component": "doctor",
			"status":    status,
			"details": map[string]any{
				"findings": findings,
			},
		}, true)
		if hasError {
			return 1
		}
		return 0
	}

	if len(findings) == 0 {
		fmt.Fprintln(out, "OK: no issues found")
		return 0
	}

	for _, f := range findings {
		fmt.Fprintf(out, "[%s] %s\n", f.Level, f.Msg)
		if f.Hint != "" {
			fmt.Fprintf(out, "  hint: %s\n", f.Hint)
		}
	}
	if hasError {
		return 1
	}
	return 0
}

func loadExecutionStats(cfg *config.Config) (map[domainexecution.Status]int, error) {
	if !cfg.Security.Audit.Enabled {
		return map[domainexecution.Status]int{}, nil
	}
	repo, err := executionpersistence.NewJSONLRepository(cfg.Security.Audit.Path)
	if err != nil {
		return nil, err
	}
	return repo.CountByStatus(context.Background())
}

func loadEvidenceSummary(cfg *config.Config) (map[string]map[string]int, error) {
	if !cfg.Security.Audit.Enabled {
		return map[string]map[string]int{
			"status": {
				"passed": 0,
				"failed": 0,
				"other":  0,
			},
			"error_kind": {
				"apply":  0,
				"verify": 0,
				"repair": 0,
				"none":   0,
				"other":  0,
			},
		}, nil
	}
	store, err := executionpersistence.NewJSONLReportStore(cfg.Security.Audit.Path)
	if err != nil {
		return nil, err
	}
	return store.Summary(context.Background())
}

func lineWebhookConfigured(cfg *config.Config) bool {
	return strings.TrimSpace(cfg.Line.ChannelSecret) != "" && strings.TrimSpace(cfg.Line.AccessToken) != ""
}

func cmdChannels() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	registry := buildChannelRegistry(cfg)
	code := runChannelsCommand(os.Args[2:], registry, os.Stdout, os.Stderr, func() time.Time { return time.Now().UTC() })
	if code != 0 {
		os.Exit(code)
	}
}

func buildChannelRegistry(cfg *config.Config) *adapterchannels.Registry {
	registry := adapterchannels.NewRegistry()
	if lineWebhookConfigured(cfg) {
		_ = registry.Register(line.NewHandler(nil, cfg.Line.ChannelSecret, cfg.Line.AccessToken))
	}
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		_ = registry.Register(telegramadapter.NewAdapter(cfg.Telegram.BotToken))
	}
	if strings.TrimSpace(cfg.Discord.BotToken) != "" {
		_ = registry.Register(discordadapter.NewAdapter(cfg.Discord.BotToken))
	}
	if strings.TrimSpace(cfg.Slack.BotToken) != "" {
		_ = registry.Register(slackadapter.NewAdapter(cfg.Slack.BotToken, cfg.Slack.SigningSecret))
	}
	return registry
}

type channelRegistry interface {
	List() []string
	ProbeAll(ctx context.Context) map[string]error
}

func runChannelsCommand(args []string, registry channelRegistry, out io.Writer, errOut io.Writer, now func() time.Time) int {
	subcmd := "list"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	jsonOut := hasFlag(args, "--json")

	switch subcmd {
	case "list":
		names := registry.List()
		if jsonOut {
			status := "empty"
			if len(names) > 0 {
				status = "configured"
			}
			writeJSONCLI(out, map[string]any{
				"ok":        true,
				"timestamp": now().Format(time.RFC3339),
				"component": "channels",
				"status":    status,
				"details": map[string]any{
					"channels": names,
				},
			}, true)
			return 0
		}
		if len(names) == 0 {
			fmt.Fprintln(out, "No channels configured")
			return 0
		}
		fmt.Fprintln(out, "Configured channels:")
		for _, name := range names {
			fmt.Fprintf(out, "  - %s\n", name)
		}
		return 0
	case "probe":
		results := registry.ProbeAll(context.Background())
		names := registry.List()
		if len(results) == 0 {
			if jsonOut {
				writeJSONCLI(out, map[string]any{
					"ok":        true,
					"timestamp": now().Format(time.RFC3339),
					"component": "channels",
					"status":    "empty",
					"details": map[string]any{
						"results": map[string]any{},
					},
				}, true)
				return 0
			}
			fmt.Fprintln(out, "No channels configured")
			return 0
		}
		hasErr := false
		perChannel := make(map[string]map[string]any, len(names))
		for _, name := range names {
			err := results[name]
			if err != nil {
				hasErr = true
				perChannel[name] = map[string]any{"ok": false, "error": err.Error()}
				if !jsonOut {
					fmt.Fprintf(out, "[DOWN] %s: %v\n", name, err)
				}
				continue
			}
			perChannel[name] = map[string]any{"ok": true}
			if !jsonOut {
				fmt.Fprintf(out, "[OK] %s\n", name)
			}
		}
		if jsonOut {
			status := "ok"
			if hasErr {
				status = "degraded"
			}
			writeJSONCLI(out, map[string]any{
				"ok":        !hasErr,
				"timestamp": now().Format(time.RFC3339),
				"component": "channels",
				"status":    status,
				"details": map[string]any{
					"results": perChannel,
				},
			}, true)
		}
		if hasErr {
			return 1
		}
		return 0
	default:
		fmt.Fprintf(errOut, "unknown channels subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw channels [list|probe]")
		return 1
	}
}

func cmdGateway() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	getStatus := func(url string) (int, error) {
		client := &http.Client{Timeout: 2 * time.Second}
		resp, err := client.Get(url) //nolint:gosec // local health probe
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		return resp.StatusCode, nil
	}
	restart := func() error {
		return exec.Command("systemctl", "restart", "picoclaw.service").Run()
	}
	code := runGatewayCommand(os.Args[2:], cfg, os.Stdout, os.Stderr, getStatus, restart, func() time.Time { return time.Now().UTC() })
	if code != 0 {
		os.Exit(code)
	}
}

func cmdOllama() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	target, restart, err := buildOllamaRestartAction(cfg)
	if err != nil {
		log.Fatalf("Failed to build ollama restart action: %v", err)
	}
	code := runOllamaCommand(
		os.Args[2:],
		cfg,
		buildHealthService(cfg),
		os.Stdout,
		os.Stderr,
		target,
		restart,
		func() time.Time { return time.Now().UTC() },
	)
	if code != 0 {
		os.Exit(code)
	}
}

func runGatewayCommand(
	args []string,
	cfg *config.Config,
	out io.Writer,
	errOut io.Writer,
	getStatus func(url string) (statusCode int, err error),
	restart func() error,
	now func() time.Time,
) int {
	subcmd := "status"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	jsonOut := hasFlag(args, "--json")

	switch subcmd {
	case "status":
		url := gatewayHealthURL(cfg)
		code, err := getStatus(url)
		if err != nil {
			if jsonOut {
				writeJSONCLI(out, map[string]any{
					"ok":        false,
					"timestamp": now().Format(time.RFC3339),
					"component": "gateway",
					"status":    "down",
					"code":      "E_GATEWAY_UNREACHABLE",
					"hint":      "picoclaw gateway restart を実行",
					"details": map[string]any{
						"url":   url,
						"error": err.Error(),
					},
				}, true)
			} else {
				fmt.Fprintf(out, "[DOWN] gateway health check failed: %v\n", err)
			}
			return 1
		}
		if code >= 200 && code < 300 {
			if jsonOut {
				writeJSONCLI(out, map[string]any{
					"ok":        true,
					"timestamp": now().Format(time.RFC3339),
					"component": "gateway",
					"status":    "running",
					"details": map[string]any{
						"url":         url,
						"status_code": code,
					},
				}, true)
			} else {
				fmt.Fprintf(out, "[OK] gateway reachable: %s (%d)\n", url, code)
			}
			return 0
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{
				"ok":        false,
				"timestamp": now().Format(time.RFC3339),
				"component": "gateway",
				"status":    "down",
				"code":      "E_GATEWAY_UNHEALTHY",
				"hint":      "health endpoint と logs を確認",
				"details": map[string]any{
					"url":         url,
					"status_code": code,
				},
			}, true)
		} else {
			fmt.Fprintf(out, "[DOWN] gateway unhealthy: %s (%d)\n", url, code)
		}
		return 1
	case "restart":
		if err := restart(); err != nil {
			if jsonOut {
				writeJSONCLI(out, map[string]any{
					"ok":        false,
					"timestamp": now().Format(time.RFC3339),
					"component": "gateway",
					"status":    "down",
					"code":      "E_GATEWAY_RESTART_FAILED",
					"hint":      "systemctl権限とサービス名を確認",
					"details": map[string]any{
						"error": err.Error(),
					},
				}, true)
			} else {
				fmt.Fprintf(out, "failed to restart via systemctl: %v\n", err)
			}
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{
				"ok":        true,
				"timestamp": now().Format(time.RFC3339),
				"component": "gateway",
				"status":    "restarted",
				"details":   map[string]any{},
			}, true)
		} else {
			fmt.Fprintln(out, "picoclaw.service restarted")
		}
		return 0
	default:
		fmt.Fprintf(errOut, "unknown gateway subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw gateway [status|restart]")
		return 1
	}
}

func runOllamaCommand(
	args []string,
	cfg *config.Config,
	checker healthChecker,
	out io.Writer,
	errOut io.Writer,
	restartTarget string,
	restart func() error,
	now func() time.Time,
) int {
	subcmd := "status"
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	jsonOut := hasFlag(args, "--json")

	switch subcmd {
	case "status":
		report := checker.RunChecks(context.Background())
		if jsonOut {
			writeJSONCLI(out, map[string]any{
				"ok":        report.Status != domainhealth.StatusDown,
				"timestamp": now().Format(time.RFC3339),
				"component": "ollama",
				"status":    report.Status,
				"details": map[string]any{
					"base_url": cfg.Ollama.BaseURL,
					"model":    cfg.Ollama.Model,
					"checks":   report.Checks,
				},
			}, true)
		} else {
			fmt.Fprintf(out, "Ollama: %s (model: %s)\n", cfg.Ollama.BaseURL, cfg.Ollama.Model)
			for _, c := range report.Checks {
				fmt.Fprintf(out, "  [%s] %s: %s (%dms)\n", c.Status, c.Name, c.Message, c.Duration.Milliseconds())
			}
			fmt.Fprintf(out, "\nOverall: %s\n", report.Status)
		}
		if report.Status == domainhealth.StatusDown {
			return 1
		}
		return 0
	case "restart":
		if err := restart(); err != nil {
			if jsonOut {
				writeJSONCLI(out, map[string]any{
					"ok":        false,
					"timestamp": now().Format(time.RFC3339),
					"component": "ollama",
					"status":    "down",
					"code":      "E_OLLAMA_RESTART_FAILED",
					"hint":      "ollama service と SSH 設定を確認",
					"details": map[string]any{
						"base_url": cfg.Ollama.BaseURL,
						"target":   restartTarget,
						"error":    err.Error(),
					},
				}, true)
			} else {
				fmt.Fprintf(errOut, "failed to restart ollama via %s: %v\n", restartTarget, err)
			}
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{
				"ok":        true,
				"timestamp": now().Format(time.RFC3339),
				"component": "ollama",
				"status":    "restarted",
				"details": map[string]any{
					"base_url": cfg.Ollama.BaseURL,
					"model":    cfg.Ollama.Model,
					"target":   restartTarget,
				},
			}, true)
		} else {
			fmt.Fprintf(out, "ollama restarted via %s\n", restartTarget)
		}
		return 0
	default:
		fmt.Fprintf(errOut, "unknown ollama subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw ollama [status|restart]")
		return 1
	}
}

func gatewayHealthURL(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/health", host, cfg.Server.Port)
}

func buildOllamaRestartAction(cfg *config.Config) (string, func() error, error) {
	u, err := url.Parse(strings.TrimSpace(cfg.Ollama.BaseURL))
	if err != nil {
		return "", nil, fmt.Errorf("invalid ollama.base_url: %w", err)
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		host = "127.0.0.1"
	}
	restartCmd := strings.TrimSpace(os.Getenv("PICOCLAW_OLLAMA_RESTART_CMD"))
	if restartCmd == "" {
		restartCmd = "sudo systemctl restart ollama"
	}
	if isLocalOllamaHost(host) {
		return "local systemctl", func() error {
			return exec.Command("bash", "-lc", restartCmd).Run()
		}, nil
	}

	sshUser := strings.TrimSpace(os.Getenv("PICOCLAW_OLLAMA_SSH_USER"))
	if sshUser == "" {
		sshUser = strings.TrimSpace(os.Getenv("USER"))
	}
	if sshUser == "" {
		return "", nil, fmt.Errorf("PICOCLAW_OLLAMA_SSH_USER is required for remote ollama restart")
	}

	sshArgs := []string{fmt.Sprintf("%s@%s", sshUser, host), restartCmd}
	if keyPath := strings.TrimSpace(os.Getenv("PICOCLAW_OLLAMA_SSH_KEY_PATH")); keyPath != "" {
		sshArgs = append([]string{"-i", keyPath}, sshArgs...)
	}

	target := fmt.Sprintf("ssh %s@%s", sshUser, host)
	return target, func() error {
		return exec.Command("ssh", sshArgs...).Run()
	}, nil
}

func isLocalOllamaHost(host string) bool {
	switch strings.TrimSpace(strings.ToLower(host)) {
	case "", "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return true
	default:
		return false
	}
}

func cmdLogs() {
	cfg, err := config.LoadConfig(getConfigPath())
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	logPath := os.Getenv("PICOCLAW_LOG_PATH")
	if strings.TrimSpace(logPath) == "" {
		logPath = "picoclaw.log"
	}
	code := runLogsCommand(
		os.Args[2:],
		logPath,
		os.Stdout,
		os.Stderr,
		printLastLinesTo,
		followFileTo,
		func() time.Time { return time.Now().UTC() },
	)
	if code != 0 {
		os.Exit(code)
	}

	_ = cfg // keep config load validation for command consistency
}

func runLogsCommand(
	args []string,
	logPath string,
	out io.Writer,
	errOut io.Writer,
	tailFn func(path string, n int, out io.Writer) error,
	followFn func(path string, out io.Writer) error,
	now func() time.Time,
) int {
	follow := hasFlag(args, "--follow")
	jsonOut := hasFlag(args, "--json")

	if jsonOut {
		status := "snapshot"
		if follow {
			status = "streaming"
		}
		writeJSONCLI(out, map[string]any{
			"ok":        true,
			"timestamp": now().Format(time.RFC3339),
			"component": "logs",
			"status":    status,
			"details": map[string]any{
				"path":   logPath,
				"follow": follow,
			},
		}, false)
	}

	if err := tailFn(logPath, 100, out); err != nil {
		fmt.Fprintf(errOut, "failed to read logs: %v\n", err)
		return 1
	}
	if !follow {
		return 0
	}
	if err := followFn(logPath, out); err != nil {
		fmt.Fprintf(errOut, "failed to follow logs: %v\n", err)
		return 1
	}
	return 0
}

func cmdEvidence() {
	configPath := getConfigPath()
	store, err := loadEvidenceStore(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize evidence store: %v\n", err)
		os.Exit(1)
	}
	code := runEvidenceCommand(os.Args[2:], store, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

type evidenceStore interface {
	ListRecent(ctx context.Context, limit int) ([]domainexecution.ExecutionReport, error)
	GetByJobID(ctx context.Context, jobID string) (domainexecution.ExecutionReport, error)
	Summary(ctx context.Context) (map[string]map[string]int, error)
}

func runEvidenceCommand(args []string, store evidenceStore, out io.Writer, errOut io.Writer) int {
	subcmd := "list"
	if len(args) > 0 {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	compact := hasFlag(args, "--compact")
	pretty := !compact

	switch subcmd {
	case "list":
		limit, jsonOut, statusFilter, errorKindFilter, sinceHours, parseErr := parseEvidenceListArgs(args[1:])
		if parseErr != nil {
			fmt.Fprintf(errOut, "%v\n", parseErr)
			return 1
		}
		items, err := store.ListRecent(context.Background(), limit)
		if err != nil {
			fmt.Fprintf(errOut, "failed to list evidence: %v\n", err)
			return 1
		}
		items = filterEvidence(items, statusFilter, errorKindFilter, sinceHours)
		if jsonOut {
			writeJSONCLI(out, map[string]any{"items": items}, pretty)
			return 0
		}
		if len(items) == 0 {
			fmt.Fprintln(out, "No evidence records")
			return 0
		}
		for _, it := range items {
			fmt.Fprintf(out, "%s | %s | %s | %s\n", it.JobID, it.Status, it.ErrorKind, it.Goal)
		}
		return 0
	case "show":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			fmt.Fprintln(errOut, "usage: picoclaw evidence show <job_id>")
			return 1
		}
		jobID := strings.TrimSpace(args[1])
		item, err := store.GetByJobID(context.Background(), jobID)
		if err != nil {
			fmt.Fprintf(errOut, "failed to get evidence: %v\n", err)
			return 1
		}
		writeJSONCLI(out, item, pretty)
		return 0
	case "summary":
		_, _, statusFilter, errorKindFilter, sinceHours, parseErr := parseEvidenceListArgs(args[1:])
		if parseErr != nil {
			fmt.Fprintf(errOut, "%v\n", parseErr)
			return 1
		}
		var summary map[string]map[string]int
		if statusFilter == "" && errorKindFilter == "" && sinceHours <= 0 {
			s, err := store.Summary(context.Background())
			if err != nil {
				fmt.Fprintf(errOut, "failed to summarize evidence: %v\n", err)
				return 1
			}
			summary = s
		} else {
			items, err := store.ListRecent(context.Background(), 10000)
			if err != nil {
				fmt.Fprintf(errOut, "failed to summarize evidence: %v\n", err)
				return 1
			}
			items = filterEvidence(items, statusFilter, errorKindFilter, sinceHours)
			summary = summarizeEvidence(items)
		}
		writeJSONCLI(out, map[string]any{"summary": summary}, pretty)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown evidence subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw evidence [list|show|summary]")
		return 1
	}
}

func parseEvidenceListArgs(args []string) (limit int, jsonOut bool, statusFilter string, errorKindFilter string, sinceHours int, parseErr error) {
	limit = 20
	validStatus := map[string]struct{}{
		"passed": {},
		"failed": {},
		"other":  {},
	}
	validErrorKind := map[string]struct{}{
		"apply":  {},
		"verify": {},
		"repair": {},
		"none":   {},
		"other":  {},
	}
	for i := 0; i < len(args); i++ {
		v := strings.TrimSpace(strings.ToLower(args[i]))
		if v == "--json" {
			jsonOut = true
			continue
		}
		if v == "--status" && i+1 < len(args) {
			statusFilter = strings.TrimSpace(strings.ToLower(args[i+1]))
			if _, ok := validStatus[statusFilter]; !ok {
				parseErr = fmt.Errorf("invalid --status: %s", strings.TrimSpace(args[i+1]))
				return
			}
			i++
			continue
		}
		if v == "--error-kind" && i+1 < len(args) {
			errorKindFilter = strings.TrimSpace(strings.ToLower(args[i+1]))
			if _, ok := validErrorKind[errorKindFilter]; !ok {
				parseErr = fmt.Errorf("invalid --error-kind: %s", strings.TrimSpace(args[i+1]))
				return
			}
			i++
			continue
		}
		if v == "--since-hours" && i+1 < len(args) {
			n, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil || n <= 0 {
				parseErr = fmt.Errorf("invalid --since-hours: %s", strings.TrimSpace(args[i+1]))
				return
			}
			sinceHours = n
			i++
			continue
		}
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	return
}

func filterEvidence(items []domainexecution.ExecutionReport, statusFilter, errorKindFilter string, sinceHours int) []domainexecution.ExecutionReport {
	if statusFilter == "" && errorKindFilter == "" && sinceHours <= 0 {
		return items
	}
	var cutoff time.Time
	if sinceHours > 0 {
		cutoff = time.Now().UTC().Add(-time.Duration(sinceHours) * time.Hour)
	}
	filtered := make([]domainexecution.ExecutionReport, 0, len(items))
	for _, it := range items {
		if statusFilter != "" && strings.ToLower(strings.TrimSpace(it.Status)) != statusFilter {
			continue
		}
		if errorKindFilter != "" && strings.ToLower(strings.TrimSpace(it.ErrorKind)) != errorKindFilter {
			continue
		}
		if !cutoff.IsZero() {
			ref := it.FinishedAt
			if ref.IsZero() {
				ref = it.CreatedAt
			}
			if ref.IsZero() || ref.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, it)
	}
	return filtered
}

func summarizeEvidence(items []domainexecution.ExecutionReport) map[string]map[string]int {
	summary := map[string]map[string]int{
		"status": {
			"passed": 0,
			"failed": 0,
			"other":  0,
		},
		"error_kind": {
			"apply":  0,
			"verify": 0,
			"repair": 0,
			"none":   0,
			"other":  0,
		},
	}
	for _, it := range items {
		switch strings.ToLower(strings.TrimSpace(it.Status)) {
		case "passed":
			summary["status"]["passed"]++
		case "failed":
			summary["status"]["failed"]++
		default:
			summary["status"]["other"]++
		}
		k := strings.ToLower(strings.TrimSpace(it.ErrorKind))
		switch k {
		case "apply":
			summary["error_kind"]["apply"]++
		case "verify":
			summary["error_kind"]["verify"]++
		case "repair":
			summary["error_kind"]["repair"]++
		case "":
			summary["error_kind"]["none"]++
		default:
			summary["error_kind"]["other"]++
		}
	}
	return summary
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if strings.EqualFold(strings.TrimSpace(a), flag) {
			return true
		}
	}
	return false
}

func writeJSONCLI(out io.Writer, v any, pretty bool) {
	enc := json.NewEncoder(out)
	if pretty {
		enc.SetIndent("", "  ")
	}
	_ = enc.Encode(v)
}

func loadEvidenceStore(configPath string) (*executionpersistence.JSONLReportStore, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	p := strings.TrimSpace(cfg.Security.Audit.Path)
	if p == "" {
		p = defaultExecutionReportPath(cfg.WorkspaceDir)
	}
	return executionpersistence.NewJSONLReportStore(p)
}

func printLastLines(path string, n int) error {
	return printLastLinesTo(path, n, os.Stdout)
}

func printLastLinesTo(path string, n int, out io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	lines := make([]string, 0, n)
	s := bufio.NewScanner(f)
	for s.Scan() {
		lines = append(lines, s.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	for _, line := range lines {
		fmt.Fprintln(out, line)
	}
	return nil
}

func followFile(path string) error {
	return followFileTo(path, os.Stdout)
}

func followFileTo(path string, out io.Writer) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, _ = f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		for {
			line, err := reader.ReadString('\n')
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			fmt.Fprint(out, line)
		}
	}
	return nil
}

// cmdHelp はヘルプメッセージを表示
func cmdHelp() {
	fmt.Printf(`RenCrow %s - Multi-LLM AI Assistant (RenCrow)

Usage: picoclaw [command]

Commands:
  run       Start the HTTP server (default)
  version   Show version information
  health    Run health checks and output JSON
  status    Show system status overview
  doctor    Diagnose config and runtime prerequisites
  channels  List/probe channel adapters
  gateway   Gateway status/restart operations
  ollama    Ollama status/restart operations
  logs      Show logs (use --follow to stream)
  evidence  List/show/summarize execution evidence
  help      Show this help message

Agent Mode:
  Use picoclaw-agent binary for distributed execution.
  See install-agent.sh or install-agent.ps1 for setup.
`, Version)
}

// buildHealthService は HealthService を構築（CLI コマンドで共用）
func buildHealthService(cfg *config.Config) *healthapp.HealthService {
	checks := []domainhealth.Check{
		infrahealth.NewOllamaCheck(cfg.Ollama.BaseURL),
	}
	requirements := collectOllamaHealthRequirements(cfg)
	for _, req := range requirements {
		checks = append(checks, infrahealth.NewOllamaModelCheck(cfg.Ollama.BaseURL, req.Name))
	}

	// 常駐モデルのコンテキスト長チェック（max_context が設定されている場合のみ）
	if cfg.Ollama.MaxContext > 0 {
		checks = append(checks, infrahealth.NewOllamaModelsCheck(
			cfg.Ollama.BaseURL,
			requirements,
		))
	}

	return healthapp.NewHealthService(checks...)
}

func collectOllamaHealthRequirements(cfg *config.Config) []infrahealth.ModelRequirement {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	add := func(out []infrahealth.ModelRequirement, name string) []infrahealth.ModelRequirement {
		name = strings.TrimSpace(name)
		if name == "" {
			return out
		}
		if _, ok := seen[name]; ok {
			return out
		}
		seen[name] = struct{}{}
		return append(out, infrahealth.ModelRequirement{Name: name, MaxContext: cfg.Ollama.MaxContext})
	}

	out := make([]infrahealth.ModelRequirement, 0, 3)
	out = add(out, cfg.Ollama.Model)
	out = add(out, cfg.Ollama.WorkerModel)
	return out
}

func inferSTTBaseURL(ttsBaseURL, sttProviderURL string) string {
	if base := extractBaseFromProviderURL(sttProviderURL); base != "" {
		return base
	}
	u, err := url.Parse(strings.TrimSpace(ttsBaseURL))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s:%d", u.Scheme, u.Hostname(), 8080)
}

func extractBaseFromProviderURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

func inferSTTProviderURL(ttsBaseURL, sttProviderURL string) string {
	raw := strings.TrimSpace(sttProviderURL)
	if raw != "" {
		return raw
	}
	base := inferSTTBaseURL(ttsBaseURL, sttProviderURL)
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/inference"
}

func inferSTTGatewayURL(sttGatewayURL, rencrowSTTURL string) string {
	if v := strings.TrimSpace(sttGatewayURL); v != "" {
		return v
	}
	return strings.TrimSpace(rencrowSTTURL)
}

func resolveSTTWebSocketHandler(sttProviderURL, sttGatewayURL string) http.Handler {
	sttWSHandler := handleSTTWebSocket(sttProviderURL)
	if strings.TrimSpace(sttGatewayURL) != "" {
		sttWSHandler = handleSTTWebSocketProxy(sttGatewayURL)
	}
	return sttWSHandler
}

func registerSTTRoutes(mux *http.ServeMux, sttWSHandler http.Handler) {
	// Primary endpoint is /stt. Keep /stt-ws and /ws for backward compatibility.
	mux.Handle("/stt", sttWSHandler)
	mux.Handle("/stt-ws", sttWSHandler)
	mux.Handle("/ws", sttWSHandler)
}

// handleSTTWebSocketProxy は /stt を voice-bridge（STT Gateway）へ透過プロキシする。
// STT_GATEWAY_URL または RENCROW_STT_URL に voice-bridge の WebSocket URL を設定すると有効になる。
// 例: RENCROW_STT_URL=ws://192.168.1.36:8090/stt
func handleSTTWebSocketProxy(gatewayURL string) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		origin := "http://localhost/"
		gw, err := websocket.Dial(gatewayURL, "", origin)
		if err != nil {
			_ = sendSTTError(conn, "voice-bridge unavailable: "+err.Error())
			return
		}
		defer gw.Close()

		errc := make(chan error, 2)
		relay := func(src, dst *websocket.Conn) {
			for {
				var msg []byte
				if err := websocket.Message.Receive(src, &msg); err != nil {
					errc <- err
					return
				}
				var sendErr error
				if src.PayloadType == websocket.TextFrame {
					sendErr = websocket.Message.Send(dst, string(msg))
				} else {
					sendErr = websocket.Message.Send(dst, msg)
				}
				if sendErr != nil {
					errc <- sendErr
					return
				}
			}
		}
		go relay(conn, gw) // browser → voice-bridge
		go relay(gw, conn) // voice-bridge → browser
		<-errc
	})
}

func handleSTTWebSocket(sttProviderURL string) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if strings.TrimSpace(sttProviderURL) == "" {
			_ = sendSTTError(conn, "stt provider url is not configured")
			return
		}

		autoFinalTimeout := sttFinalTimeoutFromEnv()
		silenceThreshold := sttSilenceAbsThresholdFromEnv()
		adaptiveInferTimeout := sttHTTPTimeoutFromEnv()
		speechStarted := false
		lastDraft := ""
		lastDraftAt := time.Time{}
		lastVoiceAt := time.Time{}
		inferCooldownUntil := time.Time{}
		lastTimeoutNotice := time.Time{}
		timeoutStreak := 0
		successStreak := 0
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastDraftAt.IsZero() && time.Since(lastDraftAt) >= autoFinalTimeout {
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": strings.TrimSpace(lastDraft),
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
					continue
				}
				return
			}
			if len(payload) == 0 {
				continue
			}

			control, isControl := parseSTTControlMessage(payload)
			if isControl {
				if control == "final_pending" {
					finalText := strings.TrimSpace(lastDraft)
					if finalText != "" {
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": finalText,
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
				}
				continue
			}
			if isLikelySilentWAV(payload, silenceThreshold) {
				if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastVoiceAt.IsZero() && time.Since(lastVoiceAt) >= autoFinalTimeout {
					_ = sendSTTEvent(conn, map[string]any{
						"type": "final",
						"text": strings.TrimSpace(lastDraft),
					})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
				}
				continue
			}
			lastVoiceAt = time.Now()
			if !inferCooldownUntil.IsZero() && time.Now().Before(inferCooldownUntil) {
				continue
			}
			if !speechStarted {
				speechStarted = true
				_ = sendSTTEvent(conn, map[string]any{"type": "speech_start"})
			}

			text, err := sttInferViaHTTP(sttProviderURL, payload, adaptiveInferTimeout)
			if err != nil {
				if isSTTTimeoutErr(err) {
					timeoutStreak++
					successStreak = 0
					if timeoutStreak >= 2 {
						adaptiveInferTimeout = adjustAdaptiveSTTTimeout(adaptiveInferTimeout, 300*time.Millisecond, 1200*time.Millisecond, 3200*time.Millisecond)
					}
					inferCooldownUntil = time.Now().Add(800 * time.Millisecond)
					if speechStarted && strings.TrimSpace(lastDraft) != "" {
						// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": strings.TrimSpace(lastDraft),
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						lastVoiceAt = time.Time{}
						speechStarted = false
					}
					// Keep UI informative without error spam when provider stalls.
					if time.Since(lastTimeoutNotice) > 3*time.Second {
						lastTimeoutNotice = time.Now()
						_ = sendSTTEvent(conn, map[string]any{
							"type": "status",
							"text": "stt provider timeout (retrying)",
						})
					}
					continue
				}
				if speechStarted && strings.TrimSpace(lastDraft) != "" {
					// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
					_ = sendSTTEvent(conn, map[string]any{
						"type": "final",
						"text": strings.TrimSpace(lastDraft),
					})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
					continue
				}
				_ = sendSTTError(conn, "stt inference failed: "+err.Error())
				continue
			}
			normalized := strings.TrimSpace(text)
			if normalized == "" {
				continue
			}
			successStreak++
			timeoutStreak = 0
			if successStreak >= 4 {
				adaptiveInferTimeout = adjustAdaptiveSTTTimeout(adaptiveInferTimeout, -100*time.Millisecond, 1200*time.Millisecond, 3200*time.Millisecond)
				successStreak = 0
			}
			inferCooldownUntil = time.Time{}
			lastDraft = normalized
			lastDraftAt = time.Now()
			_ = sendSTTEvent(conn, map[string]any{
				"type": "draft",
				"text": normalized,
			})
		}
	})
}

func sttFinalTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_FINAL_TIMEOUT_MS"))
	if raw == "" {
		return 1200 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 200 {
		return 1200 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func sttSilenceAbsThresholdFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("STT_SILENCE_ABS_THRESHOLD"))
	if raw == "" {
		return 220
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 220
	}
	return v
}

func isLikelySilentWAV(wav []byte, absThreshold int) bool {
	if len(wav) <= 44 {
		return false
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return false
	}
	sampleBytes := wav[44:]
	if len(sampleBytes) < 2 {
		return false
	}
	var sum int64
	var n int64
	for i := 0; i+1 < len(sampleBytes); i += 2 {
		s := int16(sampleBytes[i]) | int16(sampleBytes[i+1])<<8
		if s < 0 {
			sum += int64(-s)
		} else {
			sum += int64(s)
		}
		n++
	}
	if n == 0 {
		return false
	}
	avgAbs := int(sum / n)
	return avgAbs < absThreshold
}

func parseSTTControlMessage(payload []byte) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	lead := payload[0]
	if lead != '{' && lead != '[' && lead != '"' {
		return "", false
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return "", false
	}
	msgType, _ := obj["type"].(string)
	if strings.TrimSpace(msgType) != "" {
		return strings.TrimSpace(msgType), true
	}
	return "", false
}

func sendSTTEvent(conn *websocket.Conn, event map[string]any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(b))
}

func sendSTTError(conn *websocket.Conn, message string) error {
	return sendSTTEvent(conn, map[string]any{
		"type":  "error",
		"error": strings.TrimSpace(message),
	})
}

func sttInferViaHTTP(providerURL string, wav []byte, timeout time.Duration) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, providerURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Text, nil
}

func adjustAdaptiveSTTTimeout(cur, delta, minV, maxV time.Duration) time.Duration {
	next := cur + delta
	if next < minV {
		return minV
	}
	if next > maxV {
		return maxV
	}
	return next
}

func sttHTTPTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_TIMEOUT_MS"))
	if raw == "" {
		return 3000 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 300 {
		return 3000 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func isSTTTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client.timeout exceeded") || strings.Contains(msg, "context deadline exceeded")
}

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
	sshTransports        map[string]domaintransport.Transport   // v4 SSH transports
	heartbeatSvc         *heartbeat.HeartbeatService            // heartbeat service
	toolRegistry         capdomain.ToolRegistry                 // Phase 4: Shiro ツール共有用 ToolRegistry
}

type idleAwareEventListener struct {
	hub      *viewer.EventHub
	monitor  *viewer.MonitorStore
	archive  *viewer.EventLogStore
	mu       sync.RWMutex
	idleChat *idlechat.IdleChatOrchestrator
}

func (l *idleAwareEventListener) SetIdleChat(idle *idlechat.IdleChatOrchestrator) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.idleChat = idle
}

func (l *idleAwareEventListener) OnEvent(ev orchestrator.OrchestratorEvent) {
	if l.archive != nil {
		if err := l.archive.Append(ev); err != nil {
			log.Printf("WARN: failed to append viewer event log: %v", err)
		}
	}
	l.hub.OnEvent(ev)
	if l.monitor != nil {
		l.monitor.OnEvent(ev)
	}
	if !shouldStopIdleChatByEvent(ev) {
		return
	}
	l.mu.RLock()
	idle := l.idleChat
	l.mu.RUnlock()
	if idle != nil {
		idle.NotifyActivity()
	}
}

func shouldStopIdleChatByEvent(ev orchestrator.OrchestratorEvent) bool {
	if strings.EqualFold(ev.Route, "IDLECHAT") {
		return false
	}
	if ev.Type == "tts.audio_chunk" || strings.EqualFold(ev.From, "tts") {
		return false
	}
	if ev.Type == "message.received" {
		return true
	}
	if ev.Type == "entry.stage" {
		stage := strings.ToLower(strings.TrimSpace(ev.Content))
		return stage == "received" || stage == "planning"
	}
	return false
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
	mioAgent := agent.NewMioAgent(chatProvider, classifier, ruleDictionary, chatToolRunner, mcpClient, convEngine)
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
	shiroAgent := agent.NewShiroAgent(workerProvider, workerToolRunner, mcpClient, cfg.Prompts.Worker, subagentMgr)
	wildAgent := agent.NewWildAgent(wildProvider, "")
	if convEngine != nil {
		shiroAgent.WithConversationEngine(convEngine)
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
		return viewer.HandleSend(func(ctx context.Context, message string) (string, error) {
			log.Printf("[main] viewerSendFromOrch: calling ProcessMessage for viewer message: %q", message)
			resp, err := proc.ProcessMessage(ctx, orchestrator.ProcessMessageRequest{
				SessionID:   "viewer",
				Channel:     "viewer",
				ChatID:      "viewer-user",
				UserMessage: message,
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
		})
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
			cfg.Prompts.IdleChatAgents,
			cfg.IdleChat.StoryDataDir,
		)
		idleChatOrch.SetIntervalSeconds(cfg.IdleChat.IntervalSec)
		idleChatOrch.SetSpeakerProviders(map[string]llm.LLMProvider{
			"mio":   chatProvider,
			"shiro": workerProvider,
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
					deps.eventHub.OnEvent(orchestrator.NewEvent(
						viewerType,
						ev.From,
						ev.To,
						ev.Content,
						"IDLECHAT",
						"",
						ev.SessionID,
						"idlechat",
						"idlechat",
					))
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
		deps.buildDistributedMode(cfg, sessionRepo, mioAgent, shiroAgent, coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter, workerExecutionService, chatProvider, centralMemory, ttsBridge, vtuberBridge)
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

// buildCoderCapabilities は NodeCapabilities と config から []CoderCapability を構築する（Phase 3）
func buildCoderCapabilities(nodeCaps capdomain.NodeCapabilities, cfg *config.Config) []capdomain.CoderCapability {
	// 検出結果の LLM を "provider/model" でインデックス
	detected := make(map[string]capdomain.LLMCapability)
	for _, l := range nodeCaps.LLMs {
		detected[l.ProviderName+"/"+l.ModelName] = l
	}

	// プロバイダー別デフォルト品質（llm_quality_overrides に記載がない場合の fallback）
	providerDefault := map[string]int{
		"claude": 5, "openai": 4, "deepseek": 3, "ollama": 2,
	}

	type coderEntry struct {
		name string
		cc   config.CoderConfig
	}
	entries := []coderEntry{
		{"coder1", cfg.Coder1},
		{"coder2", cfg.Coder2},
		{"coder3", cfg.Coder3},
		{"coder4", cfg.Coder4},
	}

	caps := make([]capdomain.CoderCapability, 0, len(entries))
	anyUsable := false
	for _, e := range entries {
		var quality int
		var available bool

		if l, ok := detected[e.cc.Provider+"/"+e.cc.Model]; ok {
			quality = l.Quality
			available = e.cc.Enabled && l.Available
		} else {
			quality = cfg.Capability.LLMQualityOverrides[e.cc.Model]
			if quality == 0 {
				quality = providerDefault[e.cc.Provider]
			}
			available = e.cc.Enabled && e.cc.APIKey != ""
		}

		if quality > 0 {
			anyUsable = true
		}
		caps = append(caps, capdomain.CoderCapability{
			Name:      e.name,
			Quality:   quality,
			Available: available,
		})
	}

	if !anyUsable {
		return nil // 品質情報なし → 静的チェーンにフォールバック
	}
	return caps
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
	shiroAgent *agent.ShiroAgent,
	coder1Adapter *coderAdapter,
	coder2Adapter *coderAdapter,
	coder3Adapter *coderAdapter,
	coder4Adapter *coderAdapter,
	workerExecution service.WorkerExecutionService,
	ollamaProvider llm.LLMProvider,
	centralMemory *domainsession.CentralMemory,
	ttsBridge orchestrator.TTSBridge,
	vtuberBridge orchestrator.VTuberBridge,
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
	localTransports := make(map[string]*transport.LocalTransport)

	for agentName, t := range transports {
		switch v := t.(type) {
		case *transport.LocalTransport:
			if !localAgentEnabled(agentName, coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter) {
				log.Printf("Skipped LocalTransport for agent '%s' (agent not enabled in this process)", agentName)
				continue
			}
			router.RegisterAgent(agentName, v)
			localTransports[agentName] = v
			log.Printf("Registered LocalTransport for agent '%s'", agentName)
		case *transport.SSHTransport:
			// SSH接続失敗は対象 coder の縮退として扱い、Chat/Worker の起動は継続する。
			if err := registerSSHTransport(agentName, v, v, sshTransports); err != nil {
				reason := formatAgentUnavailableReason("ssh connect failed", err)
				markAgentUnavailable(d.monitorStore, agentName, reason)
				if d.eventRelay != nil {
					d.eventRelay.OnEvent(orchestrator.NewEvent(
						"agent.unavailable",
						agentName,
						"system",
						reason,
						"",
						"",
						"system",
						"system",
						"system",
					))
				}
			}
		}
	}
	d.router = router
	d.localTransports = localTransports
	d.sshTransports = sshTransports

	mioTransport := d.ensureLocalTransport("mio")
	shiroTransport := d.ensureLocalTransport("shiro")
	d.startLocalWorkerAgent("shiro", shiroTransport, shiroAgent, workerExecution)

	if lt, ok := d.localTransports["coder1"]; ok && coder1Adapter != nil {
		d.startLocalCoderAgent("coder1", lt, coder1Adapter)
	}
	if lt, ok := d.localTransports["coder2"]; ok && coder2Adapter != nil {
		d.startLocalCoderAgent("coder2", lt, coder2Adapter)
	}
	if lt, ok := d.localTransports["coder3"]; ok && coder3Adapter != nil {
		d.startLocalCoderAgent("coder3", lt, coder3Adapter)
	}
	if lt, ok := d.localTransports["coder4"]; ok && coder4Adapter != nil {
		d.startLocalCoderAgent("coder4", lt, coder4Adapter)
	}
	_ = mioTransport

	// DistributedOrchestrator（Local + SSH transports）
	distOrch := orchestrator.NewDistributedOrchestrator(
		sessionRepo,
		mioAgent,
		router,
		centralMemory,
		sshTransports,
	)
	d.distOrch = distOrch

	// v4.1: SSH 経由で CoderConfig を送信するための設定
	coderConfigs := make(map[string]interface{})
	if cfg.Coder1.Enabled && distributedAgentAvailable("coder1", localTransports, sshTransports) {
		coderConfigs["coder1"] = cfg.Coder1
	}
	if cfg.Coder2.Enabled && distributedAgentAvailable("coder2", localTransports, sshTransports) {
		coderConfigs["coder2"] = cfg.Coder2
	}
	if cfg.Coder3.Enabled && distributedAgentAvailable("coder3", localTransports, sshTransports) {
		coderConfigs["coder3"] = cfg.Coder3
	}
	if cfg.Coder4.Enabled && distributedAgentAvailable("coder4", localTransports, sshTransports) {
		coderConfigs["coder4"] = cfg.Coder4
	}
	distOrch.SetCoderConfigs(coderConfigs)

	distOrch.SetMaxRepair(cfg.Worker.MaxRepair)
	distOrch.SetDistributedTimeouts(cfg.Distributed.CoderTimeoutSec, cfg.Distributed.CoderRetryMax)
	distOrch.SetTTSBridge(ttsBridge)
	distOrch.SetVTuberBridge(vtuberBridge)
	if d.reportStore != nil {
		distOrch.SetReportStore(d.reportStore)
	}
	if d.eventRelay != nil {
		distOrch.SetEventListener(d.eventRelay)
	}
	d.lineHandler = line.NewHandler(distOrch, cfg.Line.ChannelSecret, cfg.Line.AccessToken)
	if strings.TrimSpace(cfg.Telegram.BotToken) != "" {
		tg := telegramadapter.NewAdapter(cfg.Telegram.BotToken, distOrch)
		tg.SetWebhookSecret(cfg.Telegram.WebhookSecret)
		d.telegramHandler = tg
	}
	if strings.TrimSpace(cfg.Discord.BotToken) != "" {
		dc := discordadapter.NewAdapter(cfg.Discord.BotToken, distOrch)
		dc.SetPublicKeyHex(cfg.Discord.PublicKey)
		d.discordHandler = dc
	}
	if strings.TrimSpace(cfg.Slack.BotToken) != "" {
		d.slackHandler = slackadapter.NewAdapter(cfg.Slack.BotToken, cfg.Slack.SigningSecret, distOrch)
	}

	// IdleChat統合（有効な場合）
	if d.idleChatOrch != nil {
		distOrch.SetIdleNotifier(d.idleChatOrch)
		log.Printf("IdleChat integrated with DistributedOrchestrator")
	}
}

func localAgentEnabled(agentName string, coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter *coderAdapter) bool {
	switch agentName {
	case "mio", "shiro":
		return true
	case "coder1":
		return coder1Adapter != nil
	case "coder2":
		return coder2Adapter != nil
	case "coder3":
		return coder3Adapter != nil
	case "coder4":
		return coder4Adapter != nil
	default:
		return true
	}
}

type sshTransportConnector interface {
	Connect() error
}

func registerSSHTransport(
	agentName string,
	connector sshTransportConnector,
	tr domaintransport.Transport,
	sshTransports map[string]domaintransport.Transport,
) error {
	if err := connector.Connect(); err != nil {
		log.Printf("WARN: SSH transport unavailable for agent '%s': %v", agentName, err)
		return err
	}
	sshTransports[agentName] = tr
	log.Printf("Connected SSHTransport for agent '%s'", agentName)
	return nil
}

func markAgentUnavailable(store *viewer.MonitorStore, agentName, reason string) {
	if store == nil {
		return
	}
	store.SetAgentUnavailable(agentName, reason)
}

func formatAgentUnavailableReason(prefix string, err error) string {
	msg := strings.TrimSpace(prefix)
	if err == nil {
		return msg
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return msg
	}
	if msg == "" {
		return detail
	}
	return msg + ": " + detail
}

func distributedAgentAvailable(
	agentName string,
	localTransports map[string]*transport.LocalTransport,
	sshTransports map[string]domaintransport.Transport,
) bool {
	if _, ok := localTransports[agentName]; ok {
		return true
	}
	if _, ok := sshTransports[agentName]; ok {
		return true
	}
	return false
}

func (d *Dependencies) ensureLocalTransport(agentName string) *transport.LocalTransport {
	if d.localTransports == nil {
		d.localTransports = make(map[string]*transport.LocalTransport)
	}
	if lt, ok := d.localTransports[agentName]; ok {
		return lt
	}
	if d.router != nil {
		if lt, ok := d.router.GetAgent(agentName); ok {
			d.localTransports[agentName] = lt
			return lt
		}
	}
	lt := transport.NewLocalTransport()
	d.router.RegisterAgent(agentName, lt)
	d.localTransports[agentName] = lt
	log.Printf("Registered implicit LocalTransport for agent '%s'", agentName)
	return lt
}

func (d *Dependencies) startLocalWorkerAgent(agentName string, lt *transport.LocalTransport, shiroAgent *agent.ShiroAgent, workerExecution service.WorkerExecutionService) {
	if lt == nil || shiroAgent == nil {
		return
	}
	go func() {
		for {
			msg, err := lt.Receive(context.Background())
			if err != nil {
				log.Printf("Local worker '%s' loop stopped: %v", agentName, err)
				return
			}
			resp := handleLocalWorkerMessage(agentName, msg, shiroAgent, workerExecution)
			d.deliverLocalAgentResponse(resp)
		}
	}()
}

func handleLocalWorkerMessage(agentName string, msg domaintransport.Message, shiroAgent *agent.ShiroAgent, workerExecution service.WorkerExecutionService) domaintransport.Message {
	log.Printf("[LocalWorker] recv agent=%s from=%s to=%s type=%s job=%s content_len=%d has_proposal=%t", agentName, msg.From, msg.To, msg.Type, msg.JobID, len(msg.Content), msg.Proposal != nil)
	if msg.Proposal != nil && workerExecution != nil {
		p := proposal.Reconstruct(msg.Proposal.Plan, msg.Proposal.Patch, msg.Proposal.Risk, msg.Proposal.CostHint)
		jobID, err := task.ParseJobID(msg.JobID)
		if err != nil {
			log.Printf("[LocalWorker] invalid job id agent=%s job=%s err=%v", agentName, msg.JobID, err)
			return newLocalAgentError(agentName, msg, fmt.Sprintf("invalid job ID: %v", err))
		}
		log.Printf("[LocalWorker] proposal execute start agent=%s job=%s", agentName, msg.JobID)
		result, err := workerExecution.ExecuteProposal(context.Background(), jobID, p)
		if err != nil {
			log.Printf("[LocalWorker] proposal execute error agent=%s job=%s err=%v", agentName, msg.JobID, err)
			return newLocalAgentError(agentName, msg, fmt.Sprintf("patch execution failed: %v", err))
		}
		resp := domaintransport.NewMessage(agentName, msg.From, msg.SessionID, msg.JobID, result.Summary)
		resp.Type = domaintransport.MessageTypeResult
		resp.Result = &domaintransport.ResultPayload{
			Success:       result.FailedCmds == 0,
			Summary:       result.Summary,
			ExecutedCmds:  result.ExecutedCmds,
			FailedCmds:    result.FailedCmds,
			GitCommit:     result.GitCommit,
			FailureKind:   result.FailureKind,
			FailureReason: result.FailureReason,
			Retryable:     result.Retryable,
			FailedIndex:   result.FailedIndex,
		}
		log.Printf("[LocalWorker] proposal execute complete agent=%s job=%s success=%t summary_len=%d", agentName, msg.JobID, result.FailedCmds == 0, len(result.Summary))
		return resp
	}

	jobID, err := task.ParseJobID(msg.JobID)
	if err != nil {
		jobID = task.NewJobID()
	}
	t := task.NewTask(jobID, msg.Content, "distributed", msg.SessionID)
	log.Printf("[LocalWorker] shiro execute start agent=%s job=%s", agentName, msg.JobID)
	result, err := shiroAgent.Execute(context.Background(), t)
	if err != nil {
		log.Printf("[LocalWorker] shiro execute error agent=%s job=%s err=%v", agentName, msg.JobID, err)
		return newLocalAgentError(agentName, msg, fmt.Sprintf("worker execution failed: %v", err))
	}
	resp := domaintransport.NewMessage(agentName, msg.From, msg.SessionID, msg.JobID, result)
	resp.Type = domaintransport.MessageTypeResult
	resp.Result = &domaintransport.ResultPayload{
		Success: true,
		Summary: result,
	}
	log.Printf("[LocalWorker] shiro execute complete agent=%s job=%s result_len=%d", agentName, msg.JobID, len(result))
	return resp
}

func (d *Dependencies) startLocalCoderAgent(agentName string, lt *transport.LocalTransport, coder *coderAdapter) {
	if lt == nil || coder == nil {
		return
	}
	go func() {
		for {
			msg, err := lt.Receive(context.Background())
			if err != nil {
				log.Printf("Local coder '%s' loop stopped: %v", agentName, err)
				return
			}
			log.Printf("[LocalCoder] recv agent=%s from=%s to=%s type=%s job=%s content_len=%d", agentName, msg.From, msg.To, msg.Type, msg.JobID, len(msg.Content))
			d.emitLocalAgentNote(agentName, msg.From, "依頼を受領しました。", msg)
			jobID, parseErr := task.ParseJobID(msg.JobID)
			if parseErr != nil {
				jobID = task.NewJobID()
			}
			t := task.NewTask(jobID, msg.Content, "distributed", msg.SessionID)
			log.Printf("[LocalCoder] proposal start agent=%s job=%s", agentName, msg.JobID)
			d.emitLocalAgentNote(agentName, msg.From, "proposal 生成を開始しました。", msg)
			p, err := coder.GenerateProposal(context.Background(), t)
			if err != nil {
				log.Printf("[LocalCoder] proposal error agent=%s job=%s err=%v", agentName, msg.JobID, err)
				d.emitLocalAgentNote(agentName, msg.From, "proposal 生成で失敗しました。", msg)
				d.deliverLocalAgentResponse(newLocalAgentError(agentName, msg, fmt.Sprintf("proposal generation failed: %v", err)))
				continue
			}
			if p == nil {
				log.Printf("[LocalCoder] proposal empty agent=%s job=%s", agentName, msg.JobID)
				d.emitLocalAgentNote(agentName, msg.From, "proposal が空でした。", msg)
				d.deliverLocalAgentResponse(newLocalAgentError(agentName, msg, "proposal generation returned empty result"))
				continue
			}
			log.Printf("[LocalCoder] proposal complete agent=%s job=%s plan_len=%d patch_len=%d", agentName, msg.JobID, len(p.Plan()), len(p.Patch()))
			d.emitLocalAgentNote(agentName, msg.From, "proposal 生成が完了しました。", msg)
			resp := domaintransport.NewMessage(agentName, localCoderReplyTarget(msg), msg.SessionID, msg.JobID, fmt.Sprintf("Proposal generated by %s", agentName))
			resp.Type = domaintransport.MessageTypeResult
			resp.Proposal = &domaintransport.ProposalPayload{
				Plan:     p.Plan(),
				Patch:    p.Patch(),
				Risk:     p.Risk(),
				CostHint: p.CostHint(),
			}
			d.deliverLocalAgentResponse(resp)
		}
	}()
}

func (d *Dependencies) deliverLocalAgentResponse(msg domaintransport.Message) {
	if d.router == nil {
		log.Printf("[LocalDeliver] drop reason=no_router to=%s from=%s job=%s", msg.To, msg.From, msg.JobID)
		return
	}
	target, ok := d.router.GetAgent(msg.To)
	if !ok {
		log.Printf("Local agent response dropped: target '%s' not registered", msg.To)
		return
	}
	log.Printf("[LocalDeliver] send to=%s from=%s type=%s job=%s content_len=%d has_proposal=%t", msg.To, msg.From, msg.Type, msg.JobID, len(msg.Content), msg.Proposal != nil)
	if err := target.PutInboundMessage(msg); err != nil {
		log.Printf("Local agent response delivery failed to '%s': %v", msg.To, err)
		return
	}
	log.Printf("[LocalDeliver] sent to=%s from=%s job=%s", msg.To, msg.From, msg.JobID)
}

func newLocalAgentError(agentName string, msg domaintransport.Message, errMsg string) domaintransport.Message {
	resp := domaintransport.NewMessage(agentName, localCoderReplyTarget(msg), msg.SessionID, msg.JobID, errMsg)
	resp.Type = domaintransport.MessageTypeError
	return resp
}

func localCoderReplyTarget(msg domaintransport.Message) string {
	if strings.EqualFold(strings.TrimSpace(msg.From), "shiro") {
		return "mio"
	}
	return msg.From
}

func (d *Dependencies) handleIdleChatStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if err := d.idleChatOrch.StartManualMode(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) emitLocalAgentNote(from, to, content string, msg domaintransport.Message) {
	if d.eventRelay == nil {
		return
	}
	route := ""
	if msg.Context != nil {
		if v, ok := msg.Context["route"].(string); ok {
			route = v
		}
	}
	d.eventRelay.OnEvent(orchestrator.NewEvent(
		"agent.note",
		from,
		to,
		content,
		route,
		msg.JobID,
		msg.SessionID,
		"distributed",
		msg.SessionID,
	))
}

func (d *Dependencies) handleIdleChatStop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		d.idleChatOrch.StopManualMode()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatForecast() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if err := d.idleChatOrch.StartForecastMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunForecastSession()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if err := d.idleChatOrch.StartStoryMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunSimpleStorySession()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStorySimple() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if err := d.idleChatOrch.StartSimpleStoryMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunSimpleStorySession()
		writeJSON(w, map[string]any{
			"ok":          true,
			"mode":        d.idleChatOrch.CurrentMode(),
			"chat_active": d.idleChatOrch.IsChatActive(),
		})
	}
}

func (d *Dependencies) handleIdleChatLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		limit := 20
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
			"history":       d.idleChatOrch.GetHistory(limit),
		})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
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
func resolveSubagentProvider(cfg *config.Config, fallback llm.ToolCallingProvider) llm.ToolCallingProvider {
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

// setupCoders は Config から Coder1-4 を初期化（v4.1 Agent Persona 対応）
func setupCoders(cfg *config.Config) (coder1, coder2, coder3, coder4 *coderAdapter) {
	// Shared LightMemory instances (セッション単位で共有)
	var globalLightMemory *agent.LightMemory

	coderConfigs := []struct {
		name   string
		config config.CoderConfig
		out    **coderAdapter
	}{
		{"coder1", cfg.Coder1, &coder1},
		{"coder2", cfg.Coder2, &coder2},
		{"coder3", cfg.Coder3, &coder3},
		{"coder4", cfg.Coder4, &coder4},
	}

	for _, cc := range coderConfigs {
		if !cc.config.Enabled {
			log.Printf("[setupCoders] %s (%s) disabled", cc.name, cc.config.Name)
			continue
		}

		// LLM Provider 生成
		provider, err := infrallm.CreateProvider(cc.config)
		if err != nil {
			log.Printf("[setupCoders] %s (%s) provider creation failed: %v", cc.name, cc.config.Name, err)
			continue
		}
		if provider == nil {
			log.Printf("[setupCoders] %s (%s) provider is nil (Enabled=false or error)", cc.name, cc.config.Name)
			continue
		}

		// CoderAgent 作成
		domainCoder := agent.NewCoderAgent(provider, nil, nil, cfg.Prompts.CoderProposal)

		// Agent Persona 設定（persona_file 優先、なければ personality インライン）
		personality := cc.config.Personality
		if cc.config.PersonaFile != "" {
			if content, ok := config.LoadPersonaFile(cfg.WorkspaceDir, cc.config.PersonaFile); ok {
				personality = content
				log.Printf("[setupCoders] %s (%s) persona loaded from file: %s", cc.name, cc.config.DisplayName, cc.config.PersonaFile)
			}
		}
		if personality != "" {
			coderPersona := agent.AgentPersona{
				Name:        cc.config.Name,
				Personality: personality,
				Tone:        cc.config.Tone,
			}
			domainCoder.WithPersona(coderPersona)
			log.Printf("[setupCoders] %s (%s) persona enabled", cc.name, cc.config.DisplayName)
		}

		// LightMemory 設定（全 Coder で共有）
		if cc.config.LightMemory.Enabled {
			if globalLightMemory == nil {
				maxTurns := cc.config.LightMemory.MaxTurns
				if maxTurns <= 0 {
					maxTurns = 3
				}
				globalLightMemory = agent.NewLightMemory(maxTurns)
				log.Printf("[setupCoders] LightMemory initialized with maxTurns=%d", maxTurns)
			}
			domainCoder.WithLightMemory(globalLightMemory)
			log.Printf("[setupCoders] %s (%s) LightMemory enabled", cc.name, cc.config.DisplayName)
		}

		// coderAdapter 作成
		*cc.out = &coderAdapter{domainCoder: domainCoder}
		log.Printf("[setupCoders] %s (%s) enabled: provider=%s model=%s",
			cc.name, cc.config.DisplayName, cc.config.Provider, cc.config.Model)
	}

	return
}
