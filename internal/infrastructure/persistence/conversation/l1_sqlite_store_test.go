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

func TestL1SQLiteStore_SearchCacheFreshHit(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	entry, err := store.SaveSearchCache(
		ctx,
		"web",
		"  RenCrow   最新 仕様 ",
		`[{"title":"RenCrow memo"}]`,
		[]string{"https://example.com/rencrow"},
		time.Hour,
	)
	if err != nil {
		t.Fatalf("SaveSearchCache failed: %v", err)
	}
	if entry.NormalizedQuery != "rencrow 最新 仕様" {
		t.Fatalf("unexpected normalized query: %s", entry.NormalizedQuery)
	}

	hit, err := store.GetFreshSearchCache(ctx, "web", "rencrow 最新 仕様", entry.RetrievedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetFreshSearchCache failed: %v", err)
	}
	if hit == nil {
		t.Fatal("expected fresh cache hit")
	}
	if hit.QueryHash != entry.QueryHash || hit.ResultsJSON != `[{"title":"RenCrow memo"}]` {
		t.Fatalf("unexpected cache hit: %+v", hit)
	}
	if len(hit.SourceURLs) != 1 || hit.SourceURLs[0] != "https://example.com/rencrow" {
		t.Fatalf("unexpected source urls: %+v", hit.SourceURLs)
	}
}

func TestL1SQLiteStore_SearchCacheMissesAfterExpiry(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	entry, err := store.SaveSearchCache(ctx, "web", "short lived", `[]`, nil, time.Second)
	if err != nil {
		t.Fatalf("SaveSearchCache failed: %v", err)
	}
	hit, err := store.GetFreshSearchCache(ctx, "web", "short lived", entry.ExpiresAt.Add(time.Nanosecond))
	if err != nil {
		t.Fatalf("GetFreshSearchCache failed: %v", err)
	}
	if hit != nil {
		t.Fatalf("expected expired cache miss, got %+v", hit)
	}
}

func TestL1SQLiteStore_SearchCacheRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	if _, err := store.SaveSearchCache(ctx, "web", "query", `{bad`, nil, time.Hour); err == nil {
		t.Fatal("expected invalid JSON to be rejected")
	}
	if _, err := store.SaveSearchCache(ctx, "web", "   ", `[]`, nil, time.Hour); err == nil {
		t.Fatal("expected blank query to be rejected")
	}
	if _, err := store.GetFreshSearchCache(ctx, "web", "   ", time.Now()); err == nil {
		t.Fatal("expected blank query lookup to be rejected")
	}
}

func TestL1SQLiteStore_EventLogAppendAndRecent(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	first, err := store.AppendEvent(ctx, "search.cache_hit", "conv:123", "session-1", 123, map[string]interface{}{
		"query": "RenCrow 最新仕様",
	}, "search_cache")
	if err != nil {
		t.Fatalf("AppendEvent first failed: %v", err)
	}
	second, err := store.AppendEvent(ctx, "memory.promoted", "conv:123", "session-1", 123, map[string]interface{}{
		"memory_state": MemoryStateConfirmed,
	}, "memory")
	if err != nil {
		t.Fatalf("AppendEvent second failed: %v", err)
	}
	if first.ID == "" || second.ID == "" || first.ID == second.ID {
		t.Fatalf("unexpected event ids: first=%q second=%q", first.ID, second.ID)
	}

	events, err := store.RecentEvents(ctx, "conv:123", 10)
	if err != nil {
		t.Fatalf("RecentEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].EventType != "memory.promoted" || events[1].EventType != "search.cache_hit" {
		t.Fatalf("unexpected event order: %+v", events)
	}
	if events[0].Payload["memory_state"] != MemoryStateConfirmed {
		t.Fatalf("unexpected payload: %+v", events[0].Payload)
	}
	if events[1].Source != "search_cache" || events[1].SessionID != "session-1" || events[1].ThreadID != 123 {
		t.Fatalf("unexpected event fields: %+v", events[1])
	}
}

func TestL1SQLiteStore_EventLogRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	if _, err := store.AppendEvent(ctx, "", "conv:123", "session-1", 123, nil, "test"); err == nil {
		t.Fatal("expected blank event type to be rejected")
	}
	if _, err := store.AppendEvent(ctx, "test.event", "", "session-1", 123, nil, "test"); err == nil {
		t.Fatal("expected blank namespace to be rejected")
	}
}

func TestL1SQLiteStore_SaveMessageAppendsEventLog(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.Message{
		Speaker:   domconv.SpeakerUser,
		Msg:       "イベントにも残す",
		Timestamp: time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
	}
	if err := store.SaveMessage(ctx, "session-1", 123, "conv:123", msg, MemoryStateObserved); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	events, err := store.RecentEvents(ctx, "conv:123", 10)
	if err != nil {
		t.Fatalf("RecentEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != "memory.message_saved" || ev.Source != "conversation" {
		t.Fatalf("unexpected event identity: %+v", ev)
	}
	if ev.Payload["speaker"] != string(domconv.SpeakerUser) || ev.Payload["memory_state"] != MemoryStateObserved {
		t.Fatalf("unexpected event payload: %+v", ev.Payload)
	}
}

func TestL1SQLiteStore_SaveSearchCacheAppendsEventLog(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	entry, err := store.SaveSearchCache(ctx, "web", "RenCrow 最新仕様", `[{"title":"memo"}]`, []string{"https://example.com"}, time.Hour)
	if err != nil {
		t.Fatalf("SaveSearchCache failed: %v", err)
	}
	events, err := store.RecentEvents(ctx, "search:web", 10)
	if err != nil {
		t.Fatalf("RecentEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != "search.cache_saved" || ev.Source != "search_cache" {
		t.Fatalf("unexpected event identity: %+v", ev)
	}
	if ev.Payload["query_hash"] != entry.QueryHash || ev.Payload["normalized_query"] != "rencrow 最新仕様" {
		t.Fatalf("unexpected event payload: %+v", ev.Payload)
	}
}

func TestL1SQLiteStore_UpdateMemoryStateAppendsEventLog(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	msg := domconv.Message{
		Speaker:   domconv.SpeakerUser,
		Msg:       "昇格ログ",
		Timestamp: time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC),
	}
	if err := store.SaveMessage(ctx, "session-1", 456, "conv:456", msg, MemoryStateObserved); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	memories, err := store.RecentByNamespace(ctx, "conv:456", 10)
	if err != nil {
		t.Fatalf("RecentByNamespace failed: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory event, got %d", len(memories))
	}
	if err := store.UpdateMemoryState(ctx, memories[0].ID, MemoryStateCandidate); err != nil {
		t.Fatalf("UpdateMemoryState failed: %v", err)
	}

	events, err := store.RecentEvents(ctx, "conv:456", 10)
	if err != nil {
		t.Fatalf("RecentEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected message save and state update events, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != "memory.state_updated" || ev.Source != "memory" {
		t.Fatalf("unexpected event identity: %+v", ev)
	}
	if ev.Payload["memory_id"] != memories[0].ID || ev.Payload["memory_state"] != MemoryStateCandidate {
		t.Fatalf("unexpected event payload: %+v", ev.Payload)
	}
}
