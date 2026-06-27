package idlechat

import "testing"

func TestSimpleStoryTopicStockKeepsTenTopics(t *testing.T) {
	stock := newSimpleStoryTopicStock()
	for i := 0; i < simpleStoryTopicStockSize+3; i++ {
		tale := simpleStoryTales[i%len(simpleStoryTales)]
		protagonist := protagonistOptions[i%len(protagonistOptions)]
		stock.push(simpleStoryPreparedTopic{
			Tale:        tale,
			Protagonist: protagonist,
			Result:      buildSimpleStoryTopicResult(tale.title, protagonist),
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

func TestInitSimpleStoryTopicStockFillsTenTopics(t *testing.T) {
	orch := NewIdleChatOrchestrator(nil, nil, []string{"mio", "shiro"}, 1, 1, 0.7, nil)
	defer orch.Stop()

	orch.InitSimpleStoryTopicStock()
	orch.mu.Lock()
	stock := orch.simpleStoryTopicStock
	orch.mu.Unlock()
	if stock == nil {
		t.Fatal("simpleStoryTopicStock = nil")
	}
	if got := stock.count(); got != simpleStoryTopicStockSize {
		t.Fatalf("stock.count() = %d, want %d", got, simpleStoryTopicStockSize)
	}
}
