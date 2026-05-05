package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type sourceRegistryStoreStub struct {
	entries []conversationpersistence.L1SourceRegistryEntry
	saved   []conversationpersistence.L1SourceRegistryEntry
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
