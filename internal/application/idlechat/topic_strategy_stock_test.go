package idlechat

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

type countingTopicStockProvider struct {
	calls atomic.Int32
}

func (p *countingTopicStockProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	p.calls.Add(1)
	return llm.GenerateResponse{}, fmt.Errorf("unexpected provider call")
}

func (p *countingTopicStockProvider) Name() string {
	return "counting-topic-stock"
}

func TestTopicStrategyStockKeepsTenTopicsPerStrategy(t *testing.T) {
	stock := newTopicStrategyStock()
	for _, strategy := range stockableTopicStrategies {
		for i := 0; i < topicStrategyStockSize+3; i++ {
			stock.push(strategy, TopicGenerationResult{
				Topic:    fmt.Sprintf("%s-topic-%02d", strategy, i),
				Category: categoryForTestStrategy(strategy),
				Strategy: string(strategy),
			})
		}
		if got := stock.count(strategy); got != topicStrategyStockSize {
			t.Fatalf("%s stock.count() = %d, want %d", strategy, got, topicStrategyStockSize)
		}

		first := stock.pop(strategy)
		if first == nil || first.Topic != fmt.Sprintf("%s-topic-00", strategy) {
			t.Fatalf("%s stock.pop() = %#v, want first topic", strategy, first)
		}
		if got := stock.count(strategy); got != topicStrategyStockSize-1 {
			t.Fatalf("%s stock.count() after pop = %d, want %d", strategy, got, topicStrategyStockSize-1)
		}
	}
}

func TestGenerateTopicUsesStrategyStockWithoutProviderCall(t *testing.T) {
	for _, strategy := range stockableTopicStrategies {
		t.Run(string(strategy), func(t *testing.T) {
			provider := &countingTopicStockProvider{}
			orch := NewIdleChatOrchestrator(provider, nil, []string{"mio", "shiro"}, 1, 1, 0.7, nil)
			defer orch.Stop()

			topic := fmt.Sprintf("%s stocked topic", strategy)
			if strategy == StrategyMovie {
				topic = formatMovieTopicPrompt("stocked movie")
			}
			orch.mu.Lock()
			orch.chatActive = true
			orch.topicStrategyStock = newTopicStrategyStock()
			orch.topicStrategyStock.push(strategy, TopicGenerationResult{
				Topic:               topic,
				Category:            categoryForTestStrategy(strategy),
				Strategy:            string(strategy),
				InterestingnessAxis: "axis",
				OpeningHook:         "hook",
				Avoid:               "avoid",
			})
			orch.mu.Unlock()

			gotTopic, gotStrategy := orch.generateTopicFromChat("idle-test-topic-00", strategy)
			if gotStrategy != strategy {
				t.Fatalf("strategy = %q, want %q", gotStrategy, strategy)
			}
			if gotTopic != topic {
				t.Fatalf("topic = %q, want stocked topic %q", gotTopic, topic)
			}
			if got := provider.calls.Load(); got != 0 {
				t.Fatalf("provider calls = %d, want 0", got)
			}
		})
	}
}

func categoryForTestStrategy(strategy TopicStrategy) TopicCategory {
	switch strategy {
	case StrategySingleGenre:
		return TopicCategorySingle
	case StrategyDoubleGenre:
		return TopicCategoryDouble
	case StrategyExternalStimulus:
		return TopicCategoryExternal
	case StrategyMovie:
		return TopicCategoryMovie
	case StrategyNews:
		return TopicCategoryNews
	default:
		return TopicCategorySingle
	}
}
