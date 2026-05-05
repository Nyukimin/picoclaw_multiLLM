package conversation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

func TestDuckDBStore_ExportThreadSummariesParquet(t *testing.T) {
	ctx := context.Background()
	store, err := NewDuckDBStore(":memory:")
	if err != nil {
		t.Fatalf("NewDuckDBStore failed: %v", err)
	}
	defer store.Close()

	if err := store.SaveThreadSummary(ctx, &domconv.ThreadSummary{
		ThreadID:  101,
		StartTime: time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2026, 5, 1, 10, 10, 0, 0, time.UTC),
		Domain:    "ai",
		Summary:   "AI discussion",
		Keywords:  []string{"ai", "local-llm"},
		Embedding: []float32{0.1, 0.2},
		IsNovel:   true,
	}); err != nil {
		t.Fatalf("SaveThreadSummary failed: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "thread_summaries.parquet")
	if err := store.ExportThreadSummariesParquet(ctx, outPath); err != nil {
		t.Fatalf("ExportThreadSummariesParquet failed: %v", err)
	}
	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("expected parquet file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("parquet file should not be empty")
	}

	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM read_parquet(?)", outPath).Scan(&count); err != nil {
		t.Fatalf("read_parquet failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("parquet row count: want 1, got %d", count)
	}
}
