package complexity

import (
	"strings"
	"testing"
	"time"
)

func TestValidateScanEventRequiresReportOnlyMode(t *testing.T) {
	item := ScanEvent{
		ScanID:    "scan_1",
		Repo:      "repo",
		Mode:      "apply",
		Status:    "completed",
		CreatedAt: time.Now(),
	}
	if err := ValidateScanEvent(item); err == nil || !strings.Contains(err.Error(), "report_only") {
		t.Fatalf("expected report_only validation error, got %v", err)
	}
}

func TestValidateHotspotRejectsInvalidConfidence(t *testing.T) {
	item := Hotspot{
		HotspotID:           "hot_1",
		ScanID:              "scan_1",
		FilePath:            "src/app.go",
		HotspotType:         "nested_loop",
		EstimatedComplexity: "O(n^2)",
		RiskLevel:           "medium",
		Summary:             "nested loop",
		Confidence:          1.2,
		CreatedAt:           time.Now(),
	}
	if err := ValidateHotspot(item); err == nil || !strings.Contains(err.Error(), "confidence") {
		t.Fatalf("expected confidence validation error, got %v", err)
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
