package idlechat

import (
	"log"
	"math/rand"
	"strings"
	"sync"
)

const simpleStoryTopicStockSize = 10

func SimpleStoryTopicStockTarget() int {
	return simpleStoryTopicStockSize
}

type simpleStoryPreparedTopic struct {
	Tale        simpleStoryTale
	Protagonist string
	Result      TopicGenerationResult
}

type simpleStoryTopicStock struct {
	mu    sync.Mutex
	items []simpleStoryPreparedTopic
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
	if s == nil || strings.TrimSpace(item.Result.Topic) == "" {
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

func (o *IdleChatOrchestrator) InitSimpleStoryTopicStock() {
	o.mu.Lock()
	if o.simpleStoryTopicStock == nil {
		o.simpleStoryTopicStock = newSimpleStoryTopicStock()
	}
	o.mu.Unlock()
	o.refillSimpleStoryTopicStock()
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
		log.Printf("[SimpleStory] topic popped from stock (remaining=%d)", stock.count())
		o.refillSimpleStoryTopicStock()
	}
	return item
}

func (o *IdleChatOrchestrator) refillSimpleStoryTopicStock() {
	o.mu.Lock()
	if o.simpleStoryTopicStock == nil {
		o.simpleStoryTopicStock = newSimpleStoryTopicStock()
	}
	stock := o.simpleStoryTopicStock
	o.mu.Unlock()
	for _, item := range shuffledSimpleStoryPreparedTopics() {
		if stock.count() >= simpleStoryTopicStockSize {
			break
		}
		stock.push(item)
	}
	log.Printf("[SimpleStory] topic stock ready (count=%d/%d)", stock.count(), simpleStoryTopicStockSize)
}

func buildSimpleStoryPreparedTopic() *simpleStoryPreparedTopic {
	tale := simpleStoryTales[rand.Intn(len(simpleStoryTales))]
	protagonist := protagonistOptions[rand.Intn(len(protagonistOptions))]
	return &simpleStoryPreparedTopic{
		Tale:        tale,
		Protagonist: protagonist,
		Result:      buildSimpleStoryTopicResult(tale.title, protagonist),
	}
}

func shuffledSimpleStoryPreparedTopics() []simpleStoryPreparedTopic {
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

func simpleStoryTopicKey(item simpleStoryPreparedTopic) string {
	taleTitle := strings.TrimSpace(item.Tale.title)
	protagonist := strings.TrimSpace(item.Protagonist)
	if taleTitle == "" || protagonist == "" {
		return normalizeLoopText(item.Result.Topic)
	}
	return normalizeLoopText(taleTitle + "\n" + protagonist)
}
