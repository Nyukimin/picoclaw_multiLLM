package verification

import (
	"testing"
	"time"
)

func TestVerificationStatusValid(t *testing.T) {
	valid := []VerificationStatus{
		StatusVerified,
		StatusWeaklySupported,
		StatusUnsupported,
		StatusConflict,
		StatusNotChecked,
	}
	for _, status := range valid {
		if !status.Valid() {
			t.Fatalf("expected status %s to be valid", status)
		}
	}
	if VerificationStatus("success").Valid() {
		t.Fatal("success must not be a verification status")
	}
}

func TestClaimValidateRejectsEmptyText(t *testing.T) {
	claim := Claim{ID: "claim-1", Priority: TriggerHigh, Status: StatusNotChecked}
	if err := claim.Validate(); err == nil {
		t.Fatal("expected empty claim text to be rejected")
	}
}

func TestEvidenceRefRejectsInvalidSource(t *testing.T) {
	evidence := EvidenceRef{ID: "ev-1", SourceType: EvidenceSourceType("raw_log"), Supports: true}
	if err := evidence.Validate(); err == nil {
		t.Fatal("raw_log must not be accepted as evidence source")
	}
}

func TestVerificationReportValidate(t *testing.T) {
	report := VerificationReport{
		ID:           "verify-job-1",
		JobID:        "job-1",
		SessionID:    "session-1",
		Route:        "CHAT",
		Status:       StatusNotChecked,
		TriggerLevel: TriggerLow,
		CreatedAt:    time.Now().UTC(),
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("expected report to validate: %v", err)
	}
	report.Status = VerificationStatus("passed")
	if err := report.Validate(); err == nil {
		t.Fatal("expected invalid report status to be rejected")
	}
}
