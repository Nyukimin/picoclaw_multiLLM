package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	domainai "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/aiworkflow"
)

type runtimeContextBudgetRecorderStub struct {
	usages []domainai.ContextUsage
	events []domainai.WorkflowEvent
}

func (s *runtimeContextBudgetRecorderStub) SaveContextUsage(_ context.Context, item domainai.ContextUsage) error {
	s.usages = append(s.usages, item)
	return nil
}

func (s *runtimeContextBudgetRecorderStub) SaveWorkflowEvent(_ context.Context, item domainai.WorkflowEvent) error {
	s.events = append(s.events, item)
	return nil
}

func TestBuildToolMediationRecorderUsesConfiguredLogPath(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "tool_mediation.jsonl")
	cfg := &config.Config{
		WorkspaceDir: t.TempDir(),
		ToolHarness: config.ToolHarnessConfig{
			LogPath: logPath,
		},
	}

	recorder := buildToolMediationRecorder(cfg)
	if recorder == nil {
		t.Fatal("expected recorder")
	}
	if _, err := os.Stat(logPath); err != nil {
		t.Fatalf("expected recorder file at configured path: %v", err)
	}
}

func TestBuildToolMediationRecorderDisabledByConfig(t *testing.T) {
	enabled := false
	cfg := &config.Config{
		WorkspaceDir: t.TempDir(),
		ToolHarness: config.ToolHarnessConfig{
			Enabled: &enabled,
		},
	}

	if recorder := buildToolMediationRecorder(cfg); recorder != nil {
		t.Fatal("disabled tool harness should not create recorder")
	}
}

func TestBuildToolMediationRecorderRecordEventsDisabled(t *testing.T) {
	recordEvents := false
	cfg := &config.Config{
		WorkspaceDir: t.TempDir(),
		ToolHarness: config.ToolHarnessConfig{
			RecordEvents: &recordEvents,
		},
	}

	if recorder := buildToolMediationRecorder(cfg); recorder != nil {
		t.Fatal("record_events=false should not create recorder")
	}
}

func TestBuildToolRuntimeWrapsToolContextBudget(t *testing.T) {
	recordEvents := false
	workspace := t.TempDir()
	path := filepath.Join(workspace, "large.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 400)), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &config.Config{
		WorkspaceDir: workspace,
		ToolHarness: config.ToolHarnessConfig{
			RecordEvents: &recordEvents,
		},
		AIWorkflow: config.AIWorkflowConfig{
			ContextBudgetTokens:    50,
			ContextBudgetWarnRatio: 0.8,
			ContextBudgetStopRatio: 0.95,
		},
	}

	runtime := buildToolRuntime(cfg, nil, nil, nil)
	resp, err := runtime.WorkerRuntimeRunnerV2.ExecuteV2(context.Background(), "file_read", map[string]any{"path": path})
	if err != nil {
		t.Fatalf("ExecuteV2 returned err: %v", err)
	}
	if resp == nil || !resp.IsError() {
		t.Fatalf("expected context budget error response, got %#v", resp)
	}
	if resp.Error.Details["context_budget_status"] != domainai.ContextBudgetStatusStop {
		t.Fatalf("expected stop metadata, got %#v", resp.Error.Details)
	}
}

func TestBuildToolRuntimeRecordsToolContextBudgetUsage(t *testing.T) {
	recordEvents := false
	workspace := t.TempDir()
	path := filepath.Join(workspace, "large.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 340)), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	cfg := &config.Config{
		WorkspaceDir: workspace,
		ToolHarness: config.ToolHarnessConfig{
			RecordEvents: &recordEvents,
		},
		AIWorkflow: config.AIWorkflowConfig{
			ContextBudgetTokens:    100,
			ContextBudgetWarnRatio: 0.8,
			ContextBudgetStopRatio: 0.95,
		},
	}
	recorder := &runtimeContextBudgetRecorderStub{}

	runtime := buildToolRuntime(cfg, nil, nil, recorder)
	resp, err := runtime.WorkerRuntimeRunnerV2.ExecuteV2(context.Background(), "file_read", map[string]any{"path": path})
	if err != nil {
		t.Fatalf("ExecuteV2 returned err: %v", err)
	}
	if resp == nil || resp.IsError() {
		t.Fatalf("expected warning success response, got %#v", resp)
	}
	if len(recorder.usages) != 1 {
		t.Fatalf("expected one usage record, got %#v", recorder.usages)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("expected one workflow event, got %#v", recorder.events)
	}
	if recorder.events[0].EventType != "context_budget_warning" {
		t.Fatalf("expected context budget warning event, got %#v", recorder.events[0])
	}
	if recorder.events[0].ParentEventID != recorder.usages[0].EventID {
		t.Fatalf("event should link to usage: event=%#v usage=%#v", recorder.events[0], recorder.usages[0])
	}
}
