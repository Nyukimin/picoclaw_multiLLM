package main

import (
	"context"
	"strings"
	"testing"

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
