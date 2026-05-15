package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	healthapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/health"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
	infrahealth "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/health"
	executionpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/execution"
)

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

func buildHealthService(cfg *config.Config) *healthapp.HealthService {
	if cfg.LocalLLM.Enabled && cfg.LocalLLM.Provider == "local_openai" {
		checks := buildLocalLLMHealthChecks(cfg)
		return healthapp.NewHealthService(checks...)
	}

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

func buildLocalLLMHealthChecks(cfg *config.Config) []domainhealth.Check {
	if cfg == nil {
		return nil
	}
	seen := map[string]struct{}{}
	add := func(checks []domainhealth.Check, role, baseURL, model string) []domainhealth.Check {
		role = strings.TrimSpace(role)
		baseURL = firstNonEmpty(baseURL, cfg.LocalLLM.BaseURL)
		model = strings.TrimSpace(model)
		if role == "" || baseURL == "" || model == "" {
			return checks
		}
		key := role + "\x00" + baseURL + "\x00" + model
		if _, ok := seen[key]; ok {
			return checks
		}
		seen[key] = struct{}{}
		timeout := localLLMTimeoutForAlias(cfg, role)
		return append(checks, infrahealth.NewOpenAICompatibleChatCheck(role, baseURL, model, cfg.LocalLLM.APIKey, timeout))
	}

	checks := make([]domainhealth.Check, 0, 4)
	checks = add(checks, "Chat", cfg.LocalLLM.ChatBaseURL, cfg.LocalLLM.ChatModel)
	checks = add(checks, "Worker", cfg.LocalLLM.WorkerBaseURL, cfg.LocalLLM.WorkerModel)
	if strings.TrimSpace(cfg.LocalLLM.HeavyBaseURL) != "" {
		checks = add(checks, "Heavy", cfg.LocalLLM.HeavyBaseURL, localLLMModelForAlias(cfg, "heavy"))
	}
	if cfg.LocalLLMWarmupEnabled() {
		checks = add(checks, "Wild", cfg.LocalLLM.WildBaseURL, cfg.LocalLLM.WildModel)
	}
	return checks
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

func inferTTSDebugBaseURLFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.TTS.Irodori.Enabled && strings.TrimSpace(cfg.TTS.Irodori.BaseURL) != "" {
		return strings.TrimSpace(cfg.TTS.Irodori.BaseURL)
	}
	if cfg.TTS.SBV2.Enabled && strings.TrimSpace(cfg.TTS.SBV2.BaseURL) != "" {
		return strings.TrimSpace(cfg.TTS.SBV2.BaseURL)
	}
	return strings.TrimSpace(cfg.TTS.HTTPBaseURL)
}

func inferTTSDebugHealthPathFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.TTS.Irodori.Enabled && strings.TrimSpace(cfg.TTS.Irodori.BaseURL) != "" && strings.Contains(strings.TrimSpace(cfg.TTS.Irodori.BaseURL), ":7860") {
		return "/gradio_api/info"
	}
	return ""
}
