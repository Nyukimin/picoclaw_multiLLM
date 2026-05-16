package idlechat

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

// PreparedTopic は事前生成済みのお題。
type PreparedTopic struct {
	Domain  ForecastDomain `json:"domain"`
	Topic   string         `json:"topic"`
	Seeds   []string       `json:"seeds"`
	Created time.Time      `json:"created"`
}

// forecastTopicStock はドメインごとのお題バッファ（ファイル永続化付き）。
type forecastTopicStock struct {
	mu      sync.Mutex
	stock   map[string][]PreparedTopic
	filling map[string]bool
	path    string // 永続化ファイルパス
}

// stockFile はファイル保存形式。
type stockFile struct {
	Stock map[string][]PreparedTopic `json:"stock"`
}

func newForecastTopicStock(path string) *forecastTopicStock {
	s := &forecastTopicStock{
		stock:   make(map[string][]PreparedTopic),
		filling: make(map[string]bool),
		path:    path,
	}
	s.load()
	return s
}

func (s *forecastTopicStock) load() {
	if s.path == "" {
		log.Printf("[Forecast] Stock file path is empty, skipping load")
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		log.Printf("[Forecast] Stock file not found or unreadable (%s): %v", s.path, err)
		return
	}
	var f stockFile
	if err := json.Unmarshal(data, &f); err != nil {
		log.Printf("[Forecast] Stock file parse error: %v", err)
		return
	}
	if f.Stock == nil || len(f.Stock) == 0 {
		log.Printf("[Forecast] Stock file empty or nil")
		return
	}
	s.stock = f.Stock
	total := 0
	for _, items := range s.stock {
		total += len(items)
	}
	log.Printf("[Forecast] Stock loaded from file: %d topics across %d domains", total, len(f.Stock))
}

func (s *forecastTopicStock) save() {
	if s.path == "" {
		return
	}
	f := stockFile{Stock: s.stock}
	data, err := json.Marshal(f)
	if err != nil {
		log.Printf("[Forecast] Stock file marshal error: %v", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("[Forecast] Stock file write error: %v", err)
	}
}

// pop はドメインのストックから1つ取得する。空なら nil。
func (s *forecastTopicStock) pop(domain string) *PreparedTopic {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.stock[domain]
	if len(items) == 0 {
		return nil
	}
	item := items[0]
	usedKey := normalizeLoopText(item.Topic)
	remaining := make([]PreparedTopic, 0, len(items)-1)
	for _, candidate := range items[1:] {
		if usedKey != "" && normalizeLoopText(candidate.Topic) == usedKey {
			continue
		}
		remaining = append(remaining, candidate)
	}
	s.stock[domain] = remaining
	s.save()
	return &item
}

// push はドメインのストックに追加する（上限 forecastTopicStockSize）。
func (s *forecastTopicStock) push(domain string, item PreparedTopic) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(item.Topic) == "" {
		return
	}
	items := s.stock[domain]
	itemKey := normalizeLoopText(item.Topic)
	for _, existing := range items {
		if itemKey != "" && normalizeLoopText(existing.Topic) == itemKey {
			return
		}
	}
	if len(items) >= forecastTopicStockSize {
		return
	}
	s.stock[domain] = append(items, item)
	s.save()
}

// count はドメインのストック数を返す。
func (s *forecastTopicStock) count(domain string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.stock[domain])
}

// startFilling は重複生成を防止しつつ filling フラグを立てる。既に filling なら false。
func (s *forecastTopicStock) startFilling(domain string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.filling[domain] {
		return false
	}
	s.filling[domain] = true
	return true
}

func (s *forecastTopicStock) doneFilling(domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filling[domain] = false
}

// InitForecastTopicStock はお題ストックを初期化し、全ドメインのバックグラウンド補充を開始する。
// path はストックの永続化ファイルパス。
func (o *IdleChatOrchestrator) InitForecastTopicStock(path string) {
	o.mu.Lock()
	if o.topicStockBuf != nil {
		o.mu.Unlock()
		return
	}
	o.topicStockBuf = newForecastTopicStock(path)
	o.mu.Unlock()
	log.Printf("[Forecast] Topic stock initialized, starting background fill for %d domains", len(forecastDomains))
	for _, domain := range forecastDomains {
		o.refillTopicStockAsync(domain)
	}
}

// popForecastTopic はストックからお題を取得し、バックグラウンドで補充をトリガーする。
// ストックが空の場合はインラインで生成する（フォールバック）。
func (o *IdleChatOrchestrator) popForecastTopic(domain ForecastDomain) (string, []string) {
	o.mu.Lock()
	stock := o.topicStockBuf
	o.mu.Unlock()

	if stock != nil {
		if item := stock.pop(domain.Name); item != nil {
			topic := normalizeForecastDisplayTopic(domain, item.Topic)
			if strings.TrimSpace(item.Topic) == "" {
				log.Printf("[Forecast] Empty topic popped from stock: %s, falling back", domain.Name)
			} else {
				log.Printf("[Forecast] Topic popped from stock: %s (remaining=%d)", domain.Name, stock.count(domain.Name))
				o.refillTopicStockAsync(domain)
				return topic, item.Seeds
			}
		}
	}

	// ストック空 → インライン生成（フォールバック）
	log.Printf("[Forecast] Stock empty for %s, generating inline", domain.Name)
	topic, seeds := o.generateForecastTopicInline(domain)
	return normalizeForecastDisplayTopic(domain, topic), seeds
}

// refillTopicStockAsync はバックグラウンドでストックを forecastTopicStockSize まで補充する。
func (o *IdleChatOrchestrator) refillTopicStockAsync(domain ForecastDomain) {
	o.mu.Lock()
	stock := o.topicStockBuf
	o.mu.Unlock()
	if stock == nil {
		return
	}

	if stock.count(domain.Name) >= forecastTopicStockSize {
		return
	}
	if !stock.startFilling(domain.Name) {
		return // 別の goroutine が補充中
	}
	go func(d ForecastDomain) {
		defer stock.doneFilling(d.Name)
		topic, seeds := o.generateForecastTopicInline(d)
		if topic != "" {
			stock.push(d.Name, PreparedTopic{
				Domain:  d,
				Topic:   topic,
				Seeds:   seeds,
				Created: time.Now(),
			})
			log.Printf("[Forecast] Stock refilled: %s (count=%d)", d.Name, stock.count(d.Name))
		}
		// まだ足りなければ再帰的に補充
		o.refillTopicStockAsync(d)
	}(domain)
}
