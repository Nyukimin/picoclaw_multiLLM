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

func TestDuckDBStore_ArchiveL1DataParquet(t *testing.T) {
	ctx := context.Background()
	store, err := NewDuckDBStore(":memory:")
	if err != nil {
		t.Fatalf("NewDuckDBStore failed: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 5, 5, 9, 0, 0, 0, time.UTC)
	if err := store.ArchiveL1MemoryEvents(ctx, []L1MemoryEvent{{
		ID:          "mem-1",
		Namespace:   "user:ren",
		SessionID:   "sess-1",
		ThreadID:    1,
		Speaker:     "mio",
		Message:     "confirmed preference",
		Meta:        map[string]interface{}{"type": "preference"},
		MemoryState: MemoryStateConfirmed,
		Layer:       MemoryLayerL1,
		Source:      "test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}}); err != nil {
		t.Fatalf("ArchiveL1MemoryEvents failed: %v", err)
	}
	if err := store.ArchiveL1NewsItems(ctx, []L1NewsItem{{
		ID:           "news-1",
		StagingID:    "stage-news-1",
		Category:     "ai",
		SourceID:     "rss:test",
		SourceURL:    "https://example.com/news/1",
		PublishedAt:  now,
		FetchedAt:    now,
		RawText:      "raw news",
		RawHash:      "hash-news-1",
		SummaryDraft: "summary news",
		Keywords:     []string{"ai", "local"},
		LicenseNote:  "public feed",
		Meta:         map[string]interface{}{"event_id": "evt-news-1"},
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("ArchiveL1NewsItems failed: %v", err)
	}
	if err := store.ArchiveL1KnowledgeItems(ctx, []L1KnowledgeItem{{
		ID:           "kb-1",
		StagingID:    "stage-kb-1",
		Domain:       "movie",
		Title:        "Interstellar",
		SourceID:     "manual",
		SourceURL:    "https://example.com/kb/1",
		RawText:      "raw kb",
		RawHash:      "hash-kb-1",
		SummaryDraft: "summary kb",
		Keywords:     []string{"space"},
		LicenseNote:  "manual",
		Meta:         map[string]interface{}{"year": 2014},
		CreatedAt:    now,
		UpdatedAt:    now,
	}}); err != nil {
		t.Fatalf("ArchiveL1KnowledgeItems failed: %v", err)
	}

	outDir := t.TempDir()
	paths, err := store.ExportL1ArchivesParquet(ctx, outDir)
	if err != nil {
		t.Fatalf("ExportL1ArchivesParquet failed: %v", err)
	}
	for _, name := range []string{L1ArchiveMemory, L1ArchiveNews, L1ArchiveKnowledge} {
		path := paths[name]
		if path == "" {
			t.Fatalf("missing parquet path for %s", name)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected parquet file for %s: %v", name, err)
		}
		if info.Size() == 0 {
			t.Fatalf("parquet file for %s should not be empty", name)
		}
		var count int
		if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM read_parquet(?)", path).Scan(&count); err != nil {
			t.Fatalf("read_parquet %s failed: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("parquet row count for %s: want 1, got %d", name, count)
		}
	}
}
