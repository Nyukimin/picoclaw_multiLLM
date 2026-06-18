package viewer

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBacklogStoreSaveListLatest(t *testing.T) {
	store := NewBacklogStore(filepath.Join(t.TempDir(), "backlog.jsonl"))
	ctx := context.Background()

	if err := store.Save(ctx, BacklogItem{
		ItemID:   "item-1",
		Kind:     "idea",
		Title:    "面白い案",
		Source:   "mio",
		Status:   "open",
		Priority: "high",
	}); err != nil {
		t.Fatalf("save first: %v", err)
	}
	if err := store.Save(ctx, BacklogItem{
		ItemID:     "item-1",
		Kind:       "idea",
		Title:      "面白い案",
		Source:     "mio",
		Status:     "ok",
		CheckOK:    true,
		CheckedBy:  "ren",
		TestResult: "passed",
	}); err != nil {
		t.Fatalf("save update: %v", err)
	}

	items, err := store.List(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items len = %d", len(items))
	}
	if !items[0].CheckOK || items[0].Status != "ok" || items[0].TestResult != "passed" {
		t.Fatalf("latest item not returned: %+v", items[0])
	}
}
