package superagent

import (
	"strings"
	"testing"
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
