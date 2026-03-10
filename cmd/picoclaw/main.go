package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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
	subagentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/subagent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/toolloop"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintool "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
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
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persona"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
	securityinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/security"
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
	case "version":
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
	mux.HandleFunc("/viewer", viewer.HandlePage)
	mux.HandleFunc("/viewer/events", dependencies.eventHub.HandleSSE)
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
					"running":          stats[domainexecution.StatusRunning],
					"waiting_approval": stats[domainexecution.StatusWaitingApproval],
					"denied":           stats[domainexecution.StatusDenied],
					"failed":           stats[domainexecution.StatusFailed],
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
	fmt.Fprintf(out, "PicoClaw %s\n", Version)
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
		fmt.Fprintf(out, "  waiting_approval: %d\n", stats[domainexecution.StatusWaitingApproval])
		fmt.Fprintf(out, "  denied: %d\n", stats[domainexecution.StatusDenied])
		fmt.Fprintf(out, "  failed: %d\n", stats[domainexecution.StatusFailed])
	} else {
		fmt.Fprintf(out, "\nExecution: unavailable (%v)\n", statsErr)
	}

	if deep {
		fmt.Fprintln(out, "\nDetails:")
		fmt.Fprintf(out, "  timestamp: %s\n", now().Format(time.RFC3339))
		fmt.Fprintf(out, "  security.enabled: %t\n", cfg.Security.Enabled)
		fmt.Fprintf(out, "  security.approval_mode: %s\n", cfg.Security.ApprovalMode)
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
		if cfg.Security.ApprovalMode != "never" && !lineConfigured {
			findings = append(findings, doctorFinding{
				Level: "WARN",
				Msg:   "security approval is enabled, but LINE webhook credentials are not fully configured",
				Hint:  "set line.channel_secret and line.access_token, or switch security.approval_mode=never",
			})
		}
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

func gatewayHealthURL(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Server.Host)
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d/health", host, cfg.Server.Port)
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
	fmt.Printf(`PicoClaw %s - Multi-LLM AI Assistant

Usage: picoclaw [command]

Commands:
  run       Start the HTTP server (default)
  version   Show version information
  health    Run health checks and output JSON
  status    Show system status overview
  doctor    Diagnose config and runtime prerequisites
  channels  List/probe channel adapters
  gateway   Gateway status/restart operations
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
	lineHandler        http.Handler
	telegramHandler    http.Handler
	discordHandler     http.Handler
	slackHandler       http.Handler
	eventHub           *viewer.EventHub                      // live viewer
	eventRelay         *idleAwareEventListener               // viewer + idlechat stop relay
	viewerSend         http.HandlerFunc                      // viewer message sender
	evidenceHandler    http.HandlerFunc                      // viewer evidence API
	evidenceDetail     http.HandlerFunc                      // viewer evidence detail API
	evidenceSummary    http.HandlerFunc                      // viewer evidence summary API
	entryHandler       http.HandlerFunc                      // unified entry endpoint
	chromeBridge       http.HandlerFunc                      // chrome bridge endpoint
	chromeBridgeStatus http.HandlerFunc                      // chrome bridge status endpoint
	chromeBridgeEvents http.HandlerFunc                      // chrome bridge SSE endpoint
	distOrch           *orchestrator.DistributedOrchestrator // v4 distributed orchestrator
	router             *transport.MessageRouter              // v4 distributed mode
	idleChatOrch       *idlechat.IdleChatOrchestrator        // v4 idle chat
	sshTransports      map[string]domaintransport.Transport  // v4 SSH transports
	heartbeatSvc       *heartbeat.HeartbeatService           // heartbeat service
}

type idleAwareEventListener struct {
	hub      *viewer.EventHub
	mu       sync.RWMutex
	idleChat *idlechat.IdleChatOrchestrator
}

func (l *idleAwareEventListener) SetIdleChat(idle *idlechat.IdleChatOrchestrator) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.idleChat = idle
}

func (l *idleAwareEventListener) OnEvent(ev orchestrator.OrchestratorEvent) {
	l.hub.OnEvent(ev)
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
	if ev.Type == "message.received" {
		return true
	}
	if ev.From != "" && ev.From != "user" {
		return true
	}
	if ev.To != "" && ev.To != "user" {
		return true
	}
	return false
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
	// 1. LLM Provider (v4: 単一共通モデル) + 日時注入デコレータ
	rawOllamaProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, cfg.Ollama.Model)
	ollamaProvider := infrallm.NewDateTimeProvider(rawOllamaProvider)

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
		AllowedWritePaths: []string{
			filepath.Join(cfg.WorkspaceDir, "CHAT_PERSONA.md"),
		},
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
			ApprovalMode:      cfg.Security.ApprovalMode,
			NetworkScope:      cfg.Security.NetworkScope,
			NetworkAllowed:    cfg.Security.NetworkAllowlist,
			DenyCommands:      cfg.Security.DenyCommands,
			Workspace:         cfg.WorkspaceDir,
			WorkspaceEnforced: cfg.Security.WorkspaceEnforced,
		})
		ttl := time.Duration(cfg.Security.ApprovalTTLMinutes) * time.Minute

		securedChatRunner, err := securityinfra.NewPolicyRunner(chatToolRunnerV2, policy, execRepo, "chat", ttl)
		if err != nil {
			log.Fatalf("Failed to create chat policy runner: %v", err)
		}
		securedWorkerRunner, err := securityinfra.NewPolicyRunner(workerToolRunnerV2, policy, execRepo, "worker", ttl)
		if err != nil {
			log.Fatalf("Failed to create worker policy runner: %v", err)
		}
		chatRunnerV2 = securedChatRunner
		workerRunnerV2 = securedWorkerRunner
		log.Printf("Security policy runner enabled (mode=%s, approval=%s)", cfg.Security.PolicyMode, cfg.Security.ApprovalMode)
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
	personaEditor := persona.NewFilePersonaEditor(cfg.WorkspaceDir)
	mioAgent = mioAgent.WithPersonaEditor(personaEditor)
	log.Printf("Mio: PersonaEditor injected (workspace: %s)", cfg.WorkspaceDir)

	shiroAgent := agent.NewShiroAgent(ollamaProvider, workerToolRunner, mcpClient, cfg.Prompts.Worker, subagentMgr)

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

	// EventHub (Live Viewer)
	hub := viewer.NewEventHub(200)
	deps.eventHub = hub
	deps.eventRelay = &idleAwareEventListener{hub: hub}
	reportPath := defaultExecutionReportPath(cfg.WorkspaceDir)
	ttsRuntime := buildTTSEntryRuntime(cfg)
	if reportStore, err := executionpersistence.NewJSONLReportStore(reportPath); err != nil {
		log.Printf("WARN: evidence API disabled: %v", err)
	} else {
		deps.evidenceHandler = viewer.HandleEvidenceRecent(reportStore)
		deps.evidenceDetail = viewer.HandleEvidenceDetail(reportStore)
		deps.evidenceSummary = viewer.HandleEvidenceSummary(reportStore)
		log.Printf("Viewer evidence API enabled: %s", reportPath)
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
			ollamaProvider,
			centralMemory,
			cfg.IdleChat.Participants,
			cfg.IdleChat.IntervalMin,
			cfg.IdleChat.MaxTurns,
			cfg.IdleChat.Temperature,
			cfg.Prompts.IdleChatAgents,
		)
		topicStorePath := filepath.Join(cfg.Session.StorageDir, "idlechat_topics.jsonl")
		if err := idleChatOrch.SetTopicStore(topicStorePath); err != nil {
			log.Printf("WARN: idleChat topic store disabled: %v", err)
		} else {
			log.Printf("IdleChat topic store enabled: %s", topicStorePath)
		}
		if deps.eventHub != nil {
			idleChatOrch.SetEventEmitter(func(ev idlechat.TimelineEvent) {
				deps.eventHub.OnEvent(orchestrator.NewEvent(
					ev.Type,
					ev.From,
					ev.To,
					ev.Content,
					"IDLECHAT",
					"",
					ev.SessionID,
					"idlechat",
					"idlechat",
				))
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
		deps.buildDistributedMode(cfg, sessionRepo, mioAgent, ollamaProvider, centralMemory)
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
			workerExecutionService,
		)
		orch.SetEventListener(deps.eventRelay)
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
	ollamaProvider llm.LLMProvider,
	centralMemory *domainsession.CentralMemory,
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

	// DistributedOrchestrator（Local + SSH transports）
	distOrch := orchestrator.NewDistributedOrchestrator(
		sessionRepo,
		mioAgent,
		router,
		centralMemory,
		sshTransports,
	)
	d.distOrch = distOrch
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
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
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
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
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
