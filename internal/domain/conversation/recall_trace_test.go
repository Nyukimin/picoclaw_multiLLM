package conversation

import (
	"testing"
	"time"
)

func TestRecallPackToTraceItems(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	rp := &RecallPack{
		ShortContext: []Message{{Speaker: SpeakerUser, Msg: "今の話題"}},
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
	if len(items) != 5 {
		t.Fatalf("expected 5 trace items, got %d: %+v", len(items), items)
	}
	if items[0].Layer != "L0" || items[0].Kind != "short_context" || items[0].Summary != "今の話題" {
		t.Fatalf("unexpected L0 trace: %+v", items[0])
	}
	if items[1].Layer != "L2" || items[1].Kind != "thread_summary" || items[1].Summary != "mid memory" {
		t.Fatalf("unexpected mid trace: %+v", items[0])
	}
	if items[4].Layer != "L1" || items[4].Kind != "search_cache" || items[4].SourceURLs[0] != "https://example.com/a" {
		t.Fatalf("unexpected search trace: %+v", items[4])
	}
}
