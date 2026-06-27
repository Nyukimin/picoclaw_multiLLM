package idlechat

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const simpleStoryTopicStockSize = 10
const simpleStoryTopicStockRefillLimit = simpleStoryTopicStockSize * 3

func SimpleStoryTopicStockTarget() int {
	return simpleStoryTopicStockSize
}

type simpleStoryPreparedTopic struct {
	Tale          simpleStoryTale
	Protagonist   string
	Result        TopicGenerationResult
	StoryTitle    string
	StoryText     string
	QualityReview string
	RevisionNote  string
}

type simpleStoryTopicStock struct {
	mu      sync.Mutex
	items   []simpleStoryPreparedTopic
	filling bool
}

func newSimpleStoryTopicStock() *simpleStoryTopicStock {
	return &simpleStoryTopicStock{}
}

func (s *simpleStoryTopicStock) pop() *simpleStoryPreparedTopic {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) == 0 {
		return nil
	}
	item := s.items[0]
	usedKey := simpleStoryTopicKey(item)
	remaining := make([]simpleStoryPreparedTopic, 0, len(s.items)-1)
	for _, candidate := range s.items[1:] {
		if usedKey != "" && simpleStoryTopicKey(candidate) == usedKey {
			continue
		}
		remaining = append(remaining, candidate)
	}
	s.items = remaining
	return &item
}

func (s *simpleStoryTopicStock) push(item simpleStoryPreparedTopic) bool {
	if s == nil || strings.TrimSpace(item.Result.Topic) == "" || strings.TrimSpace(item.StoryText) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.items) >= simpleStoryTopicStockSize {
		return false
	}
	itemKey := simpleStoryTopicKey(item)
	for _, existing := range s.items {
		if itemKey != "" && simpleStoryTopicKey(existing) == itemKey {
			return false
		}
	}
	s.items = append(s.items, item)
	return true
}

func (s *simpleStoryTopicStock) count() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items)
}

func (s *simpleStoryTopicStock) startFilling() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filling {
		return false
	}
	s.filling = true
	return true
}

func (s *simpleStoryTopicStock) doneFilling() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.filling = false
	s.mu.Unlock()
}

func (o *IdleChatOrchestrator) InitSimpleStoryTopicStock() {
	o.mu.Lock()
	if o.simpleStoryTopicStock == nil {
		o.simpleStoryTopicStock = newSimpleStoryTopicStock()
	}
	o.mu.Unlock()
	o.refillSimpleStoryTopicStockAsync()
}

func (o *IdleChatOrchestrator) popSimpleStoryTopicStock() *simpleStoryPreparedTopic {
	o.mu.Lock()
	stock := o.simpleStoryTopicStock
	o.mu.Unlock()
	if stock == nil {
		return nil
	}
	item := stock.pop()
	if item != nil {
		log.Printf("[SimpleStory] completed story popped from stock (remaining=%d)", stock.count())
		o.refillSimpleStoryTopicStockAsync()
	}
	return item
}

func (o *IdleChatOrchestrator) refillSimpleStoryTopicStockAsync() {
	o.mu.Lock()
	if o.simpleStoryTopicStock == nil {
		o.simpleStoryTopicStock = newSimpleStoryTopicStock()
	}
	stock := o.simpleStoryTopicStock
	o.mu.Unlock()
	if stock.count() >= simpleStoryTopicStockSize || !stock.startFilling() {
		return
	}
	ctx := o.ctx
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		defer stock.doneFilling()
		o.refillSimpleStoryTopicStockSync(ctx, stock)
	}()
}

func (o *IdleChatOrchestrator) refillSimpleStoryTopicStockSync(ctx context.Context, stock *simpleStoryTopicStock) {
	if ctx == nil {
		ctx = context.Background()
	}
	attempts := 0
	for _, seed := range shuffledSimpleStoryPreparedSeeds() {
		if stock.count() >= simpleStoryTopicStockSize || attempts >= simpleStoryTopicStockRefillLimit {
			break
		}
		if !o.waitForTopicStrategyStockIdle(ctx) {
			return
		}
		o.topicStockFillMu.Lock()
		if !o.canGenerateTopicStrategyStock() {
			o.topicStockFillMu.Unlock()
			continue
		}
		attempts++
		item, err := o.generateSimpleStoryPreparedTopic(ctx, seed.Tale, seed.Protagonist)
		o.topicStockFillMu.Unlock()
		if err != nil {
			log.Printf("[SimpleStory] completed story stock refill skipped: %v", err)
			continue
		}
		if stock.push(*item) {
			log.Printf("[SimpleStory] completed story stock refilled (count=%d/%d)", stock.count(), simpleStoryTopicStockSize)
		}
	}
	log.Printf("[SimpleStory] completed story stock ready (count=%d/%d)", stock.count(), simpleStoryTopicStockSize)
}

func (o *IdleChatOrchestrator) buildSimpleStoryPreparedTopic(ctx context.Context) (*simpleStoryPreparedTopic, error) {
	tale := simpleStoryTales[rand.Intn(len(simpleStoryTales))]
	protagonist := protagonistOptions[rand.Intn(len(protagonistOptions))]
	return o.generateSimpleStoryPreparedTopic(ctx, tale, protagonist)
}

func shuffledSimpleStoryPreparedSeeds() []simpleStoryPreparedTopic {
	items := make([]simpleStoryPreparedTopic, 0, len(simpleStoryTales)*len(protagonistOptions))
	for _, tale := range simpleStoryTales {
		for _, protagonist := range protagonistOptions {
			items = append(items, simpleStoryPreparedTopic{
				Tale:        tale,
				Protagonist: protagonist,
				Result:      buildSimpleStoryTopicResult(tale.title, protagonist),
			})
		}
	}
	rand.Shuffle(len(items), func(i, j int) {
		items[i], items[j] = items[j], items[i]
	})
	return items
}

func (o *IdleChatOrchestrator) waitForSimpleStoryStock(ctx context.Context, timeout time.Duration) *simpleStoryPreparedTopic {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if item := o.popSimpleStoryTopicStock(); item != nil {
			return item
		}
		select {
		case <-ctx.Done():
			return nil
		case <-deadline.C:
			return nil
		case <-ticker.C:
		}
	}
}

func simpleStoryTopicKey(item simpleStoryPreparedTopic) string {
	taleTitle := strings.TrimSpace(item.Tale.title)
	protagonist := strings.TrimSpace(item.Protagonist)
	if taleTitle == "" || protagonist == "" {
		return normalizeLoopText(item.Result.Topic)
	}
	storyTitle := strings.TrimSpace(item.StoryTitle)
	if storyTitle == "" {
		return normalizeLoopText(taleTitle + "\n" + protagonist)
	}
	return normalizeLoopText(taleTitle + "\n" + protagonist + "\n" + storyTitle)
}

func requireCompleteSimpleStory(item *simpleStoryPreparedTopic) error {
	if item == nil {
		return fmt.Errorf("story-simple completed stock unavailable")
	}
	if strings.TrimSpace(item.StoryText) == "" {
		return fmt.Errorf("story-simple stock item has no completed body")
	}
	return nil
}
