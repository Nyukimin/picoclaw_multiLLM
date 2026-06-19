package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	domainrouting "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	domainsuperagent "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/superagent"
)

func TestNewSuperAgentRunQueueProcessorSendsQueueItemToOrchestrator(t *testing.T) {
	processor := &captureSuperAgentRunQueueProcessor{
		response: orchestrator.ProcessMessageResponse{
			Route: domainrouting.RouteCODE,
			JobID: "job-1",
		},
	}
	item := domainsuperagent.RunQueueItem{
		QueueID:      " q-1 ",
		RunID:        "run-1",
		WorkstreamID: "ws-1",
		Goal:         " continue the queued run ",
		Action:       " resume ",
	}

	summary, err := newSuperAgentRunQueueProcessor(processor).ProcessRunQueueItem(context.Background(), item)
	if err != nil {
		t.Fatalf("ProcessRunQueueItem() error = %v", err)
	}
	if summary != "route=CODE job_id=job-1" {
		t.Fatalf("summary = %q, want route=CODE job_id=job-1", summary)
	}
	req := processor.request
	if req.SessionID != "ws-1" || req.Channel != "superagent" || req.ChatID != "q-1" || req.UserMessage != "continue the queued run" {
		t.Fatalf("request = %#v", req)
	}
}

func TestNewSuperAgentRunQueueProcessorRejectsUnsupportedAction(t *testing.T) {
	processor := &captureSuperAgentRunQueueProcessor{}
	_, err := newSuperAgentRunQueueProcessor(processor).ProcessRunQueueItem(context.Background(), domainsuperagent.RunQueueItem{
		QueueID: "q-1",
		Goal:    "run",
		Action:  "external_pr",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported run queue action") {
		t.Fatalf("ProcessRunQueueItem() error = %v, want unsupported action error", err)
	}
	if processor.called {
		t.Fatal("processor was called for unsupported action")
	}
}

func TestNewSuperAgentRunQueueProcessorAllowsExplicitChatAction(t *testing.T) {
	processor := &captureSuperAgentRunQueueProcessor{
		response: orchestrator.ProcessMessageResponse{
			Route: domainrouting.RouteCHAT,
			JobID: "job-chat",
		},
	}
	summary, err := newSuperAgentRunQueueProcessor(processor).ProcessRunQueueItem(context.Background(), domainsuperagent.RunQueueItem{
		QueueID: "q-1",
		Goal:    "run",
		Action:  "chat",
	})
	if err != nil {
		t.Fatalf("ProcessRunQueueItem() error = %v", err)
	}
	if summary != "route=CHAT job_id=job-chat" {
		t.Fatalf("summary = %q, want route=CHAT job_id=job-chat", summary)
	}
}

func TestNewSuperAgentRunQueueProcessorRejectsChatFallbackForResume(t *testing.T) {
	processor := &captureSuperAgentRunQueueProcessor{
		response: orchestrator.ProcessMessageResponse{
			Route: domainrouting.RouteCHAT,
			JobID: "job-chat",
		},
	}
	_, err := newSuperAgentRunQueueProcessor(processor).ProcessRunQueueItem(context.Background(), domainsuperagent.RunQueueItem{
		QueueID: "q-1",
		Goal:    "run",
		Action:  "resume",
	})
	if err == nil || !strings.Contains(err.Error(), "CHAT route") {
		t.Fatalf("ProcessRunQueueItem() error = %v, want CHAT route error", err)
	}
}

func TestNewSuperAgentRunQueueProcessorRejectsMissingJobID(t *testing.T) {
	processor := &captureSuperAgentRunQueueProcessor{
		response: orchestrator.ProcessMessageResponse{
			Route: domainrouting.RouteCODE,
		},
	}
	_, err := newSuperAgentRunQueueProcessor(processor).ProcessRunQueueItem(context.Background(), domainsuperagent.RunQueueItem{
		QueueID: "q-1",
		Goal:    "run",
		Action:  "resume",
	})
	if err == nil || !strings.Contains(err.Error(), "job_id") {
		t.Fatalf("ProcessRunQueueItem() error = %v, want job_id error", err)
	}
}

func TestMemoryLifecycleJobConfigAcceleratesMonthIntoOneHour(t *testing.T) {
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_ACCEL_MONTH_SEC", "3600")
	var wall = time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	cfg := memoryLifecycleJobConfigFromEnv(func() time.Time { return wall })
	if cfg.Interval != 30*time.Second {
		t.Fatalf("interval=%s, want 30s", cfg.Interval)
	}
	wall = wall.Add(time.Hour)
	got := cfg.Now()
	want := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("accelerated now=%s, want %s", got, want)
	}
}

func TestMemoryLifecycleJobConfigCanRunMonthAsFastAsOneSecond(t *testing.T) {
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_ACCEL_MONTH_SEC", "1")
	var wall = time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	cfg := memoryLifecycleJobConfigFromEnv(func() time.Time { return wall })
	if cfg.Interval != time.Second {
		t.Fatalf("interval=%s, want 1s", cfg.Interval)
	}
	wall = wall.Add(time.Second)
	got := cfg.Now()
	want := time.Date(2026, 7, 19, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("accelerated now=%s, want %s", got, want)
	}
}

func TestMemoryLifecycleJobConfigAllowsExplicitAcceleratedInterval(t *testing.T) {
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_ACCEL_MONTH_SEC", "3600")
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_INTERVAL_SEC", "5")
	cfg := memoryLifecycleJobConfigFromEnv(time.Now)
	if cfg.Interval != 5*time.Second {
		t.Fatalf("interval=%s, want 5s", cfg.Interval)
	}
}

func TestMemoryLifecycleJobConfigIgnoresInvalidAcceleration(t *testing.T) {
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_ACCEL_MONTH_SEC", "bad")
	t.Setenv("RENCROW_MEMORY_LIFECYCLE_INTERVAL_SEC", "bad")
	cfg := memoryLifecycleJobConfigFromEnv(time.Now)
	if cfg.Interval != 24*time.Hour {
		t.Fatalf("interval=%s, want normal 24h", cfg.Interval)
	}
}

func TestMemoryLifecycleJobConfigDefaultEnvIsNormal(t *testing.T) {
	_ = os.Unsetenv("RENCROW_MEMORY_LIFECYCLE_ACCEL_MONTH_SEC")
	_ = os.Unsetenv("RENCROW_MEMORY_LIFECYCLE_INTERVAL_SEC")
	cfg := memoryLifecycleJobConfigFromEnv(time.Now)
	if cfg.Interval != 24*time.Hour || cfg.Label != "normal" {
		t.Fatalf("unexpected default cfg: %+v", cfg)
	}
}

type captureSuperAgentRunQueueProcessor struct {
	called   bool
	request  orchestrator.ProcessMessageRequest
	response orchestrator.ProcessMessageResponse
}

func (p *captureSuperAgentRunQueueProcessor) ProcessMessage(_ context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error) {
	p.called = true
	p.request = req
	return p.response, nil
}
