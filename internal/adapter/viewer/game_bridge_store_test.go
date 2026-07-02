package viewer

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGameBridgeStoreDeduplicatesEventID(t *testing.T) {
	store := NewGameBridgeStore(filepath.Join(t.TempDir(), "game_bridge_events.jsonl"))
	req := GameResultRequest{
		GameID:          "territory_commander",
		SessionID:       "tc_test",
		Turn:            3,
		Persona:         "mio",
		ExecutedActions: []string{"defend"},
		Result:          map[string]any{"success": true, "event": "defended_center"},
	}

	first, err := store.SaveGameBridgeResult(context.Background(), req)
	if err != nil {
		t.Fatalf("first SaveGameBridgeResult returned error: %v", err)
	}
	second, err := store.SaveGameBridgeResult(context.Background(), req)
	if err != nil {
		t.Fatalf("second SaveGameBridgeResult returned error: %v", err)
	}
	if first.EventID != second.EventID || first.CreatedAt != second.CreatedAt {
		t.Fatalf("duplicate save did not return existing event\nfirst=%+v\nsecond=%+v", first, second)
	}

	events, err := store.RecentGameBridgeEvents(context.Background(), "territory_commander", "tc_test", 10)
	if err != nil {
		t.Fatalf("RecentGameBridgeEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
}
