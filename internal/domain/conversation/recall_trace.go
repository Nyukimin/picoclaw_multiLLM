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
	Score       float32
	Decision    string
	Reason      string
	PromptIndex int
}

func (rp *RecallPack) ToTraceItems() []RecallTraceItem {
	if rp == nil {
		return nil
	}
	items := make([]RecallTraceItem, 0, len(rp.ShortContext)+len(rp.MidSummaries)+len(rp.LongFacts)+len(rp.KBSnippets)+len(rp.SearchCacheSnippets)+1)
	if rp.RollingSummary != "" {
		items = append(items, RecallTraceItem{
			Layer:       "L0",
			Kind:        "rolling_summary",
			Summary:     rp.RollingSummary,
			Decision:    "included",
			Reason:      "L0 rolling summary keeps older current-thread context compact",
			PromptIndex: len(items),
		})
	}
	for _, msg := range rp.ShortContext {
		items = append(items, RecallTraceItem{
			Layer:       "L0",
			Kind:        "short_context",
			Summary:     msg.Msg,
			Decision:    "included",
			Reason:      "recent L0 turn preserved as short context",
			PromptIndex: len(items),
		})
	}
	for _, summary := range rp.MidSummaries {
		items = append(items, RecallTraceItem{
			Layer:       "L2",
			Kind:        "thread_summary",
			Summary:     summary.Summary,
			Score:       summary.Score,
			Decision:    "included",
			Reason:      "role-filtered L2 thread summary selected for prompt",
			PromptIndex: len(items),
		})
	}
	for _, fact := range rp.LongFacts {
		items = append(items, RecallTraceItem{
			Layer:       "L3",
			Kind:        "long_fact",
			Summary:     fact,
			Decision:    "included",
			Reason:      "L3 long-term memory selected for prompt",
			PromptIndex: len(items),
		})
	}
	for _, snippet := range rp.KBSnippets {
		items = append(items, RecallTraceItem{
			Layer:       "L3",
			Kind:        "knowledge",
			Summary:     snippet,
			Decision:    "included",
			Reason:      "Knowledge DB snippet selected for prompt",
			PromptIndex: len(items),
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
			Decision:    "included",
			Reason:      "fresh L1 search cache hit selected for prompt",
			PromptIndex: len(items),
		})
	}
	return items
}
