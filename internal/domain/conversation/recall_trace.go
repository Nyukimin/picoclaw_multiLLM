package conversation

import "time"

type RecallTrace struct {
	ResponseID string
	SessionID  string
	Role       string
	Items      []RecallTraceItem
	CreatedAt  time.Time
}
type RecallTraceItem struct {
	Layer       string
	Kind        string
	Summary     string
	Query       string
	Provider    string
	SourceURLs  []string
	RetrievedAt time.Time
}

func (rp *RecallPack) ToTraceItems() []RecallTraceItem {
	if rp == nil {
		return nil
	}
	items := make([]RecallTraceItem, 0, len(rp.ShortContext)+len(rp.MidSummaries)+len(rp.LongFacts)+len(rp.KBSnippets)+len(rp.SearchCacheSnippets)+1)
	if rp.RollingSummary != "" {
		items = append(items, RecallTraceItem{
			Layer:   "L0",
			Kind:    "rolling_summary",
			Summary: rp.RollingSummary,
		})
	}
	for _, msg := range rp.ShortContext {
		items = append(items, RecallTraceItem{
			Layer:   "L0",
			Kind:    "short_context",
			Summary: msg.Msg,
		})
	}
	for _, summary := range rp.MidSummaries {
		items = append(items, RecallTraceItem{
			Layer:   "L2",
			Kind:    "thread_summary",
			Summary: summary.Summary,
		})
	}
	for _, fact := range rp.LongFacts {
		items = append(items, RecallTraceItem{
			Layer:   "L3",
			Kind:    "long_fact",
			Summary: fact,
		})
	}
	for _, snippet := range rp.KBSnippets {
		items = append(items, RecallTraceItem{
			Layer:   "L3",
			Kind:    "knowledge",
			Summary: snippet,
		})
	}
	for _, cache := range rp.SearchCacheSnippets {
		items = append(items, RecallTraceItem{
			Layer:       "L1",
			Kind:        "search_cache",
			Summary:     cache.ResultsJSON,
			Query:       cache.Query,
			Provider:    cache.Provider,
			SourceURLs:  append([]string(nil), cache.SourceURLs...),
			RetrievedAt: cache.RetrievedAt,
		})
	}
	return items
}
