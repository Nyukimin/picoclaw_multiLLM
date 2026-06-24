package dci

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSearchTraceRejectsMalformedTrace(t *testing.T) {
	now := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	valid := func() SearchTrace {
		return SearchTrace{
			EventID:            "evt_dci_1",
			StartedAt:          now,
			EndedAt:            now.Add(time.Second),
			Actor:              "Worker",
			Mode:               "dci",
			UserQuery:          "DCI",
			Status:             "completed",
			FinalEvidenceCount: 1,
			Steps: []SearchStep{{
				StepNo:      1,
				Tool:        "file_read",
				ResultCount: 1,
				Status:      "completed",
				CreatedAt:   now,
			}},
		}
	}
	tests := []struct {
		name   string
		mutate func(*SearchTrace)
		want   string
	}{
		{name: "missing started_at", mutate: func(trace *SearchTrace) {
			trace.StartedAt = time.Time{}
		}, want: "started_at"},
		{name: "terminal missing ended_at", mutate: func(trace *SearchTrace) {
			trace.EndedAt = time.Time{}
		}, want: "ended_at"},
		{name: "failed missing error", mutate: func(trace *SearchTrace) {
			trace.Status = "failed"
			trace.ErrorMessage = ""
		}, want: "error_message"},
		{name: "negative evidence count", mutate: func(trace *SearchTrace) {
			trace.FinalEvidenceCount = -1
		}, want: "final_evidence_count"},
		{name: "duplicate step", mutate: func(trace *SearchTrace) {
			trace.Steps = append(trace.Steps, trace.Steps[0])
		}, want: "duplicate step_no"},
		{name: "step missing created_at", mutate: func(trace *SearchTrace) {
			trace.Steps[0].CreatedAt = time.Time{}
		}, want: "created_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trace := valid()
			tt.mutate(&trace)
			err := ValidateSearchTrace(trace)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateSearchTrace() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateSearchTraceAcceptsCompleteTrace(t *testing.T) {
	now := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	err := ValidateSearchTrace(SearchTrace{
		EventID:            "evt_dci_1",
		StartedAt:          now,
		EndedAt:            now.Add(time.Second),
		Actor:              "Worker",
		Mode:               "dci",
		UserQuery:          "DCI",
		Status:             "completed",
		FinalEvidenceCount: 1,
		Steps: []SearchStep{{
			StepNo:      1,
			Tool:        "file_read",
			ResultCount: 1,
			Status:      "ok",
			CreatedAt:   now,
		}},
	})
	if err != nil {
		t.Fatalf("ValidateSearchTrace() error = %v", err)
	}
}
