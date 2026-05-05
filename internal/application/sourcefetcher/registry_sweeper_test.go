package sourcefetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

func TestSweepDueSourcesStagesValidatesAndPromotesRSS(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>AI Update</title><link>` + "https://example.com/ai-update" + `</link><description>Local LLM news</description><pubDate>Tue, 05 May 2026 10:00:00 GMT</pubDate></item>
</channel></rss>`))
	}))
	defer srv.Close()

	store, err := conversationpersistence.NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	if _, err := store.SaveSourceRegistryEntry(ctx, conversationpersistence.L1SourceRegistryEntry{
		SourceID:      "rss:test",
		URL:           srv.URL,
		Kind:          conversationpersistence.L1SourceKindRSS,
		TrustScore:    0.9,
		FetchInterval: time.Hour,
		LicenseNote:   "rss",
		Enabled:       true,
		Meta: map[string]interface{}{
			"category":  "ai",
			"namespace": "kb:ai",
		},
	}); err != nil {
		t.Fatalf("SaveSourceRegistryEntry failed: %v", err)
	}

	result, err := SweepDueSources(ctx, store, now, SweepOptions{LimitPerSource: 5, MinimumTrustScore: 0.5})
	if err != nil {
		t.Fatalf("SweepDueSources failed: %v", err)
	}
	if result.Sources != 1 || result.Staged != 1 || result.Validated != 1 || result.PromotedNews != 1 {
		t.Fatalf("unexpected sweep result: %+v", result)
	}
	news, err := store.RecentNewsItems(ctx, "ai", 10)
	if err != nil {
		t.Fatalf("RecentNewsItems failed: %v", err)
	}
	if len(news) != 1 || news[0].SummaryDraft != "AI Update" || news[0].SourceID != "rss:test" {
		t.Fatalf("unexpected promoted news: %+v", news)
	}
	due, err := store.DueSourceRegistryEntries(ctx, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("DueSourceRegistryEntries failed: %v", err)
	}
	if len(due) != 0 {
		t.Fatalf("source should not be due immediately after sweep: %+v", due)
	}
}

func TestRunSourceStagesValidatesAndPromotesSelectedRSS(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>Selected Update</title><link>` + "https://example.com/selected" + `</link><description>Selected body</description><pubDate>Tue, 05 May 2026 10:00:00 GMT</pubDate></item>
</channel></rss>`))
	}))
	defer srv.Close()

	store, err := conversationpersistence.NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	if _, err := store.SaveSourceRegistryEntry(ctx, conversationpersistence.L1SourceRegistryEntry{
		SourceID:      "rss:selected",
		URL:           srv.URL,
		Kind:          conversationpersistence.L1SourceKindRSS,
		TrustScore:    0.9,
		FetchInterval: time.Hour,
		LicenseNote:   "rss",
		Enabled:       true,
		Meta:          map[string]interface{}{"category": "ai", "namespace": "kb:ai"},
	}); err != nil {
		t.Fatalf("SaveSourceRegistryEntry failed: %v", err)
	}

	result, err := RunSource(ctx, store, "rss:selected", now, SweepOptions{LimitPerSource: 5, MinimumTrustScore: 0.5})
	if err != nil {
		t.Fatalf("RunSource failed: %v", err)
	}
	if result.Sources != 1 || result.Staged != 1 || result.Validated != 1 || result.PromotedNews != 1 {
		t.Fatalf("unexpected run result: %+v", result)
	}
	news, err := store.RecentNewsItems(ctx, "ai", 10)
	if err != nil {
		t.Fatalf("RecentNewsItems failed: %v", err)
	}
	if len(news) != 1 || news[0].SummaryDraft != "Selected Update" {
		t.Fatalf("unexpected promoted news: %+v", news)
	}
}

func TestRunSourceStagesValidatesAndPromotesPyPIHTTPSource(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"info":{"name":"sample","summary":"sample package"},"releases":{"1.0.0":[]}}`))
	}))
	defer srv.Close()

	store, err := conversationpersistence.NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	if _, err := store.SaveSourceRegistryEntry(ctx, conversationpersistence.L1SourceRegistryEntry{
		SourceID:      "pypi:sample",
		URL:           srv.URL,
		Kind:          conversationpersistence.L1SourceKindPyPI,
		TrustScore:    0.9,
		FetchInterval: time.Hour,
		LicenseNote:   "pypi json api",
		Enabled:       true,
		Meta:          map[string]interface{}{"namespace": "kb:pypi", "domain": "pypi", "title": "sample"},
	}); err != nil {
		t.Fatalf("SaveSourceRegistryEntry failed: %v", err)
	}

	result, err := RunSource(ctx, store, "pypi:sample", now, SweepOptions{LimitPerSource: 5, MinimumTrustScore: 0.5})
	if err != nil {
		t.Fatalf("RunSource failed: %v", err)
	}
	if result.Sources != 1 || result.Staged != 1 || result.Validated != 1 || result.PromotedKnowledge != 1 {
		t.Fatalf("unexpected run result: %+v", result)
	}
	items, err := store.RecentKnowledgeItems(ctx, "pypi", 10)
	if err != nil {
		t.Fatalf("RecentKnowledgeItems failed: %v", err)
	}
	if len(items) != 1 || items[0].Title != "sample" || items[0].SourceID != "pypi:sample" {
		t.Fatalf("unexpected promoted knowledge: %+v", items)
	}
}
