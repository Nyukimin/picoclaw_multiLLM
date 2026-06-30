package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	modulellm "github.com/Nyukimin/picoclaw_multiLLM/modules/llm"
)

const renCrowLLMStartupProbeTimeout = 1500 * time.Millisecond

type renCrowLLMStartupProbe struct {
	Name  string
	URL   string
	Token string
}

type renCrowLLMStartupProbeResult struct {
	Name      string
	URL       string
	Status    int
	OK        bool
	ElapsedMS int64
	Error     string
}

func startRenCrowLLMStartupCheck(cfg *config.Config, llmOpsToken string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := &http.Client{Timeout: renCrowLLMStartupProbeTimeout}
		logRenCrowLLMStartupCheck(ctx, cfg, llmOpsToken, client, log.Printf)
	}()
}

func logRenCrowLLMStartupCheck(ctx context.Context, cfg *config.Config, llmOpsToken string, client *http.Client, printf func(string, ...any)) []renCrowLLMStartupProbeResult {
	if printf == nil {
		printf = log.Printf
	}
	if cfg == nil {
		printf("[RenCrow_LLM][startup] skipped: config=nil")
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: renCrowLLMStartupProbeTimeout}
	}
	localCfg := localRuntimeConfigFromAppConfig(cfg)
	moduleTokenEnv := strings.TrimSpace(cfg.RenCrow.LLM.TokenEnv)
	moduleTokenPresent := false
	moduleToken := ""
	if moduleTokenEnv != "" {
		moduleToken = strings.TrimSpace(os.Getenv(moduleTokenEnv))
		moduleTokenPresent = moduleToken != ""
	}
	if moduleToken == "" {
		moduleToken = strings.TrimSpace(llmOpsToken)
	}
	llmOpsTokenPresent := strings.TrimSpace(llmOpsToken) != ""
	printf("[RenCrow_LLM][startup] config rencrow_enabled=%t rencrow_base=%s llm_ops_enabled=%t llm_ops_base=%s llm_ops_token_present=%t token_env=%s token_env_present=%t local_llm_enabled=%t provider=%s timeout_sec=%d concurrency_global=%d concurrency_model=%d",
		cfg.RenCrow.LLM.Enabled,
		trimURL(cfg.RenCrow.LLM.BaseURL),
		cfg.LLMOps.Enabled,
		trimURL(cfg.LLMOps.BaseURL),
		llmOpsTokenPresent,
		emptyDash(moduleTokenEnv),
		moduleTokenPresent,
		cfg.LocalLLM.Enabled,
		emptyDash(cfg.LocalLLM.Provider),
		cfg.LocalLLM.TimeoutSec,
		cfg.LocalLLM.GlobalConcurrency,
		cfg.LocalLLM.ModelConcurrency,
	)
	logRenCrowLLMRecipients(cfg, printf)
	logRenCrowLLMLocalParameters(localCfg, printf)
	ensureRenCrowLLMOpsStarted(ctx, cfg, client, printf)

	probes := buildRenCrowLLMStartupProbes(cfg, localCfg, moduleToken, llmOpsToken)
	results := runRenCrowLLMStartupProbes(ctx, client, probes)
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	for _, result := range results {
		if result.OK {
			printf("[RenCrow_LLM][startup] probe name=%s ok=true status=%d elapsed_ms=%d url=%s",
				result.Name, result.Status, result.ElapsedMS, result.URL)
			continue
		}
		printf("[RenCrow_LLM][startup] probe name=%s ok=false status=%d elapsed_ms=%d url=%s error=%q",
			result.Name, result.Status, result.ElapsedMS, result.URL, result.Error)
	}
	return results
}

func ensureRenCrowLLMOpsStarted(ctx context.Context, cfg *config.Config, client *http.Client, printf func(string, ...any)) {
	if cfg == nil || !cfg.LLMOps.Enabled || !cfg.LLMOps.AutoStart {
		return
	}
	base := trimURL(cfg.LLMOps.BaseURL)
	if base == "" {
		return
	}
	health := runRenCrowLLMStartupProbe(ctx, client, renCrowLLMStartupProbe{
		Name: "llm_ops.health.before_autostart",
		URL:  base + "/health",
	})
	if health.OK {
		printf("[RenCrow_LLM][startup] autostart skipped: llm_ops already healthy status=%d elapsed_ms=%d", health.Status, health.ElapsedMS)
		return
	}
	pid, logPath, err := launchRenCrowLLMOps(cfg.LLMOps)
	if err != nil {
		printf("[RenCrow_LLM][startup] autostart failed: %v", err)
		return
	}
	printf("[RenCrow_LLM][startup] autostart launched pid=%d log=%s reason=%q", pid, logPath, health.Error)

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return
		}
		probe := runRenCrowLLMStartupProbe(ctx, client, renCrowLLMStartupProbe{
			Name: "llm_ops.health.after_autostart",
			URL:  base + "/health",
		})
		if probe.OK {
			printf("[RenCrow_LLM][startup] autostart ready status=%d elapsed_ms=%d", probe.Status, probe.ElapsedMS)
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	printf("[RenCrow_LLM][startup] autostart launched but health did not become ready within timeout")
}

