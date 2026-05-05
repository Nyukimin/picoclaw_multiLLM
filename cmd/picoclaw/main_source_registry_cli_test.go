package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type sourceRegistryCLIStoreStub struct {
	entries []conversationpersistence.L1SourceRegistryEntry
}

func (s *sourceRegistryCLIStoreStub) SaveSourceRegistryEntry(_ context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error) {
	for i := range s.entries {
		if s.entries[i].SourceID == entry.SourceID {
			s.entries[i] = entry
			return &entry, nil
		}
	}
	s.entries = append(s.entries, entry)
	return &entry, nil
}

func (s *sourceRegistryCLIStoreStub) ListSourceRegistryEntries(_ context.Context, enabledOnly bool) ([]conversationpersistence.L1SourceRegistryEntry, error) {
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

func TestRunSourceRegistryCommand_SaveAndList(t *testing.T) {
	store := &sourceRegistryCLIStoreStub{}
	var out, errOut bytes.Buffer

	code := runSourceRegistryCommand([]string{
		"save",
		"--source-id", "rss:ai",
		"--url", "https://example.com/feed.xml",
		"--kind", "rss",
		"--trust-score", "0.8",
		"--interval-sec", "7200",
		"--license-note", "public feed",
		"--namespace", "kb:news",
		"--json",
	}, store, &out, &errOut)
	if code != 0 {
		t.Fatalf("save should pass, code=%d err=%s", code, errOut.String())
	}
	if len(store.entries) != 1 {
		t.Fatalf("expected 1 saved entry, got %d", len(store.entries))
	}
	got := store.entries[0]
	if got.SourceID != "rss:ai" || got.URL != "https://example.com/feed.xml" || got.FetchInterval != 2*time.Hour || got.Meta["namespace"] != "kb:news" {
		t.Fatalf("unexpected saved entry: %+v", got)
	}
	if !strings.Contains(out.String(), `"source_id":"rss:ai"`) {
		t.Fatalf("expected json output, got %s", out.String())
	}

	out.Reset()
	errOut.Reset()
	code = runSourceRegistryCommand([]string{"list", "--json"}, store, &out, &errOut)
	if code != 0 {
		t.Fatalf("list should pass, code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"source_id":"rss:ai"`) {
		t.Fatalf("expected listed entry, got %s", out.String())
	}
}

func TestRunSourceRegistryCommand_SaveRequiresFields(t *testing.T) {
	store := &sourceRegistryCLIStoreStub{}
	var out, errOut bytes.Buffer

	code := runSourceRegistryCommand([]string{"save", "--source-id", "rss:missing"}, store, &out, &errOut)
	if code == 0 {
		t.Fatal("save should fail without required fields")
	}
	if !strings.Contains(errOut.String(), "source-id, url, kind, license-note are required") {
		t.Fatalf("unexpected error: %s", errOut.String())
	}
}

func TestRunSourceRegistryCommand_Disable(t *testing.T) {
	store := &sourceRegistryCLIStoreStub{entries: []conversationpersistence.L1SourceRegistryEntry{{
		SourceID:      "rss:ai",
		URL:           "https://example.com/feed.xml",
		Kind:          conversationpersistence.L1SourceKindRSS,
		TrustScore:    0.8,
		FetchInterval: time.Hour,
		LicenseNote:   "public feed",
		Enabled:       true,
		Meta:          map[string]interface{}{"namespace": "kb:news"},
	}}}
	var out, errOut bytes.Buffer

	code := runSourceRegistryCommand([]string{"disable", "rss:ai", "--json"}, store, &out, &errOut)
	if code != 0 {
		t.Fatalf("disable should pass, code=%d err=%s", code, errOut.String())
	}
	if len(store.entries) != 1 || store.entries[0].Enabled {
		t.Fatalf("entry should be disabled: %+v", store.entries)
	}
	if !strings.Contains(out.String(), `"enabled":false`) {
		t.Fatalf("expected disabled json output, got %s", out.String())
	}
}
