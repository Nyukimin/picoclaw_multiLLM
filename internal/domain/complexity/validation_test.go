package complexity

import (
	"strings"
	"testing"
	"time"
)

func TestValidateScanEventRequiresReportOnlyMode(t *testing.T) {
	now := fixedComplexityValidationTime()
	item := ScanEvent{
		ScanID:    "scan_1",
		Repo:      "repo",
		Mode:      "apply",
		Status:    "completed",
		CreatedAt: now,
	}
	if err := ValidateScanEvent(item); err == nil || !strings.Contains(err.Error(), "report_only") {
		t.Fatalf("expected report_only validation error, got %v", err)
	}
}

func TestValidateScanEventRejectsCompletedWithoutCompletedAt(t *testing.T) {
	err := ValidateScanEvent(ScanEvent{
		ScanID:    "scan_1",
		Repo:      "repo",
		Mode:      "report_only",
		Status:    "completed",
		CreatedAt: fixedComplexityValidationTime(),
	})
	if err == nil || !strings.Contains(err.Error(), "completed_at") {
		t.Fatalf("expected completed_at validation error, got %v", err)
	}
}

func TestValidateHotspotRejectsInvalidConfidence(t *testing.T) {
	now := fixedComplexityValidationTime()
	item := Hotspot{
		HotspotID:           "hot_1",
		ScanID:              "scan_1",
		FilePath:            "src/app.go",
		HotspotType:         "nested_loop",
		EstimatedComplexity: "O(n^2)",
		RiskLevel:           "medium",
		Summary:             "nested loop",
		Confidence:          1.2,
		CreatedAt:           now,
	}
	if err := ValidateHotspot(item); err == nil || !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("expected confidence validation error, got %v", err)
	}
}

func TestValidateComplexityRejectsMissingCreatedAt(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "scan",
			err: ValidateScanEvent(ScanEvent{
				ScanID: "scan_1",
				Repo:   "repo",
				Mode:   "report_only",
				Status: "running",
			}),
		},
		{
			name: "hotspot",
			err: ValidateHotspot(Hotspot{
				HotspotID:           "hot_1",
				ScanID:              "scan_1",
				FilePath:            "src/app.go",
				HotspotType:         "nested_loop",
				EstimatedComplexity: "O(n^2)",
				RiskLevel:           "medium",
				Summary:             "nested loop",
			}),
		},
		{
			name: "evidence",
			err: ValidateHotspotEvidence(HotspotEvidence{
				EvidenceID: "ev_1",
				HotspotID:  "hot_1",
				FilePath:   "src/app.go",
			}),
		},
		{
			name: "report",
			err: ValidateReportArtifact(ReportArtifact{
				ArtifactID: "art_1",
				ScanID:     "scan_1",
				Type:       "complexity_hotspot_report",
				Title:      "Complexity Hotspot Report",
				Status:     "generated",
				Content:    "report",
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil || !strings.Contains(tt.err.Error(), "created_at") {
				t.Fatalf("validation error = %v, want created_at", tt.err)
			}
		})
	}
}

func TestValidateHotspotEvidenceRequiresIDs(t *testing.T) {
	err := ValidateHotspotEvidence(HotspotEvidence{FilePath: "src/app.go"})
	if err == nil || !strings.Contains(err.Error(), "evidence_id") {
		t.Fatalf("expected evidence_id validation error, got %v", err)
	}
}

func TestValidateReportArtifactRequiresContent(t *testing.T) {
	err := ValidateReportArtifact(ReportArtifact{
		ArtifactID: "art_1",
		ScanID:     "scan_1",
		Type:       "complexity_hotspot_report",
		Title:      "Complexity Hotspot Report",
		Status:     "generated",
	})
	if err == nil || !strings.Contains(err.Error(), "content") {
		t.Fatalf("expected content validation error, got %v", err)
	}
}

func fixedComplexityValidationTime() time.Time {
	return time.Date(2026, 5, 20, 6, 45, 0, 0, time.UTC)
}
