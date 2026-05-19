package knowledgememory

import (
	"context"
	"testing"
	"time"

	domainkm "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/knowledgememory"
)

func TestJSONLStoreSavesKnowledgeMemoryRecords(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	if err := store.SavePersonalArchiveEntry(context.Background(), domainkm.PersonalArchiveEntry{
		EntryID:      "pa_1",
		UserID:       "ren",
		OriginalText: "bio",
		Protected:    true,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("SavePersonalArchiveEntry() error = %v", err)
	}
	if err := store.SaveDailyIntakeRule(context.Background(), domainkm.DailyIntakeRule{
		RuleID:    "rule_1",
		UserID:    "ren",
		Topic:     "AI",
		Cadence:   "daily",
		Status:    "active",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("SaveDailyIntakeRule() error = %v", err)
	}
	if err := store.SaveTemporalMemoryMarker(context.Background(), domainkm.TemporalMemoryMarker{
		MarkerID:    "tm_1",
		Layer:       "today",
		ReferenceID: "pa_1",
		Summary:     "bio",
		CreatedAt:   now,
	}); err != nil {
		t.Fatalf("SaveTemporalMemoryMarker() error = %v", err)
	}
	personal, err := store.ListPersonalArchiveEntries(context.Background(), 10)
	if err != nil || len(personal) != 1 {
		t.Fatalf("ListPersonalArchiveEntries() = %#v, %v", personal, err)
	}
	rules, err := store.ListDailyIntakeRules(context.Background(), 10)
	if err != nil || len(rules) != 1 {
		t.Fatalf("ListDailyIntakeRules() = %#v, %v", rules, err)
	}
	markers, err := store.ListTemporalMemoryMarkers(context.Background(), 10)
	if err != nil || len(markers) != 1 {
		t.Fatalf("ListTemporalMemoryMarkers() = %#v, %v", markers, err)
	}
}
