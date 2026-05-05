package viewer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type sourceRegistryStoreStub struct {
	entries []conversationpersistence.L1SourceRegistryEntry
	saved   []conversationpersistence.L1SourceRegistryEntry
}

func TestHandleSourceRegistry_RunSelectedSource(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>Test</title>
<item><title>Viewer Run</title><link>https://example.com/viewer-run</link><description>Viewer body</description><pubDate>Tue, 05 May 2026 10:00:00 GMT</pubDate></item>
</channel></rss>`))
	}))
	defer srv.Close()
	store, err := conversationpersistence.NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	if _, err := store.SaveSourceRegistryEntry(ctx, conversationpersistence.L1SourceRegistryEntry{
		SourceID:      "rss:viewer-run",
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
	h := HandleSourceRegistry(store)
	req := httptest.NewRequest(http.MethodPost, "/viewer/source-registry?action=run&source_id=rss:viewer-run", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	news, err := store.RecentNewsItems(ctx, "ai", 10)
	if err != nil {
		t.Fatalf("RecentNewsItems failed: %v", err)
	}
	if len(news) != 1 || news[0].SummaryDraft != "Viewer Run" {
		t.Fatalf("unexpected promoted news: %+v", news)
	}
}

func (s *sourceRegistryStoreStub) SaveSourceRegistryEntry(_ context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error) {
	s.saved = append(s.saved, entry)
	return &entry, nil
}

func (s *sourceRegistryStoreStub) ListSourceRegistryEntries(_ context.Context, enabledOnly bool) ([]conversationpersistence.L1SourceRegistryEntry, error) {
	if !enabledOnly {
		return s.entries, nil
	}
	out := make([]conversationpersistence.L1SourceRegistryEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		if entry.Enabled {
			out = append(out, entry)
		}
	}
	return out, nil
}

func (s *sourceRegistryStoreStub) DueSourceRegistryEntries(_ context.Context, _ time.Time) ([]conversationpersistence.L1SourceRegistryEntry, error) {
	return nil, nil
}
func (s *sourceRegistryStoreStub) SourceTrustScores(_ context.Context) (map[string]float64, error) {
	return map[string]float64{}, nil
}
func (s *sourceRegistryStoreStub) StageSourceRegistryFetch(_ context.Context, _ string, _ conversationpersistence.L1SourceFetchPayload) (*conversationpersistence.L1StagingItem, error) {
	return nil, fmt.Errorf("not used")
}
func (s *sourceRegistryStoreStub) ValidateStagingItem(_ context.Context, _ string, _ conversationpersistence.L1StagingValidationPolicy) (*conversationpersistence.L1StagingValidationResult, error) {
	return nil, fmt.Errorf("not used")
}
func (s *sourceRegistryStoreStub) PromoteValidatedStagingItemToNews(_ context.Context, _ string, _ string) (*conversationpersistence.L1NewsItem, error) {
	return nil, fmt.Errorf("not used")
}
func (s *sourceRegistryStoreStub) PromoteValidatedStagingItemToKnowledge(_ context.Context, _ string, _ string) (*conversationpersistence.L1KnowledgeItem, error) {
	return nil, fmt.Errorf("not used")
}
func (s *sourceRegistryStoreStub) MarkSourceRegistryFetched(_ context.Context, _ string, _ time.Time, _ string, _ string) error {
	return nil
}

func TestHandleSourceRegistry_JSONSaveAndList(t *testing.T) {
	store := &sourceRegistryStoreStub{}
	h := HandleSourceRegistry(store)

	body := `{"source_id":"rss:ai","url":"https://example.com/feed.xml","kind":"rss","trust_score":0.8,"fetch_interval_sec":3600,"license_note":"public feed","enabled":true,"meta":{"namespace":"kb:news"}}`
	req := httptest.NewRequest(http.MethodPost, "/viewer/source-registry", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].FetchInterval != time.Hour || store.saved[0].Meta["namespace"] != "kb:news" {
		t.Fatalf("unexpected saved entry: %+v", store.saved)
	}

	store.entries = store.saved
	req = httptest.NewRequest(http.MethodGet, "/viewer/source-registry", nil)
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Entries []sourceRegistryEntryDTO `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.Entries) != 1 || out.Entries[0].SourceID != "rss:ai" {
		t.Fatalf("unexpected entries: %+v", out.Entries)
	}
}

func TestHandleSourceRegistry_YAMLImportExport(t *testing.T) {
	store := &sourceRegistryStoreStub{}
	h := HandleSourceRegistry(store)
	yamlBody := `
entries:
  - source_id: rss:movie
    url: https://example.com/movie.xml
    kind: rss
    trust_score: 0.7
    fetch_interval_sec: 7200
    license_note: public feed
    enabled: true
    meta:
      namespace: kb:movie
`
	req := httptest.NewRequest(http.MethodPost, "/viewer/source-registry?format=yaml", strings.NewReader(yamlBody))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(store.saved) != 1 || store.saved[0].SourceID != "rss:movie" {
		t.Fatalf("unexpected yaml import: %+v", store.saved)
	}

	store.entries = store.saved
	req = httptest.NewRequest(http.MethodGet, "/viewer/source-registry?format=yaml", nil)
	rec = httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "source_id: rss:movie") || rec.Header().Get("Content-Type") != "application/x-yaml" {
		t.Fatalf("unexpected yaml export: content-type=%q body=%s", rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
