package idlechat

import (
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func TestNextIdleSessionPlanCoversCanonicalNormalCategories(t *testing.T) {
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	var got []TopicStrategy
	for i := 0; i < 5; i++ {
		plan := o.nextIdleSessionPlanLocked()
		if plan.mode != "idle" {
			t.Fatalf("plan %d mode = %q, want idle", i, plan.mode)
		}
		got = append(got, plan.strategy)
	}

	want := []TopicStrategy{
		StrategySingleGenre,
		StrategyDoubleGenre,
		StrategyExternalStimulus,
		StrategyMovie,
		StrategyNews,
	}
	if len(got) != len(want) {
		t.Fatalf("strategies len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("strategy %d = %q, want %q; got=%v", i, got[i], want[i], got)
		}
	}
}

func TestMovieCategoryIsIndependentStrategy(t *testing.T) {
	prompt, genres, anchor := generateMoviePrompt()
	if len(genres) != 1 {
		t.Fatalf("movie genres len = %d, want 1", len(genres))
	}
	if strings.TrimSpace(anchor.Value) == "" {
		t.Fatalf("movie anchor is empty: %+v", anchor)
	}
	if !strings.Contains(prompt, "どんな映画？") {
		t.Fatalf("movie prompt does not require movie question: %s", prompt)
	}
	if strings.Contains(prompt, "News") || strings.Contains(prompt, "ニュース見出し") {
		t.Fatalf("movie prompt mixed news concerns: %s", prompt)
	}
}

func TestExternalPromptUsesWikipediaOnly(t *testing.T) {
	withDailySeedCache(t, &DailySeedCache{
		Date:           "2026-05-27",
		WikipediaSeeds: []string{"地下鉄博物館"},
		NewsSeeds:      []string{"政府が新制度を発表"},
		FetchedAt:      time.Now(),
	})

	prompt, source, ok := generateExternalPrompt()
	if !ok {
		t.Fatalf("external prompt unavailable: source=%q", source)
	}
	if !strings.HasPrefix(source, "Wikipedia:") {
		t.Fatalf("external source = %q, want Wikipedia", source)
	}
	if strings.Contains(prompt, "政府が新制度を発表") || strings.Contains(source, "News:") {
		t.Fatalf("external prompt used news seed: source=%q prompt=%s", source, prompt)
	}
	if strings.Contains(prompt, "Wikipedia") || strings.Contains(prompt, "外部刺激") || strings.Contains(prompt, "偶然の記事") {
		t.Fatalf("external prompt leaks provider/mechanism wording: source=%q prompt=%s", source, prompt)
	}
	if !strings.Contains(prompt, "素材: 地下鉄博物館") {
		t.Fatalf("external prompt does not expose concrete seed as material: %s", prompt)
	}
}

func TestNewsPromptUsesNewsSeedWithoutGenreMixing(t *testing.T) {
	withDailySeedCache(t, &DailySeedCache{
		Date:           "2026-05-27",
		WikipediaSeeds: []string{"地下鉄博物館"},
		NewsSeeds:      []string{"新しい医療制度の検討が始まる"},
		FetchedAt:      time.Now(),
	})

	prompt, source, ok := generateNewsPrompt()
	if !ok {
		t.Fatalf("news prompt unavailable: source=%q", source)
	}
	if source != "News:新しい医療制度の検討が始まる" {
		t.Fatalf("news source = %q", source)
	}
	if strings.Contains(prompt, "組み合わせジャンル") || strings.Contains(prompt, "外部刺激") {
		t.Fatalf("news prompt mixed external/genre contract: %s", prompt)
	}
	if !strings.Contains(prompt, "ニュース見出し") || !strings.Contains(prompt, "新しい医療制度の検討が始まる") {
		t.Fatalf("news prompt does not focus on headline: %s", prompt)
	}
}

func TestNewsPromptUnavailableIsExplicit(t *testing.T) {
	withDailySeedCache(t, &DailySeedCache{
		Date:           "2026-05-27",
		WikipediaSeeds: []string{"地下鉄博物館"},
		NewsSeeds:      nil,
		FetchedAt:      time.Now(),
	})

	prompt, source, ok := generateNewsPrompt()
	if ok {
		t.Fatalf("news prompt ok = true, want false: prompt=%q source=%q", prompt, source)
	}
	if prompt != "" || source != "news_seed_unavailable" {
		t.Fatalf("unavailable news = prompt %q source %q", prompt, source)
	}
}

func withDailySeedCache(t *testing.T, cache *DailySeedCache) {
	t.Helper()
	cacheMu.Lock()
	old := dailyCache
	dailyCache = cache
	cacheMu.Unlock()
	t.Cleanup(func() {
		cacheMu.Lock()
		dailyCache = old
		cacheMu.Unlock()
	})
}
