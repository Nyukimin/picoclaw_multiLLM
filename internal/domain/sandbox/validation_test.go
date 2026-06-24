package sandbox

import (
	"strings"
	"testing"
	"time"
)

func TestValidateSandboxRejectsMissingCreatedAt(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{name: "sandbox", err: ValidateSandboxRecord(SandboxRecord{SandboxID: "sbx_1", Type: "worktree", Path: "/tmp/sbx", Status: SandboxStatusActive})},
		{name: "artifact", err: ValidateSandboxArtifact(SandboxArtifact{ArtifactID: "art_1", SandboxID: "sbx_1", Type: "rollback_plan", FilePath: "rollback.md", Status: "completed"})},
		{name: "promotion", err: ValidatePromotionRequest(PromotionRequest{PromotionID: "promo_1", SandboxID: "sbx_1", TargetPath: "internal/app.go"})},
		{name: "gate", err: ValidatePromotionGateLog(PromotionGateLog{EventID: "evt_1", PromotionID: "promo_1", GateStatus: GateStatusNeedsReview})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil || !strings.Contains(tc.err.Error(), "created_at") {
				t.Fatalf("err=%v, want created_at", tc.err)
			}
		})
	}
}

func TestValidateSandboxAcceptsTimestampedRecords(t *testing.T) {
	now := time.Date(2026, 5, 20, 7, 50, 0, 0, time.UTC)
	if err := ValidateSandboxRecord(SandboxRecord{SandboxID: "sbx_1", Type: "worktree", Path: "/tmp/sbx", Status: SandboxStatusActive, CreatedAt: now}); err != nil {
		t.Fatalf("sandbox should be valid: %v", err)
	}
	if err := ValidateSandboxArtifact(SandboxArtifact{ArtifactID: "art_1", SandboxID: "sbx_1", Type: "rollback_plan", FilePath: "rollback.md", Status: "completed", CreatedAt: now}); err != nil {
		t.Fatalf("artifact should be valid: %v", err)
	}
	if err := ValidatePromotionRequest(PromotionRequest{PromotionID: "promo_1", SandboxID: "sbx_1", TargetPath: "internal/app.go", CreatedAt: now}); err != nil {
		t.Fatalf("promotion should be valid: %v", err)
	}
	if err := ValidatePromotionGateLog(PromotionGateLog{EventID: "evt_1", PromotionID: "promo_1", GateStatus: GateStatusNeedsReview, CreatedAt: now}); err != nil {
		t.Fatalf("gate log should be valid: %v", err)
	}
}
