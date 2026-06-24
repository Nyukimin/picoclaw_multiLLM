package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

func TestRunKnowledgeCommandImportCoreJSONL(t *testing.T) {
	store, err := conversationpersistence.NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	inputPath := filepath.Join(t.TempDir(), "knowledge.jsonl")
	if err := os.WriteFile(inputPath, []byte(`{"id":"movie:test","domain":"movie","title":"Test Movie","summary":"映画メモ","source_id":"manual:seed"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	var out, errOut bytes.Buffer

	code := runKnowledgeCommand([]string{"import-core-jsonl", inputPath, "--json"}, store, &out, &errOut)
	if code != 0 {
		t.Fatalf("import should pass, code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"imported":1`) {
		t.Fatalf("expected json result, got %s", out.String())
	}
	items, err := store.RecentStagingItems(context.Background(), conversationpersistence.L1StagingStatusPending, 10)
	if err != nil {
		t.Fatalf("RecentStagingItems failed: %v", err)
	}
	if len(items) != 1 || items[0].Namespace != "kb:movie" || items[0].EventID != "movie:test" {
		t.Fatalf("unexpected staged items: %+v", items)
	}
}
