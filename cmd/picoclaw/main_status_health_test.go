package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
)

func TestCollectOllamaHealthRequirements_IncludesWorkerModel(t *testing.T) {
	cfg := &config.Config{
		Ollama: config.OllamaConfig{
			BaseURL:     "http://127.0.0.1:11434",
			Model:       "chat-v1:latest",
			WorkerModel: "worker-v1:latest",
			MaxContext:  4096,
		},
	}

	got := collectOllamaHealthRequirements(cfg)
	if len(got) != 2 {
		t.Fatalf("expected 2 requirements, got %d: %#v", len(got), got)
	}
	if got[0].Name != "chat-v1:latest" || got[1].Name != "worker-v1:latest" {
		t.Fatalf("unexpected requirements: %#v", got)
	}
	if got[0].MaxContext != 4096 || got[1].MaxContext != 4096 {
		t.Fatalf("expected max context to propagate, got %#v", got)
	}
}

func TestCollectOllamaHealthRequirements_DeduplicatesModels(t *testing.T) {
	cfg := &config.Config{
		Ollama: config.OllamaConfig{
			Model:       "chat-v1:latest",
			WorkerModel: "chat-v1:latest",
			MaxContext:  4096,
		},
	}

	got := collectOllamaHealthRequirements(cfg)
	if len(got) != 1 {
		t.Fatalf("expected deduplicated requirements, got %#v", got)
	}
}

type fakeHealthChecker struct {
	report domainhealth.HealthReport
}

func (f *fakeHealthChecker) RunChecks(_ context.Context) domainhealth.HealthReport {
	return f.report
}

func TestRunHealthCommand_JSONContract(t *testing.T) {
	checker := &fakeHealthChecker{
		report: domainhealth.HealthReport{
			Status: domainhealth.StatusOK,
			Checks: []domainhealth.CheckResult{
				{Name: "ollama", Status: domainhealth.StatusOK, Message: "ok", Duration: 5 * time.Millisecond},
			},
		},
	}
	var out, errOut bytes.Buffer
	code := runHealthCommand([]string{"--json"}, checker, &out, &errOut, fixedNow)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	var payload struct {
		OK        bool   `json:"ok"`
		Component string `json:"component"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !payload.OK || payload.Component != "health" || payload.Status != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestRunStatusCommand_DeepUsageJSON(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18790},
		Ollama: config.OllamaConfig{BaseURL: "http://127.0.0.1:11434", Model: "qwen3:8b"},
	}
	checker := &fakeHealthChecker{report: domainhealth.HealthReport{
		Status: domainhealth.StatusDegraded,
		Checks: []domainhealth.CheckResult{
			{Name: "ollama", Status: domainhealth.StatusDegraded, Message: "slow", Duration: 50 * time.Millisecond},
		},
	}}
	statsLoader := func(_ *config.Config) (map[domainexecution.Status]int, error) {
		return map[domainexecution.Status]int{
			domainexecution.StatusRunning:         2,
			domainexecution.StatusWaitingApproval: 1,
			domainexecution.StatusDenied:          0,
			domainexecution.StatusFailed:          3,
		}, nil
	}
	usageLoader := func(_ *config.Config) (map[string]map[string]int, error) {
		return map[string]map[string]int{"status": {"passed": 4, "failed": 1}}, nil
	}

	var out, errOut bytes.Buffer
	code := runStatusCommand(
		[]string{"--deep", "--usage", "--json"},
		cfg,
		checker,
		statsLoader,
		usageLoader,
		&out,
		&errOut,
		fixedNow,
	)
	if code != 0 {
		t.Fatalf("expected code 0, got %d (err=%s)", code, errOut.String())
	}
	var payload struct {
		Component string         `json:"component"`
		Status    string         `json:"status"`
		Details   map[string]any `json:"details"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if payload.Component != "status" || payload.Status != "degraded" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if _, ok := payload.Details["execution"]; !ok {
		t.Fatalf("expected execution details: %+v", payload.Details)
	}
	if _, ok := payload.Details["usage"]; !ok {
		t.Fatalf("expected usage details: %+v", payload.Details)
	}
}

func TestRunStatusCommand_UsageErrorText(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 18790},
		Ollama: config.OllamaConfig{BaseURL: "http://127.0.0.1:11434", Model: "qwen3:8b"},
	}
	checker := &fakeHealthChecker{report: domainhealth.HealthReport{Status: domainhealth.StatusOK}}
	statsLoader := func(_ *config.Config) (map[domainexecution.Status]int, error) {
		return map[domainexecution.Status]int{}, nil
	}
	usageLoader := func(_ *config.Config) (map[string]map[string]int, error) {
		return nil, errors.New("no evidence")
	}
	var out, errOut bytes.Buffer
	code := runStatusCommand(
		[]string{"--usage"},
		cfg,
		checker,
		statsLoader,
		usageLoader,
		&out,
		&errOut,
		fixedNow,
	)
	if code != 0 {
		t.Fatalf("expected code 0, got %d", code)
	}
	if !strings.Contains(out.String(), "Usage:") || !strings.Contains(out.String(), "unavailable") {
		t.Fatalf("expected usage unavailable output, got: %s", out.String())
	}
}
