package conversation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

func TestL1SQLiteStore_SaveMessageAndRecentByNamespace(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.Message{
		Speaker:   domconv.SpeakerUser,
		Msg:       "覚えておく候補",
		Timestamp: time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		Meta:      map[string]interface{}{"route": "chat"},
	}
	if err := store.SaveMessage(ctx, "session-1", 123, "conv:123", msg, MemoryStateObserved); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	events, err := store.RecentByNamespace(ctx, "conv:123", 10)
	if err != nil {
		t.Fatalf("RecentByNamespace failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.Namespace != "conv:123" || ev.SessionID != "session-1" || ev.ThreadID != 123 {
		t.Fatalf("unexpected identity fields: %+v", ev)
	}
	if ev.Speaker != domconv.SpeakerUser || ev.Message != "覚えておく候補" {
		t.Fatalf("unexpected message fields: %+v", ev)
	}
	if ev.MemoryState != MemoryStateObserved || ev.Layer != MemoryLayerL1 {
		t.Fatalf("unexpected memory fields: %+v", ev)
	}
	if ev.Meta["route"] != "chat" {
		t.Fatalf("unexpected meta: %+v", ev.Meta)
	}
}

func TestL1SQLiteStore_DefaultNamespaceAndState(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.NewMessage(domconv.SpeakerMio, "返答", nil)
	if err := store.SaveMessage(ctx, "session-1", 456, "", msg, ""); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	events, err := store.RecentBySession(ctx, "session-1", 10)
	if err != nil {
		t.Fatalf("RecentBySession failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Namespace != "conv:456" {
		t.Fatalf("unexpected namespace: %s", events[0].Namespace)
	}
	if events[0].MemoryState != MemoryStateObserved {
		t.Fatalf("unexpected state: %s", events[0].MemoryState)
	}
}

func TestL1SQLiteStore_UpdateMemoryState(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.Message{
		Speaker:   domconv.SpeakerUser,
		Msg:       "候補から確定へ",
		Timestamp: time.Date(2026, 5, 5, 13, 0, 0, 0, time.UTC),
	}
	if err := store.SaveMessage(ctx, "session-1", 789, "conv:789", msg, MemoryStateObserved); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	events, err := store.RecentByNamespace(ctx, "conv:789", 10)
	if err != nil {
		t.Fatalf("RecentByNamespace failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if err := store.UpdateMemoryState(ctx, events[0].ID, MemoryStateCandidate); err != nil {
		t.Fatalf("UpdateMemoryState candidate failed: %v", err)
	}
	candidates, err := store.RecentByState(ctx, MemoryStateCandidate, 10)
	if err != nil {
		t.Fatalf("RecentByState candidate failed: %v", err)
	}
	if len(candidates) != 1 || candidates[0].ID != events[0].ID {
		t.Fatalf("unexpected candidate events: %+v", candidates)
	}

	if err := store.UpdateMemoryState(ctx, events[0].ID, MemoryStateConfirmed); err != nil {
		t.Fatalf("UpdateMemoryState confirmed failed: %v", err)
	}
	confirmed, err := store.RecentByState(ctx, MemoryStateConfirmed, 10)
	if err != nil {
		t.Fatalf("RecentByState confirmed failed: %v", err)
	}
	if len(confirmed) != 1 || confirmed[0].MemoryState != MemoryStateConfirmed {
		t.Fatalf("unexpected confirmed events: %+v", confirmed)
	}
	candidates, err = store.RecentByState(ctx, MemoryStateCandidate, 10)
	if err != nil {
		t.Fatalf("RecentByState candidate after confirm failed: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected no candidate events, got %+v", candidates)
	}
}

func TestL1SQLiteStore_RejectsInvalidMemoryState(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.NewMessage(domconv.SpeakerUser, "bad state", nil)
	if err := store.SaveMessage(ctx, "session-1", 1, "", msg, "trusted"); err == nil {
		t.Fatal("expected SaveMessage to reject invalid memory state")
	}
	if _, err := store.RecentByState(ctx, "trusted", 10); err == nil {
		t.Fatal("expected RecentByState to reject invalid memory state")
	}
	if err := store.UpdateMemoryState(ctx, "missing", "trusted"); err == nil {
		t.Fatal("expected UpdateMemoryState to reject invalid memory state")
	}
}
