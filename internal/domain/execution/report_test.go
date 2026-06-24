package execution

import (
	"testing"
	"time"
)

func TestExecutionReportValidate(t *testing.T) {
	r := ExecutionReport{
		JobID:      "j1",
		Goal:       "TTS実装して",
		Status:     "passed",
		CreatedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid report, got %v", err)
	}

	r.JobID = ""
	if err := r.Validate(); err == nil {
		t.Fatal("expected validation error for empty job id")
	}
}
