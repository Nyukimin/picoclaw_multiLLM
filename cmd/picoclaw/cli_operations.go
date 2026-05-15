package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	adapterchannels "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels"
	discordadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/discord"
	slackadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/slack"
	telegramadapter "github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/channels/telegram"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/line"
	knowledgeapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/knowledge"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/sourcefetcher"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	executionpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
)

func cmdVersion() {
	fmt.Printf("picoclaw %s\ncommit: %s\nbuilt:  %s\n", Version, Commit, BuildDate)
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

func buildOutboundChannelRegistry(cfg *config.Config) *adapterchannels.Registry {
	registry := adapterchannels.NewRegistry()
	if strings.TrimSpace(cfg.Line.AccessToken) != "" {
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

func cmdSourceRegistry() {
	configPath := getConfigPath()
	store, err := loadSourceRegistryStore(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize source registry store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	code := runSourceRegistryCommand(os.Args[2:], store, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func cmdKnowledge() {
	configPath := getConfigPath()
	store, err := loadSourceRegistryStore(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize knowledge store: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	code := runKnowledgeCommand(os.Args[2:], store, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

type sourceRegistryCLIStore interface {
	SaveSourceRegistryEntry(ctx context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error)
	ListSourceRegistryEntries(ctx context.Context, enabledOnly bool) ([]conversationpersistence.L1SourceRegistryEntry, error)
}

type knowledgeCLIStore interface {
	knowledgeapp.StagingStore
}

func runKnowledgeCommand(args []string, store knowledgeCLIStore, out io.Writer, errOut io.Writer) int {
	subcmd := ""
	if len(args) > 0 {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch subcmd {
	case "import-core-jsonl":
		jsonOut := hasFlag(args[1:], "--json")
		var inputPath string
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "--") {
				continue
			}
			inputPath = arg
			break
		}
		if strings.TrimSpace(inputPath) == "" {
			fmt.Fprintln(errOut, "usage: picoclaw knowledge import-core-jsonl <path> [--json]")
			return 1
		}
		f, err := os.Open(inputPath)
		if err != nil {
			fmt.Fprintf(errOut, "failed to open knowledge jsonl: %v\n", err)
			return 1
		}
		defer f.Close()
		result, err := knowledgeapp.ImportKnowledgeCoreJSONL(context.Background(), store, f, knowledgeapp.ImportOptions{})
		if err != nil {
			fmt.Fprintf(errOut, "failed to import knowledge jsonl: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"imported": result.Imported}, false)
			return 0
		}
		fmt.Fprintf(out, "imported knowledge core records: %d\n", result.Imported)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown knowledge subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw knowledge import-core-jsonl <path>")
		return 1
	}
}

func runSourceRegistryCommand(args []string, store sourceRegistryCLIStore, out io.Writer, errOut io.Writer) int {
	subcmd := "list"
	if len(args) > 0 {
		subcmd = strings.ToLower(strings.TrimSpace(args[0]))
	}
	switch subcmd {
	case "list":
		jsonOut := hasFlag(args[1:], "--json")
		enabledOnly := hasFlag(args[1:], "--enabled-only")
		entries, err := store.ListSourceRegistryEntries(context.Background(), enabledOnly)
		if err != nil {
			fmt.Fprintf(errOut, "failed to list source registry: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"entries": sourceRegistryCLIEntries(entries)}, false)
			return 0
		}
		if len(entries) == 0 {
			fmt.Fprintln(out, "No source registry entries")
			return 0
		}
		for _, entry := range entries {
			fmt.Fprintf(out, "%s | %s | %.2f | %s | enabled=%v\n", entry.SourceID, entry.Kind, entry.TrustScore, entry.URL, entry.Enabled)
		}
		return 0
	case "save":
		entry, jsonOut, err := parseSourceRegistrySaveArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 1
		}
		saved, err := store.SaveSourceRegistryEntry(context.Background(), entry)
		if err != nil {
			fmt.Fprintf(errOut, "failed to save source registry: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"entry": sourceRegistryCLIEntry(*saved)}, false)
			return 0
		}
		fmt.Fprintf(out, "saved source registry entry: %s\n", saved.SourceID)
		return 0
	case "disable":
		sourceID, jsonOut, err := parseSourceRegistryDisableArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 1
		}
		entries, err := store.ListSourceRegistryEntries(context.Background(), false)
		if err != nil {
			fmt.Fprintf(errOut, "failed to list source registry: %v\n", err)
			return 1
		}
		var target *conversationpersistence.L1SourceRegistryEntry
		for i := range entries {
			if entries[i].SourceID == sourceID {
				target = &entries[i]
				break
			}
		}
		if target == nil {
			fmt.Fprintf(errOut, "source registry entry not found: %s\n", sourceID)
			return 1
		}
		target.Enabled = false
		saved, err := store.SaveSourceRegistryEntry(context.Background(), *target)
		if err != nil {
			fmt.Fprintf(errOut, "failed to disable source registry: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"entry": sourceRegistryCLIEntry(*saved)}, false)
			return 0
		}
		fmt.Fprintf(out, "disabled source registry entry: %s\n", saved.SourceID)
		return 0
	case "sweep":
		registryStore, ok := store.(sourcefetcher.RegistryStore)
		if !ok {
			fmt.Fprintln(errOut, "source registry store does not support sweep")
			return 1
		}
		opts, jsonOut, err := parseSourceRegistrySweepArgs(args[1:])
		if err != nil {
			fmt.Fprintf(errOut, "%v\n", err)
			return 1
		}
		result, err := sourcefetcher.SweepDueSources(context.Background(), registryStore, time.Now().UTC(), opts)
		if err != nil {
			fmt.Fprintf(errOut, "failed to sweep source registry: %v\n", err)
			return 1
		}
		if jsonOut {
			writeJSONCLI(out, map[string]any{"result": sourceRegistrySweepResultCLI(result)}, false)
			return 0
		}
		fmt.Fprintf(out, "sweep complete: sources=%d staged=%d validated=%d promoted_news=%d failed=%d\n",
			result.Sources, result.Staged, result.Validated, result.PromotedNews, result.Failed)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown source-registry subcommand: %s\n", subcmd)
		fmt.Fprintln(errOut, "usage: picoclaw source-registry [list|save|disable|sweep]")
		return 1
	}
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

func parseSourceRegistrySaveArgs(args []string) (conversationpersistence.L1SourceRegistryEntry, bool, error) {
	values := map[string]string{}
	jsonOut := false
	enabled := true
	for i := 0; i < len(args); i++ {
		key := strings.TrimSpace(args[i])
		switch key {
		case "--json":
			jsonOut = true
		case "--disabled":
			enabled = false
		case "--source-id", "--url", "--kind", "--trust-score", "--interval-sec", "--license-note", "--namespace":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return conversationpersistence.L1SourceRegistryEntry{}, jsonOut, fmt.Errorf("%s requires a value", key)
			}
			values[key] = strings.TrimSpace(args[i+1])
			i++
		default:
			return conversationpersistence.L1SourceRegistryEntry{}, jsonOut, fmt.Errorf("unknown source-registry save option: %s", key)
		}
	}
	sourceID := values["--source-id"]
	sourceURL := values["--url"]
	kind := values["--kind"]
	licenseNote := values["--license-note"]
	if sourceID == "" || sourceURL == "" || kind == "" || licenseNote == "" {
		return conversationpersistence.L1SourceRegistryEntry{}, jsonOut, errors.New("source-id, url, kind, license-note are required")
	}
	trustScore := 0.5
	if raw := values["--trust-score"]; raw != "" {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return conversationpersistence.L1SourceRegistryEntry{}, jsonOut, fmt.Errorf("invalid --trust-score: %s", raw)
		}
		trustScore = parsed
	}
	interval := time.Hour
	if raw := values["--interval-sec"]; raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return conversationpersistence.L1SourceRegistryEntry{}, jsonOut, fmt.Errorf("invalid --interval-sec: %s", raw)
		}
		interval = time.Duration(parsed) * time.Second
	}
	meta := map[string]interface{}{}
	if namespace := values["--namespace"]; namespace != "" {
		meta["namespace"] = namespace
	}
	return conversationpersistence.L1SourceRegistryEntry{
		SourceID:      sourceID,
		URL:           sourceURL,
		Kind:          kind,
		TrustScore:    trustScore,
		FetchInterval: interval,
		LicenseNote:   licenseNote,
		Enabled:       enabled,
		Meta:          meta,
	}, jsonOut, nil
}

func parseSourceRegistryDisableArgs(args []string) (string, bool, error) {
	sourceID := ""
	jsonOut := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--source-id":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", jsonOut, errors.New("--source-id requires a value")
			}
			sourceID = strings.TrimSpace(args[i+1])
			i++
		default:
			if strings.HasPrefix(arg, "--") {
				return "", jsonOut, fmt.Errorf("unknown source-registry disable option: %s", arg)
			}
			if sourceID == "" {
				sourceID = arg
			}
		}
	}
	if sourceID == "" {
		return "", jsonOut, errors.New("source-id is required")
	}
	return sourceID, jsonOut, nil
}

func parseSourceRegistrySweepArgs(args []string) (sourcefetcher.SweepOptions, bool, error) {
	opts := sourcefetcher.SweepOptions{LimitPerSource: 10, MinimumTrustScore: 0.5}
	jsonOut := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch arg {
		case "--json":
			jsonOut = true
		case "--limit":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return opts, jsonOut, errors.New("--limit requires a value")
			}
			n, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil || n <= 0 {
				return opts, jsonOut, fmt.Errorf("invalid --limit: %s", args[i+1])
			}
			opts.LimitPerSource = n
			i++
		case "--min-trust":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return opts, jsonOut, errors.New("--min-trust requires a value")
			}
			n, err := strconv.ParseFloat(strings.TrimSpace(args[i+1]), 64)
			if err != nil || n < 0 || n > 1 {
				return opts, jsonOut, fmt.Errorf("invalid --min-trust: %s", args[i+1])
			}
			opts.MinimumTrustScore = n
			i++
		default:
			return opts, jsonOut, fmt.Errorf("unknown source-registry sweep option: %s", arg)
		}
	}
	return opts, jsonOut, nil
}

func sourceRegistrySweepResultCLI(result sourcefetcher.SweepResult) map[string]any {
	return map[string]any{
		"sources":            result.Sources,
		"staged":             result.Staged,
		"validated":          result.Validated,
		"promoted_news":      result.PromotedNews,
		"promoted_knowledge": result.PromotedKnowledge,
		"failed":             result.Failed,
	}
}

func sourceRegistryCLIEntries(entries []conversationpersistence.L1SourceRegistryEntry) []map[string]any {
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		out = append(out, sourceRegistryCLIEntry(entry))
	}
	return out
}

func sourceRegistryCLIEntry(entry conversationpersistence.L1SourceRegistryEntry) map[string]any {
	return map[string]any{
		"source_id":          entry.SourceID,
		"url":                entry.URL,
		"kind":               entry.Kind,
		"trust_score":        entry.TrustScore,
		"fetch_interval_sec": int64(entry.FetchInterval.Seconds()),
		"license_note":       entry.LicenseNote,
		"enabled":            entry.Enabled,
		"meta":               entry.Meta,
	}
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

func loadSourceRegistryStore(configPath string) (*conversationpersistence.L1SQLiteStore, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	p := strings.TrimSpace(cfg.Conversation.L1SQLitePath)
	if p == "" {
		return nil, errors.New("conversation.l1_sqlite_path is required for source-registry CLI")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, err
	}
	return conversationpersistence.NewL1SQLiteStore(p)
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
  source-registry  List/register L1 source registry entries
  knowledge  Import Knowledge DB seed data
  help      Show this help message

Agent Mode:
  Use picoclaw-agent binary for distributed execution.
  See install-agent.sh or install-agent.ps1 for setup.
`, Version)
}

// buildHealthService は HealthService を構築（CLI コマンドで共用）
