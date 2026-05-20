package knowledgememory

import (
	"strings"
	"testing"
	"time"
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

func TestValidateTemporalMemoryMarkerRejectsNegativeAccessCount(t *testing.T) {
	err := ValidateTemporalMemoryMarker(TemporalMemoryMarker{
		MarkerID:    "tm_1",
		Layer:       "week",
		ReferenceID: "ref_1",
		Summary:     "summary",
		AccessCount: -1,
	})
	if err == nil || !strings.Contains(err.Error(), "access_count") {
		t.Fatalf("expected access_count error, got %v", err)
	}
}

func TestValidateKnowledgeMemoryRejectsMissingCreatedAt(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "personal archive",
			err: ValidatePersonalArchiveEntry(PersonalArchiveEntry{
				EntryID:      "pa_1",
				UserID:       "ren",
				OriginalText: "bio",
				Protected:    true,
			}),
		},
		{
			name: "creative",
			err: ValidateCreativeKnowledgeItem(CreativeKnowledgeItem{
				ItemID: "ck_1",
				Title:  "Work",
				Status: "candidate",
			}),
		},
		{
			name: "news",
			err: ValidateNewsKnowledgeItem(NewsKnowledgeItem{
				ItemID: "news_1",
				Source: "example",
				Topic:  "tech",
				Status: "candidate",
			}),
		},
		{
			name: "daily intake",
			err: ValidateDailyIntakeRule(DailyIntakeRule{
				RuleID:  "rule_1",
				UserID:  "ren",
				Topic:   "AI",
				Cadence: "daily",
				Status:  "active",
			}),
		},
		{
			name: "temporal marker",
			err: ValidateTemporalMemoryMarker(TemporalMemoryMarker{
				MarkerID:    "tm_1",
				Layer:       "today",
				ReferenceID: "ref_1",
				Summary:     "summary",
			}),
		},
		{
			name: "dream",
			err: ValidateDreamConsolidationRun(DreamConsolidationRun{
				RunID:        "dream_1",
				Status:       "proposal",
				ReviewStatus: "pending",
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

func TestValidateKnowledgeMemoryRejectsUnknownStatus(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "creative",
			err: ValidateCreativeKnowledgeItem(CreativeKnowledgeItem{
				ItemID:    "ck_1",
				Title:     "Work",
				Status:    "done",
				CreatedAt: now,
			}),
		},
		{
			name: "news",
			err: ValidateNewsKnowledgeItem(NewsKnowledgeItem{
				ItemID:    "news_1",
				Source:    "example",
				Topic:     "tech",
				Status:    "done",
				CreatedAt: now,
			}),
		},
		{
			name: "daily intake",
			err: ValidateDailyIntakeRule(DailyIntakeRule{
				RuleID:    "rule_1",
				UserID:    "ren",
				Topic:     "AI",
				Cadence:   "daily",
				Status:    "done",
				CreatedAt: now,
			}),
		},
		{
			name: "dream status",
			err: ValidateDreamConsolidationRun(DreamConsolidationRun{
				RunID:        "dream_1",
				Status:       "done",
				ReviewStatus: "pending",
				CreatedAt:    now,
			}),
		},
		{
			name: "dream review status",
			err: ValidateDreamConsolidationRun(DreamConsolidationRun{
				RunID:        "dream_1",
				Status:       "proposal",
				ReviewStatus: "done",
				CreatedAt:    now,
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil || !strings.Contains(tt.err.Error(), "unsupported") {
				t.Fatalf("validation error = %v, want unsupported", tt.err)
			}
		})
	}
}

func TestValidateDreamConsolidationRejectsAutoApprove(t *testing.T) {
	err := ValidateDreamConsolidationRun(DreamConsolidationRun{
		RunID:        "dream_1",
		Status:       "draft",
		ReviewStatus: "approved",
		CreatedAt:    time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC),
	})
	if err == nil || !strings.Contains(err.Error(), "auto-approved") {
		t.Fatalf("expected auto-approved error, got %v", err)
	}
}

func TestValidateDreamConsolidationRejectsInconsistentReviewState(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		run  DreamConsolidationRun
		want string
	}{
		{
			name: "pending promoted",
			run:  DreamConsolidationRun{RunID: "dream_1", Status: "promoted", ReviewStatus: "pending", CreatedAt: now},
			want: "pending review",
		},
		{
			name: "rejected reviewed",
			run:  DreamConsolidationRun{RunID: "dream_1", Status: "reviewed", ReviewStatus: "rejected", CreatedAt: now},
			want: "rejected review",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDreamConsolidationRun(tt.run)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ValidateDreamConsolidationRun() error = %v, want %q", err, tt.want)
			}
		})
	}
}
