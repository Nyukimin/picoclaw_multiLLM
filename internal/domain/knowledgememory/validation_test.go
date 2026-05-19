package knowledgememory

import (
	"strings"
	"testing"
)

func TestValidatePersonalArchiveRequiresProtectedOriginal(t *testing.T) {
	err := ValidatePersonalArchiveEntry(PersonalArchiveEntry{
		EntryID:      "pa_1",
		UserID:       "ren",
		OriginalText: "bio",
		Protected:    false,
	})
	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("expected protected error, got %v", err)
	}
}

func TestValidateTemporalMemoryMarkerRejectsUnknownLayer(t *testing.T) {
	err := ValidateTemporalMemoryMarker(TemporalMemoryMarker{
		MarkerID:    "tm_1",
		Layer:       "unknown",
		ReferenceID: "ref_1",
		Summary:     "summary",
	})
	if err == nil || !strings.Contains(err.Error(), "layer") {
		t.Fatalf("expected layer error, got %v", err)
	}
}

func TestValidateDreamConsolidationRejectsAutoApprove(t *testing.T) {
	err := ValidateDreamConsolidationRun(DreamConsolidationRun{
		RunID:        "dream_1",
		Status:       "candidate",
		ReviewStatus: "approved",
	})
	if err == nil || !strings.Contains(err.Error(), "auto-approved") {
		t.Fatalf("expected auto-approved error, got %v", err)
	}
}
