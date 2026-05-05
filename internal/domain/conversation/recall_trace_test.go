package conversation

import (
	"testing"
	"time"
)

func TestRecallPackToTraceItems(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	rp := &RecallPack{
		MidSummaries: []ThreadSummary{{Summary: "mid memory", Domain: "movie"}},
		LongFacts:    []string{"long fact"},
		KBSnippets:   []string{"kb snippet"},
		SearchCacheSnippets: []SearchCacheSnippet{{
			Query:       "latest ai",
			Provider:    "web",
			ResultsJSON: `{"ok":true}`,
			SourceURLs:  []string{"https://example.com/a"},
			RetrievedAt: now,
		}},
	}

	items := rp.ToTraceItems()
	if len(items) != 4 {
		t.Fatalf("expected 4 trace items, got %d: %+v", len(items), items)
	}
	if items[0].Layer != "L2" || items[0].Kind != "thread_summary" || items[0].Summary != "mid memory" {
		t.Fatalf("unexpected mid trace: %+v", items[0])
	}
	if items[3].Layer != "L1" || items[3].Kind != "search_cache" || items[3].SourceURLs[0] != "https://example.com/a" {
		t.Fatalf("unexpected search trace: %+v", items[3])
	}
}
