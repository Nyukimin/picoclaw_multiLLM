package superagent

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSubagentTaskRequiresScopeAndTermination(t *testing.T) {
	err := ValidateSubagentTask(SubagentTask{
		SubagentID:  "sub_1",
		ParentRunID: "run_1",
		AgentType:   "ResearchAgent",
		Task:        "調査",
		Status:      "pending",
	})
	if err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("expected scope error, got %v", err)
	}
}

func TestValidateContextPackRespectsMaxTokens(t *testing.T) {
	err := ValidateContextPack(ContextPack{
		ContextPackID: "ctx_1",
		RunID:         "run_1",
		Summary:       "summary",
		TokenEstimate: 4000,
	}, 3000)
	if err == nil || !strings.Contains(err.Error(), "max_context_pack_tokens") {
		t.Fatalf("expected token limit error, got %v", err)
	}
}

func TestValidateTraceEventRequiresEventType(t *testing.T) {
	err := ValidateTraceEvent(TraceEvent{EventID: "evt_1", Status: "completed"})
	if err == nil || !strings.Contains(err.Error(), "event_type") {
		t.Fatalf("expected event_type error, got %v", err)
	}
}

func TestValidateSuperAgentRejectsMissingTimestamp(t *testing.T) {
	cases := []struct {
		name string
		err  string
		run  func() error
	}{
		{
			name: "agent run started_at",
			err:  "started_at",
			run: func() error {
				return ValidateAgentRun(AgentRun{RunID: "run_1", AgentType: "LeadAgent", Status: "running"})
			},
		},
		{
			name: "subagent task created_at",
			err:  "created_at",
			run: func() error {
				return ValidateSubagentTask(SubagentTask{
					SubagentID:           "sub_1",
					ParentRunID:          "run_1",
					AgentType:            "ResearchAgent",
					Task:                 "調査",
					Scope:                []string{"docs/"},
					TerminationCondition: "report",
					Status:               "pending",
				})
			},
		},
		{
			name: "context pack created_at",
			err:  "created_at",
			run: func() error {
				return ValidateContextPack(ContextPack{ContextPackID: "ctx_1", RunID: "run_1", Summary: "summary", TokenEstimate: 1200}, 3000)
			},
		},
		{
			name: "message channel created_at",
			err:  "created_at",
			run: func() error {
				return ValidateMessageChannel(MessageChannel{ChannelID: "chan_1", ChannelType: "superagent", Status: "active"})
			},
		},
		{
			name: "trace event created_at",
			err:  "created_at",
			run: func() error {
				return ValidateTraceEvent(TraceEvent{EventID: "evt_1", EventType: "lead_agent_started", Status: "completed"})
			},
		},
		{
			name: "run queue created_at",
			err:  "created_at",
			run: func() error {
				return ValidateRunQueueItem(RunQueueItem{QueueID: "queue_1", Goal: "resume run", Action: "resume", Status: "queued"})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatalf("expected %s error", tc.err)
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("expected error to contain %q, got %v", tc.err, err)
			}
		})
	}
}

func TestValidateSuperAgentRejectsTerminalWithoutCompletedAt(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "agent run",
			run: func() error {
				return ValidateAgentRun(AgentRun{RunID: "run_1", AgentType: "LeadAgent", Status: "failed", StartedAt: now, Summary: "failed"})
			},
		},
		{
			name: "run queue",
			run: func() error {
				return ValidateRunQueueItem(RunQueueItem{QueueID: "queue_1", Goal: "resume run", Action: "resume", Status: "completed", CreatedAt: now})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected completed_at error")
			}
			if !strings.Contains(err.Error(), "completed_at") {
				t.Fatalf("expected completed_at error, got %v", err)
			}
		})
	}
}
