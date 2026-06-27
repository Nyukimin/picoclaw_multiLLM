package idlechat

import (
	"context"
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/moviecatalog"
)

const movieContextMemoLimit = 6

func (o *IdleChatOrchestrator) enrichTopicContext(ctx context.Context, result TopicGenerationResult) TopicGenerationResult {
	if result.Category != TopicCategoryMovie {
		return result
	}
	if ctx == nil {
		ctx = context.Background()
	}
	o.mu.Lock()
	dbPath := o.movieCatalogDBPath
	o.mu.Unlock()
	memo, err := moviecatalog.LookupContextMemo(ctx, moviecatalog.ContextMemoOptions{
		DBPath: dbPath,
		Topic:  result.Topic,
		Genre:  result.Seed.Genre1,
		Limit:  movieContextMemoLimit,
	})
	if err != nil {
		log.Printf("[IdleChat] movie context memo lookup failed: %v", err)
		return result
	}
	if !memo.Available || len(memo.Terms) == 0 {
		if !memo.Available {
			log.Printf("[IdleChat] movie context memo unavailable: movie catalog DB not found")
		}
		return result
	}
	result.ContextTerms = mergeTopicContextTerms(result.ContextTerms, movieContextMemoTerms(memo.Terms)...)
	log.Printf("[IdleChat] movie context memo attached: terms=%d query=%q", len(result.ContextTerms), memo.Query)
	return result
}

func movieContextMemoTerms(terms []moviecatalog.ContextMemoTerm) []TopicContextTerm {
	out := make([]TopicContextTerm, 0, len(terms))
	for _, term := range terms {
		if strings.TrimSpace(term.Term) == "" {
			continue
		}
		out = append(out, TopicContextTerm{
			Term:      strings.TrimSpace(term.Term),
			Meaning:   strings.TrimSpace(term.Meaning),
			Relevance: strings.TrimSpace(term.Relevance),
			Source:    strings.TrimSpace(term.Source),
		})
	}
	return out
}

func mergeTopicContextTerms(base []TopicContextTerm, extra ...TopicContextTerm) []TopicContextTerm {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]TopicContextTerm, 0, len(base)+len(extra))
	for _, term := range append(base, extra...) {
		key := strings.ToLower(strings.TrimSpace(term.Source) + ":" + strings.TrimSpace(term.Term))
		if strings.TrimSpace(term.Term) == "" || key == ":" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
	}
	return out
}
