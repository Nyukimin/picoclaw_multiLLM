package idlechat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestSimpleStoryTopicStockKeepsTenTopics(t *testing.T) {
	stock := newSimpleStoryTopicStock()
	for i := 0; i < simpleStoryTopicStockSize+3; i++ {
		tale := simpleStoryTales[i%len(simpleStoryTales)]
		protagonist := protagonistOptions[i%len(protagonistOptions)]
		stock.push(simpleStoryPreparedTopic{
			Tale:        tale,
			Protagonist: protagonist,
			Result:      buildSimpleStoryTopicResult(tale.title, protagonist),
			StoryTitle:  "完成済み物語",
			StoryText:   completedSimpleStoryTestBody(protagonist),
		})
	}
	if got := stock.count(); got != simpleStoryTopicStockSize {
		t.Fatalf("stock.count() = %d, want %d", got, simpleStoryTopicStockSize)
	}

	first := stock.pop()
	if first == nil {
		t.Fatal("stock.pop() = nil, want first story topic")
	}
	if got := stock.count(); got != simpleStoryTopicStockSize-1 {
		t.Fatalf("stock.count() after pop = %d, want %d", got, simpleStoryTopicStockSize-1)
	}
}

func TestSimpleStoryTopicStockRejectsTopicOnlyItems(t *testing.T) {
	stock := newSimpleStoryTopicStock()
	if stock.push(simpleStoryPreparedTopic{
		Tale:        simpleStoryTales[0],
		Protagonist: protagonistOptions[0],
		Result:      buildSimpleStoryTopicResult(simpleStoryTales[0].title, protagonistOptions[0]),
	}) {
		t.Fatal("topic-only story-simple item should not enter completed stock")
	}
}

func TestInitSimpleStoryTopicStockFillsTenCompletedStoriesByWorker(t *testing.T) {
	provider := &storyStockTestProvider{}
	orch := NewIdleChatOrchestrator(provider, nil, []string{"mio", "shiro"}, 1, 1, 0.7, nil)
	defer orch.Stop()

	orch.InitSimpleStoryTopicStock()
	stock := waitForSimpleStoryStockCount(t, orch, simpleStoryTopicStockSize)
	if stock == nil {
		t.Fatal("simpleStoryTopicStock = nil")
	}
	if got := stock.count(); got != simpleStoryTopicStockSize {
		t.Fatalf("stock.count() = %d, want %d", got, simpleStoryTopicStockSize)
	}
	item := stock.pop()
	if item == nil || strings.TrimSpace(item.StoryText) == "" {
		t.Fatalf("stock item should contain completed story body: %#v", item)
	}
	if provider.calls < simpleStoryTopicStockSize*2 {
		t.Fatalf("worker calls = %d, want at least generation+quality per story", provider.calls)
	}
}

func waitForSimpleStoryStockCount(t *testing.T, orch *IdleChatOrchestrator, want int) *simpleStoryTopicStock {
	t.Helper()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		orch.mu.Lock()
		stock := orch.simpleStoryTopicStock
		orch.mu.Unlock()
		if stock != nil && stock.count() >= want {
			return stock
		}
		select {
		case <-deadline:
			if stock != nil {
				return stock
			}
			return nil
		case <-ticker.C:
		}
	}
}

type storyStockTestProvider struct {
	calls int
}

func (p *storyStockTestProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	p.calls++
	last := ""
	if len(req.Messages) > 0 {
		last = req.Messages[len(req.Messages)-1].Content
	}
	if strings.Contains(last, "出力形式:") || strings.Contains(last, "判定基準:") {
		return llm.GenerateResponse{Content: "QUALITY: pass\nSCORE: 92\nISSUES:\n- なし\nREVISION_PROMPT:"}, nil
	}
	return llm.GenerateResponse{Content: "【完成済み物語】\n" + completedSimpleStoryTestBody(strings.Join(protagonistOptions, "と"))}, nil
}

func (p *storyStockTestProvider) Name() string {
	return "story-stock-test"
}

func completedSimpleStoryTestBody(protagonist string) string {
	return strings.Repeat(protagonist+"は村の困りごとを調べ、仲間の反応を変えながら事件を解決した。", 7) +
		"こうして事件は解決し、村には新しい祭りと少し変な教訓が残ったのでした。"
}
