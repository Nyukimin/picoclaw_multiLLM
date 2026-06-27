package idlechat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	topicStrategyStockSize        = 10
	topicStrategyStockIdlePoll    = 2 * time.Second
	topicStrategyStockRefillLimit = topicStrategyStockSize * 3
)

var stockableTopicStrategies = []TopicStrategy{
	StrategySingleGenre,
	StrategyDoubleGenre,
	StrategyExternalStimulus,
	StrategyMovie,
	StrategyNews,
}

func TopicStrategyStockTarget() int {
	return topicStrategyStockSize
}

func SingleTopicStockTarget() int {
	return topicStrategyStockSize
}

func DoubleTopicStockTarget() int {
	return topicStrategyStockSize
}

type topicStrategyStock struct {
	mu      sync.Mutex
	items   map[TopicStrategy][]TopicGenerationResult
	filling map[TopicStrategy]bool
}

func newTopicStrategyStock() *topicStrategyStock {
	return &topicStrategyStock{
		items:   make(map[TopicStrategy][]TopicGenerationResult),
		filling: make(map[TopicStrategy]bool),
	}
}

func (s *topicStrategyStock) pop(strategy TopicStrategy) *TopicGenerationResult {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items[strategy]
	if len(items) == 0 {
		return nil
	}
	item := items[0]
	usedKey := normalizeLoopText(item.Topic)
	remaining := make([]TopicGenerationResult, 0, len(items)-1)
	for _, candidate := range items[1:] {
		if usedKey != "" && normalizeLoopText(candidate.Topic) == usedKey {
			continue
		}
		remaining = append(remaining, candidate)
	}
	s.items[strategy] = remaining
	return &item
}

func (s *topicStrategyStock) push(strategy TopicStrategy, item TopicGenerationResult) bool {
	if s == nil || strings.TrimSpace(item.Topic) == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.items[strategy]
	if len(items) >= topicStrategyStockSize {
		return false
	}
	itemKey := normalizeLoopText(item.Topic)
	for _, existing := range items {
		if itemKey != "" && normalizeLoopText(existing.Topic) == itemKey {
			return false
		}
	}
	s.items[strategy] = append(items, item)
	return true
}

func (s *topicStrategyStock) count(strategy TopicStrategy) int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.items[strategy])
}

func (s *topicStrategyStock) startFilling(strategy TopicStrategy) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filling[strategy] {
		return false
	}
	s.filling[strategy] = true
	return true
}

func (s *topicStrategyStock) doneFilling(strategy TopicStrategy) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.filling[strategy] = false
	s.mu.Unlock()
}

func (o *IdleChatOrchestrator) InitTopicStrategyStocks() {
	o.mu.Lock()
	if o.topicStrategyStock == nil {
		o.topicStrategyStock = newTopicStrategyStock()
	}
	o.mu.Unlock()
	for _, strategy := range stockableTopicStrategies {
		o.refillTopicStrategyStockAsync(strategy)
	}
}

func (o *IdleChatOrchestrator) InitSingleTopicStock() {
	o.ensureTopicStrategyStock()
	o.refillTopicStrategyStockAsync(StrategySingleGenre)
}

func (o *IdleChatOrchestrator) InitDoubleTopicStock() {
	o.ensureTopicStrategyStock()
	o.refillTopicStrategyStockAsync(StrategyDoubleGenre)
}

func (o *IdleChatOrchestrator) ensureTopicStrategyStock() *topicStrategyStock {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.topicStrategyStock == nil {
		o.topicStrategyStock = newTopicStrategyStock()
	}
	return o.topicStrategyStock
}

func (o *IdleChatOrchestrator) popTopicStrategyStock(strategy TopicStrategy) *TopicGenerationResult {
	o.mu.Lock()
	stock := o.topicStrategyStock
	o.mu.Unlock()
	if stock == nil {
		return nil
	}
	result := stock.pop(strategy)
	if result != nil {
		log.Printf("[IdleChat] %s topic popped from stock (remaining=%d)", strategy, stock.count(strategy))
		o.refillTopicStrategyStockAsync(strategy)
	}
	return result
}

func (o *IdleChatOrchestrator) refillTopicStrategyStockAsync(strategy TopicStrategy) {
	if !isStockableTopicStrategy(strategy) {
		return
	}
	stock := o.ensureTopicStrategyStock()
	ctx := o.ctx
	if stock.count(strategy) >= topicStrategyStockSize || !stock.startFilling(strategy) {
		return
	}
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		defer stock.doneFilling(strategy)
		for attempts := 0; stock.count(strategy) < topicStrategyStockSize && attempts < topicStrategyStockRefillLimit; attempts++ {
			if !o.waitForTopicStrategyStockIdle(ctx) {
				return
			}
			o.topicStockFillMu.Lock()
			if !o.canGenerateTopicStrategyStock() {
				o.topicStockFillMu.Unlock()
				continue
			}
			before := stock.count(strategy)
			result, err := o.generateTopicStrategyStockItem(ctx, strategy)
			o.topicStockFillMu.Unlock()
			if err != nil {
				log.Printf("[IdleChat] %s topic stock refill skipped: %v", strategy, err)
				continue
			}
			if stock.push(strategy, *result) {
				log.Printf("[IdleChat] %s topic stock refilled (count=%d/%d)", strategy, stock.count(strategy), topicStrategyStockSize)
				continue
			}
			if stock.count(strategy) <= before {
				log.Printf("[IdleChat] %s topic stock produced duplicate topic (count=%d)", strategy, stock.count(strategy))
			}
		}
	}()
}

func (o *IdleChatOrchestrator) waitForTopicStrategyStockIdle(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if o.canGenerateTopicStrategyStock() {
			return true
		}
		timer := time.NewTimer(topicStrategyStockIdlePoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false
		case <-timer.C:
		}
	}
}

func (o *IdleChatOrchestrator) canGenerateTopicStrategyStock() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return !o.chatActive && !o.chatBusy && !o.workerBusy
}

func (o *IdleChatOrchestrator) generateTopicStrategyStockItem(ctx context.Context, strategy TopicStrategy) (*TopicGenerationResult, error) {
	seed, ok := buildTopicSeedForStrategy(strategy)
	if !ok {
		return nil, fmt.Errorf("%s topic seed unavailable", strategy)
	}
	recent := recentTopicRecords(o.getRecentTopics(12))
	o.mu.Lock()
	topicGenerationConfig := o.topicGenerationConfig
	o.mu.Unlock()
	if !topicGenerationConfig.Enabled {
		topicGenerationConfig.Enabled = true
	}
	if topicGenerationConfig.ProviderName == "" {
		topicGenerationConfig.ProviderName = "chatworker"
	}
	generator := NewTopicGenerator(o.providerForSpeaker("chatworker"), topicGenerationConfig)
	result, err := generator.GenerateInterestingTopic(ctx, seed.Category, seed, recent)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("%s topic generation returned nil result", strategy)
	}
	movieMode := strategy == StrategyMovie
	topic := normalizeIdleTopic(result.Topic, movieMode)
	if topic == "" {
		return nil, fmt.Errorf("%s topic generation returned empty topic", strategy)
	}
	copied := *result
	copied.Topic = topic
	copied.Category = seed.Category
	copied.Strategy = string(strategy)
	return &copied, nil
}

func isStockableTopicStrategy(strategy TopicStrategy) bool {
	switch strategy {
	case StrategySingleGenre, StrategyDoubleGenre, StrategyExternalStimulus, StrategyMovie, StrategyNews:
		return true
	default:
		return false
	}
}