func launchRenCrowLLMOps(cfg config.LLMOpsConfig) (int, string, error) {
	root := strings.TrimSpace(cfg.Root)
	if root == "" {
		return 0, "", fmt.Errorf("llm_ops.root is empty")
	}
	command := expandRenCrowLLMLaunchCommand(cfg.Command, root)
	if len(command) == 0 || strings.TrimSpace(command[0]) == "" {
		return 0, "", fmt.Errorf("llm_ops.command is empty")
	}
	logPath := expandRenCrowLLMLaunchValue(cfg.LogPath, root)
	if strings.TrimSpace(logPath) == "" {
		logPath = filepath.Join(root, "run", "mlx-mgmt-rio.log")
	}
	if !filepath.IsAbs(logPath) {
		logPath = filepath.Join(root, logPath)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return 0, logPath, fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, logPath, fmt.Errorf("open log: %w", err)
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = root
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	for key, value := range cfg.Env {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		cmd.Env = append(cmd.Env, key+"="+expandRenCrowLLMLaunchValue(value, root))
	}
	prepareBackgroundCommand(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return 0, logPath, fmt.Errorf("start %s: %w", strings.Join(command, " "), err)
	}
	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
	}()
	return cmd.Process.Pid, logPath, nil
}

func expandRenCrowLLMLaunchCommand(command []string, root string) []string {
	out := make([]string, 0, len(command))
	for _, part := range command {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, expandRenCrowLLMLaunchValue(part, root))
	}
	return out
}

func expandRenCrowLLMLaunchValue(value string, root string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "{root}", strings.TrimSpace(root))
}

func logRenCrowLLMRecipients(cfg *config.Config, printf func(string, ...any)) {
	keys := make([]string, 0, len(cfg.RenCrow.LLM.Recipients))
	for key := range cfg.RenCrow.LLM.Recipients {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		r := cfg.RenCrow.LLM.Recipients[key]
		printf("[RenCrow_LLM][startup] recipient id=%s role=%s model=%s selection=%s",
			key, emptyDash(r.Role), emptyDash(r.Model), emptyDash(r.Selection))
	}
}

func logRenCrowLLMLocalParameters(localCfg modulellm.LocalRuntimeConfig, printf func(string, ...any)) {
	for _, item := range []struct {
		key   string
		alias string
	}{
		{key: "chat", alias: "Chat"},
		{key: "worker", alias: "Worker"},
		{key: "chatworker", alias: "ChatWorker"},
		{key: "heavy", alias: "Heavy"},
		{key: "wild", alias: "Wild"},
	} {
		printf("[RenCrow_LLM][startup] local_endpoint role=%s base=%s model=%s",
			item.alias,
			trimURL(modulellm.LocalBaseURLForAlias(localCfg, item.key)),
			emptyDash(modulellm.LocalModelForAlias(localCfg, item.key)),
		)
	}
}

func buildRenCrowLLMStartupProbes(cfg *config.Config, localCfg modulellm.LocalRuntimeConfig, moduleToken, llmOpsToken string) []renCrowLLMStartupProbe {
	var probes []renCrowLLMStartupProbe
	rencrowBase := trimURL(cfg.RenCrow.LLM.BaseURL)
	if cfg.RenCrow.LLM.Enabled && rencrowBase != "" {
		if path := strings.TrimSpace(cfg.RenCrow.LLM.Health.LivePath); path != "" {
			probes = append(probes, renCrowLLMStartupProbe{Name: "rencrow.llm.live", URL: rencrowBase + path})
		}
		if path := strings.TrimSpace(cfg.RenCrow.LLM.Health.ReadyPath); path != "" {
			probes = append(probes, renCrowLLMStartupProbe{Name: "rencrow.llm.ready", URL: rencrowBase + path})
		}
		if path := strings.TrimSpace(cfg.RenCrow.LLM.Endpoints.StatusPath); path != "" {
			probes = append(probes, renCrowLLMStartupProbe{Name: "rencrow.llm.status", URL: rencrowBase + path, Token: moduleToken})
		}
	}
	llmOpsBase := trimURL(cfg.LLMOps.BaseURL)
	if cfg.LLMOps.Enabled && llmOpsBase != "" {
		probes = append(probes,
			renCrowLLMStartupProbe{Name: "llm_ops.health", URL: llmOpsBase + "/health"},
			renCrowLLMStartupProbe{Name: "llm_ops.status", URL: llmOpsBase + "/v1/status", Token: strings.TrimSpace(llmOpsToken)},
		)
	}
	for _, item := range []struct {
		key  string
		name string
	}{
		{key: "chat", name: "local.chat.models"},
		{key: "worker", name: "local.worker.models"},
		{key: "chatworker", name: "local.chatworker.models"},
		{key: "heavy", name: "local.heavy.models"},
		{key: "wild", name: "local.wild.models"},
	} {
		base := trimURL(modulellm.LocalBaseURLForAlias(localCfg, item.key))
		if cfg.LocalLLM.Enabled && base != "" {
			probes = append(probes, renCrowLLMStartupProbe{Name: item.name, URL: base + "/v1/models"})
		}
	}
	return probes
}

func runRenCrowLLMStartupProbes(ctx context.Context, client *http.Client, probes []renCrowLLMStartupProbe) []renCrowLLMStartupProbeResult {
	results := make([]renCrowLLMStartupProbeResult, len(probes))
	var wg sync.WaitGroup
	for i, probe := range probes {
		wg.Add(1)
		go func(i int, probe renCrowLLMStartupProbe) {
			defer wg.Done()
			results[i] = runRenCrowLLMStartupProbe(ctx, client, probe)
		}(i, probe)
	}
	wg.Wait()
	return results
}

func runRenCrowLLMStartupProbe(ctx context.Context, client *http.Client, probe renCrowLLMStartupProbe) renCrowLLMStartupProbeResult {
	started := time.Now()
	result := renCrowLLMStartupProbeResult{Name: probe.Name, URL: probe.URL}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if token := strings.TrimSpace(probe.Token); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	result.ElapsedMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	result.Status = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if !result.OK {
		result.Error = fmt.Sprintf("http_status=%d", resp.StatusCode)
	}
	return result
}

func trimURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}
