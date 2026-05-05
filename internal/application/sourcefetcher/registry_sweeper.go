package sourcefetcher

import (
	"context"
	"fmt"
	"strings"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	"github.com/mmcdole/gofeed"
)

type RegistryStore interface {
	DueSourceRegistryEntries(ctx context.Context, now time.Time) ([]conversationpersistence.L1SourceRegistryEntry, error)
	SourceTrustScores(ctx context.Context) (map[string]float64, error)
	StageSourceRegistryFetch(ctx context.Context, sourceID string, payload conversationpersistence.L1SourceFetchPayload) (*conversationpersistence.L1StagingItem, error)
	ValidateStagingItem(ctx context.Context, id string, policy conversationpersistence.L1StagingValidationPolicy) (*conversationpersistence.L1StagingValidationResult, error)
	PromoteValidatedStagingItemToNews(ctx context.Context, id string, category string) (*conversationpersistence.L1NewsItem, error)
	MarkSourceRegistryFetched(ctx context.Context, sourceID string, fetchedAt time.Time, status string, lastError string) error
}

type SweepOptions struct {
	LimitPerSource    int
	MinimumTrustScore float64
}

type SweepResult struct {
	Sources      int
	Staged       int
	Validated    int
	PromotedNews int
	Failed       int
}

func SweepDueSources(ctx context.Context, store RegistryStore, now time.Time, opts SweepOptions) (SweepResult, error) {
	if store == nil {
		return SweepResult{}, fmt.Errorf("source registry store is nil")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if opts.LimitPerSource <= 0 {
		opts.LimitPerSource = 10
	}
	sources, err := store.DueSourceRegistryEntries(ctx, now)
	if err != nil {
		return SweepResult{}, err
	}
	trustScores, err := store.SourceTrustScores(ctx)
	if err != nil {
		return SweepResult{}, err
	}
	result := SweepResult{Sources: len(sources)}
	parser := gofeed.NewParser()
	for _, source := range sources {
		if source.Kind != conversationpersistence.L1SourceKindRSS && source.Kind != conversationpersistence.L1SourceKindAtom {
			continue
		}
		if err := sweepFeedSource(ctx, store, parser, source, trustScores, now, opts, &result); err != nil {
			result.Failed++
			_ = store.MarkSourceRegistryFetched(ctx, source.SourceID, now, "error", err.Error())
			continue
		}
		if err := store.MarkSourceRegistryFetched(ctx, source.SourceID, now, "ok", ""); err != nil {
			return result, err
		}
	}
	return result, nil
}

func sweepFeedSource(ctx context.Context, store RegistryStore, parser *gofeed.Parser, source conversationpersistence.L1SourceRegistryEntry, trustScores map[string]float64, now time.Time, opts SweepOptions, result *SweepResult) error {
	feed, err := parser.ParseURLWithContext(source.URL, ctx)
	if err != nil {
		return err
	}
	category := stringFromMeta(source.Meta, "category", "general")
	namespace := stringFromMeta(source.Meta, "namespace", "kb:news")
	limit := opts.LimitPerSource
	for i, item := range feed.Items {
		if i >= limit {
			break
		}
		raw := strings.TrimSpace(strings.Join(nonEmpty(item.Title, item.Description, item.Content), "\n"))
		if raw == "" {
			continue
		}
		publishedAt := now
		if item.PublishedParsed != nil {
			publishedAt = item.PublishedParsed.UTC()
		}
		staged, err := store.StageSourceRegistryFetch(ctx, source.SourceID, conversationpersistence.L1SourceFetchPayload{
			SourceURL:    firstNonEmpty(item.Link, source.URL),
			FetchedAt:    now,
			PublishedAt:  publishedAt,
			RawText:      raw,
			SummaryDraft: strings.TrimSpace(item.Title),
			Keywords:     []string{category},
			Meta: map[string]interface{}{
				"fetcher":   "source_registry",
				"category":  category,
				"namespace": namespace,
			},
		})
		if err != nil {
			return err
		}
		result.Staged++
		validation, err := store.ValidateStagingItem(ctx, staged.ID, conversationpersistence.L1StagingValidationPolicy{
			SourceTrustScores: trustScores,
			MinimumTrustScore: opts.MinimumTrustScore,
			Now:               now,
		})
		if err != nil {
			return err
		}
		if !validation.Passed {
			continue
		}
		result.Validated++
		if _, err := store.PromoteValidatedStagingItemToNews(ctx, staged.ID, category); err != nil {
			return err
		}
		result.PromotedNews++
	}
	return nil
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromMeta(meta map[string]interface{}, key string, def string) string {
	if meta == nil {
		return def
	}
	if value, ok := meta[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return def
}
