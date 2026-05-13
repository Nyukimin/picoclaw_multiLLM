package idlechat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	forecastTurnsPerDomain     = 100 // 1ドメインあたりの最大ターン数
	forecastCheckpointInterval = 15  // 進行チェックポイントの間隔（ターン数）
	forecastTopicStockSize     = 2   // ドメインあたりのお題ストック数
	forecastSeedLimit          = 10
	forecastGoogleTrendLimit   = 2
)

// ForecastDomain は未来展望セッションの1ドメインを定義する。
type ForecastDomain struct {
	Name    string   // 表示名（例: "AI技術"）
	RSSURLs []string // NHK RSS カテゴリURL
}

var forecastDomains = []ForecastDomain{
	{
		Name:    "AI技術",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat2.xml"},
	},
	{
		Name:    "その他技術",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat2.xml"},
	},
	{
		Name:    "医療",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat7.xml", "https://www.nhk.or.jp/rss/news/cat2.xml"},
	},
	{
		Name:    "社会保障",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat7.xml", "https://www.nhk.or.jp/rss/news/cat1.xml"},
	},
	{
		Name:    "政治",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat3.xml"},
	},
	{
		Name:    "経済",
		RSSURLs: []string{"https://www.nhk.or.jp/rss/news/cat4.xml"},
	},
}

var forecastTopicAngles = []string{
	"制度・規制・ガバナンスの変化",
	"生活者の行動変化と受け止め方",
	"地方と都市の格差・インフラ影響",
	"雇用・教育・働き方の再編",
	"国際競争と地政学リスク",
	"コスト構造・投資・産業再編",
	"倫理・信頼・説明責任",
	"現場運用で起きる意外な副作用",
}

func shuffledForecastDomains() []ForecastDomain {
	out := append([]ForecastDomain(nil), forecastDomains...)
	rand.Shuffle(len(out), func(i, j int) {
		out[i], out[j] = out[j], out[i]
	})
	return out
}

// --- お題ストック ---

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

// generateForecastTopicInline は従来のインライン生成パイプライン。
func (o *IdleChatOrchestrator) generateForecastTopicInline(domain ForecastDomain) (string, []string) {
	trendSeeds := fetchTrendSeeds(domain)
	nhkSeeds := fetchDomainSeeds(domain, 10)
	allHeadlines := rankForecastSeeds(domain, append(trendSeeds, nhkSeeds...))
	keyword := o.extractForecastKeyword(domain, allHeadlines)
	deepSeeds := fetchGoogleNewsSeeds(keyword, 5)
	seeds := rankForecastSeeds(domain, append(allHeadlines, deepSeeds...))
	log.Printf("[Forecast] %s: keyword=%q trends=%d nhk=%d google=%d", domain.Name, keyword, len(trendSeeds), len(nhkSeeds), len(deepSeeds))
	topic := o.generateForecastTopic(domain, seeds)
	return topic, seeds
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

// fetchDomainSeeds は指定ドメインのRSSからシードを取得する。
func fetchDomainSeeds(domain ForecastDomain, limit int) []string {
	var all []string
	for _, rssURL := range domain.RSSURLs {
		headlines, err := fetchNewsHeadlinesFrom(rssURL, limit)
		if err != nil {
			log.Printf("[Forecast] RSS fetch failed (%s): %v", rssURL, err)
			continue
		}
		all = append(all, headlines...)
	}
	// 重複排除
	seen := make(map[string]struct{}, len(all))
	unique := make([]string, 0, len(all))
	for _, h := range all {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			unique = append(unique, h)
		}
	}
	if len(unique) > limit {
		rand.Shuffle(len(unique), func(i, j int) { unique[i], unique[j] = unique[j], unique[i] })
		unique = unique[:limit]
	}
	return unique
}

// generateForecastTopicPrompt はドメインとニュースシードから未来展望トピックのプロンプトを生成する。
func generateForecastTopicPrompt(domain ForecastDomain, seeds []string, avoidThemes []string) string {
	seedSection := ""
	if len(seeds) > 0 {
		picked := seeds
		if len(picked) > 5 {
			picked = pickRandom(seeds, 5)
		}
		seedSection = fmt.Sprintf("\n\n最新ニュース（%s）:\n- %s", domain.Name, strings.Join(picked, "\n- "))
	}
	angle := forecastTopicAngles[rand.Intn(len(forecastTopicAngles))]
	avoidSection := ""
	if len(avoidThemes) > 0 {
		picked := avoidThemes
		if len(picked) > 4 {
			picked = picked[:4]
		}
		avoidSection = fmt.Sprintf("\n\n避けたい既出テーマ:\n- %s", strings.Join(picked, "\n- "))
	}

	return fmt.Sprintf(`あなたは「%s」分野の未来を展望する議論のお題を1つ提案してください。%s%s

要件:
- 現在の動向・ニュースから3〜10年後の社会への影響を考えさせるお題
- 具体的な論点が含まれ、賛否両論が生まれるもの
- 「もし〜だったら」形式は使わない
- 楽観/悲観の両面から議論できるもの
- 今回は特に「%s」という切り口を強める
- 同じような論点の焼き直しを避け、切り口をずらす
- 既出テーマに近い案は避ける

回答はお題だけを1行で出力してください。
- 質問文・感想文は禁止
- 「%sの未来:」のような接頭辞は不要
- 体言止め、または「〜を考える」「〜の行方」のような題名調にする
- 50文字以内を目安に簡潔にする`, domain.Name, seedSection, avoidSection, angle, domain.Name)
}

// buildForecastLLMTopic は LLM に渡す詳細版トピックを構築する。
// 背景情報・ニュースシード・議論の方向性を含む。Viewer/TTS には使わない。
func buildForecastLLMTopic(domain ForecastDomain, displayTopic string, seeds []string) string {
	displayTopic = normalizeForecastDisplayTopic(domain, displayTopic)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s 未来展望】%s\n\n", domain.Name, displayTopic))
	if len(seeds) > 0 {
		sb.WriteString("背景情報（最新ニュース・トレンド）:\n")
		shown := seeds
		if len(shown) > 8 {
			shown = shown[:8]
		}
		for _, s := range shown {
			sb.WriteString("- ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(`議論の方向性:
- 上記の最新動向を踏まえ、3〜10年後の社会への具体的な影響を議論する
- 楽観/悲観の両面から、根拠のある主張を展開する
- 抽象論ではなく、具体的な事例・数字・影響を挙げる
- 過去の類似事例との比較や、国際的な視点も取り入れる`)
	return sb.String()
}

func normalizeForecastDisplayTopic(domain ForecastDomain, topic string) string {
	if normalized := normalizeIdleTopic(topic, false); normalized != "" {
		return normalized
	}
	name := strings.TrimSpace(domain.Name)
	if name == "" {
		name = "未来展望"
	}
	return fmt.Sprintf("%sの3年後を考える", name)
}

// RunForecastSession は6ドメインを順に回す未来展望セッションを実行する。
func (o *IdleChatOrchestrator) RunForecastSession() {
	sessionID := fmt.Sprintf("forecast-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	sessionDomains := append([]ForecastDomain(nil), forecastDomains...)

	log.Printf("[Forecast] Session %s started (%d domains, max %d turns/domain)", sessionID, len(sessionDomains), forecastTurnsPerDomain)

	o.mu.Lock()
	o.chatActive = true
	o.sessionMode = "forecast"
	o.mu.Unlock()

	totalTurns := o.runForecastSessionDomains(sessionID, startedAt, sessionDomains)

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.sessionContext = ""
	o.lastActivity = time.Now()
	o.mu.Unlock()
	log.Printf("[Forecast] Session %s completed (%d total turns)", sessionID, totalTurns)
}

func (o *IdleChatOrchestrator) runForecastDomainSession(domain ForecastDomain) {
	sessionID := fmt.Sprintf("forecast-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	totalTurns := o.runForecastSessionDomains(sessionID, startedAt, []ForecastDomain{domain})
	log.Printf("[Forecast] Session %s completed (%d total turns)", sessionID, totalTurns)
}

func (o *IdleChatOrchestrator) runForecastSessionDomains(sessionID string, startedAt time.Time, sessionDomains []ForecastDomain) int {
	totalTurns := 0

	for domainIdx, domain := range sessionDomains {
		select {
		case <-o.ctx.Done():
			return totalTurns
		default:
		}

		o.mu.Lock()
		if !o.chatActive {
			o.mu.Unlock()
			log.Printf("[Forecast] Session interrupted before domain %s", domain.Name)
			return totalTurns
		}
		o.mu.Unlock()

		// ドメインアナウンス
		announce := fmt.Sprintf("%sのテーマの時間です。", domain.Name)
		log.Printf("[Forecast] [Domain %d/%d] %s", domainIdx+1, len(sessionDomains), domain.Name)

		announceMsg := domaintransport.NewMessage("user", "mio", sessionID, "", announce)
		announceMsg.Type = domaintransport.MessageTypeIdleChat
		o.memory.RecordMessage(announceMsg)
		ttsDone := o.emitTimelineEvent(TimelineEvent{
			Type:      "idlechat.message",
			From:      "user",
			To:        "mio",
			Content:   announce,
			SessionID: sessionID,
		})
		o.waitForTTSDone(ttsDone)

		// ドメイン特化トピック生成: ストックから取得（空ならインライン生成）
		displayTopic, seeds := o.popForecastTopic(domain)
		llmTopic := buildForecastLLMTopic(domain, displayTopic, seeds)

		o.mu.Lock()
		o.currentTopic = fmt.Sprintf("[%s] %s", domain.Name, displayTopic)
		o.mu.Unlock()

		// Viewer/TTS にはシンプルなお題を表示
		topicAnnounce := fmt.Sprintf("お題は、%s", displayTopic)
		topicMsg := domaintransport.NewMessage("user", "mio", sessionID, "", topicAnnounce)
		topicMsg.Type = domaintransport.MessageTypeIdleChat
		o.memory.RecordMessage(topicMsg)
		ttsDone = o.emitTimelineEvent(TimelineEvent{
			Type:      "idlechat.message",
			From:      "user",
			To:        "mio",
			Content:   topicAnnounce,
			SessionID: sessionID,
		})
		o.waitForTTSDone(ttsDone)
		o.waitBreak(topicBreak)

		// ドメイン内ターンループ（generateResponse には詳細版 llmTopic を渡す）
		topic := displayTopic // saveSummary 用
		transcript := make([]string, 0, forecastTurnsPerDomain)
		coveredThemes := make([]string, 0, 8)
		currentSpeaker := o.chatSpeakerIndex()
		segmentTurns := 0
		loopReason := ""
		interrupted := false
		genFailed := false

		// ドメイン開始時に sessionContext をクリア
		o.mu.Lock()
		o.sessionContext = ""
		o.mu.Unlock()

		for turn := 0; turn < forecastTurnsPerDomain; turn++ {
			select {
			case <-o.ctx.Done():
				return totalTurns
			default:
			}

			o.mu.Lock()
			if !o.chatActive {
				o.mu.Unlock()
				interrupted = true
				loopReason = "interrupted"
				break
			}
			o.mu.Unlock()

			speaker := o.participants[currentSpeaker]
			nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

			// チェックポイント: 既出テーマを蓄積し sessionContext に反映
			if segmentTurns > 0 && segmentTurns%forecastCheckpointInterval == 0 {
				newThemes := o.extractCoveredThemes(domain, displayTopic, transcript, coveredThemes)
				if len(newThemes) > 0 {
					coveredThemes = append(coveredThemes, newThemes...)
					o.updateForecastSessionContext(domain, displayTopic, coveredThemes)
					log.Printf("[Forecast] Checkpoint at turn %d: covered themes now %d", segmentTurns, len(coveredThemes))
				}
			}

			// LLM には詳細な背景情報付きトピックを渡す
			response, err := o.generateResponse(speaker, nextSpeaker, sessionID, totalTurns+turn, segmentTurns, llmTopic)
			if err != nil {
				log.Printf("[Forecast] Generation error: %v", err)
				genFailed = true
				loopReason = "generation_error"
				break
			}
			if isResponseTooSimilar(response, transcript) {
				loopReason = "pre_emit_similarity"
				log.Printf("[Forecast] Repetitive response, moving to next domain")
				break
			}

			response = ensureTrailingPeriod(response)

			msg := domaintransport.NewMessage(speaker, nextSpeaker, sessionID, "", response)
			msg.Type = domaintransport.MessageTypeIdleChat
			o.memory.RecordMessage(msg)
			ttsDone := o.emitTimelineEvent(TimelineEvent{
				Type:      "idlechat.message",
				From:      speaker,
				To:        nextSpeaker,
				Content:   response,
				SessionID: sessionID,
			})
			transcript = append(transcript, fmt.Sprintf("%s: %s", speaker, response))
			segmentTurns++
			totalTurns++

			log.Printf("[Forecast] [%s Turn %d] %s→%s: %s", domain.Name, turn, speaker, nextSpeaker, truncate(response, 80))
			o.waitForTTSDone(ttsDone)
			o.waitBreak(speakerBreak)

			if reason := detectLoopReason(transcript); reason != "" {
				loopReason = reason
				log.Printf("[Forecast] Loop detected in %s, moving to next domain", domain.Name)
				break
			}
			currentSpeaker = (currentSpeaker + 1) % len(o.participants)
		}

		// ドメイン要約保存（Coder2で要約 + 継続考察テーマ付与）
		endedAt := time.Now().In(jst)
		if segmentTurns > 0 {
			summary := o.saveForecastSummary(sessionID, domain, topic, transcript, startedAt, endedAt, segmentTurns,
				interrupted || genFailed || loopReason != "", loopReason)
			o.speakSummary(sessionID, summary)
		}

		if interrupted {
			return totalTurns
		}

		// ドメイン間ブレイク（最後のドメイン以外）
		if domainIdx < len(sessionDomains)-1 {
			o.waitBreak(topicBreak)
		}
	}

	return totalTurns
}

// saveForecastSummary は Coder2 で要約+継続考察テーマを生成して保存する。
func (o *IdleChatOrchestrator) saveForecastSummary(sessionID string, domain ForecastDomain, topic string, transcript []string, startedAt, endedAt time.Time, turns int, loopRestarted bool, loopReason string) string {
	summary := o.summarizeByForecastLLM(domain, topic, transcript)
	summary = annotateLoopSummary(summary, loopRestarted, loopReason)
	fullTopic := fmt.Sprintf("[%s] %s", domain.Name, topic)
	qualityReview, promptGuidance := o.reviewSessionEnd(fullTopic, fmt.Sprintf("forecast/%s", domain.Name), transcript, summary, loopReason)
	title := fmt.Sprintf("%d月%d日の%sの話題まとめ", endedAt.Month(), endedAt.Day(), truncate(fullTopic, 24))
	record := SessionSummary{
		SessionID:       sessionID,
		Title:           title,
		Topic:           fullTopic,
		Strategy:        TopicStrategy(fmt.Sprintf("forecast/%s", domain.Name)),
		Summary:         summary,
		QualityReview:   qualityReview,
		PromptGuidance:  promptGuidance,
		StartedAt:       startedAt.Format(time.RFC3339),
		EndedAt:         endedAt.Format(time.RFC3339),
		Turns:           turns,
		LoopRestarted:   loopRestarted,
		LoopReason:      loopReason,
		TopicProvider:   "forecast",
		SummaryProvider: "shiro",
		Transcript:      append([]string(nil), transcript...),
	}
	o.mu.Lock()
	o.history = append(o.history, record)
	if len(o.history) > 200 {
		o.history = o.history[len(o.history)-200:]
	}
	o.addPromptGuideLocked(promptGuidance)
	store := o.topicStore
	o.mu.Unlock()
	if store != nil {
		if err := store.Append(record); err != nil {
			log.Printf("[Forecast] topic store append failed: %v", err)
		}
	}

	// タイムラインに要約を emit
	msg := domaintransport.NewMessage("shiro", "forecast_summary", sessionID, "", title+"\n"+summary)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.summary",
		From:      "shiro",
		To:        "forecast_summary",
		Content:   title + "\n" + summary,
		SessionID: sessionID,
	})
	return summary
}

// summarizeByForecastLLM は Coder2 で未来展望ディスカッションを要約し、継続考察テーマを付与する。
func (o *IdleChatOrchestrator) summarizeByForecastLLM(domain ForecastDomain, topic string, transcript []string) string {
	if len(transcript) == 0 {
		return "会話ログがありません。"
	}
	body := strings.Join(transcript, "\n")
	messages := []llm.Message{
		{Role: "system", Content: "あなたは未来予測・社会分析の専門家です。議論を的確に要約し、さらに深掘りすべき論点を提示してください。"},
		{Role: "user", Content: fmt.Sprintf(`以下は「%s」分野の未来展望ディスカッションです。

話題: %s

%s

以下の形式で要約してください:

## 議論の要約
- 主要な論点と結論を3〜5点で簡潔に

## 注目すべき視点
- 議論の中で特に鋭かった指摘や新しい切り口を1〜2点

## 継続考察テーマ
この議論を踏まえて、次に掘り下げるべきテーマを3つ提案してください:
1. （テーマ名）: 一行説明
2. （テーマ名）: 一行説明
3. （テーマ名）: 一行説明`, domain.Name, topic, body)},
	}
	req := llm.GenerateRequest{Messages: messages, MaxTokens: shiroMaxTokens, Temperature: 0.4}
	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, req)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		log.Printf("[Forecast] Summary generation failed (worker): %v", err)
		if err == nil {
			logIdleRaw("forecast.summary.generate", resp.Content)
		}
		return truncate(body, 200)
	}
	logIdleRaw("forecast.summary.generate", resp.Content)
	summary := sanitizeIdleSummaryResponse(resp.Content, topic)
	if summary == "" {
		log.Printf("[Forecast] Summary sanitize failed; raw=%q", truncate(strings.TrimSpace(resp.Content), 180))
		return truncate(body, 200)
	}
	return summary
}

// extractCoveredThemes は直近のトランスクリプトから新たに出た論点をキーワードリストとして抽出する。
func (o *IdleChatOrchestrator) extractCoveredThemes(domain ForecastDomain, topic string, transcript []string, existingThemes []string) []string {
	if len(transcript) < forecastCheckpointInterval {
		return nil
	}
	window := transcript
	if len(window) > forecastCheckpointInterval {
		window = window[len(window)-forecastCheckpointInterval:]
	}
	body := strings.Join(window, "\n")

	existingSection := ""
	if len(existingThemes) > 0 {
		existingSection = fmt.Sprintf("\n\n既に記録済みの論点（繰り返し禁止）:\n- %s", strings.Join(existingThemes, "\n- "))
	}

	messages := []llm.Message{
		{Role: "system", Content: "あなたは議論の進行管理者です。既出論点を正確に記録します。"},
		{Role: "user", Content: fmt.Sprintf(`以下は「%s」の議論（直近%dターン）です。

話題: %s
%s

会話ログ:
%s

この区間で新たに出た論点・主張を、1行1項目の箇条書きで抽出してください。
- 各項目は10〜20文字程度の短いキーワード/フレーズ
- 既に記録済みの論点と重複するものは除外
- 最大5項目
- 箇条書き（「- 」始まり）のみ出力、それ以外の文は不要`, domain.Name, len(window), topic, existingSection, body)},
	}
	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   shiroMaxTokens,
		Temperature: 0.3,
	})
	if err != nil {
		log.Printf("[Forecast] Theme extraction failed (worker): %v", err)
		return nil
	}
	logIdleRaw("forecast.theme_extract.generate", resp.Content)

	var themes []string
	for _, line := range strings.Split(resp.Content, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "・")
		line = strings.TrimSpace(line)
		if line != "" && len(themes) < 5 {
			themes = append(themes, line)
		}
	}
	return themes
}

// updateForecastSessionContext は蓄積された既出テーマを sessionContext に反映する。
// これにより generateResponse の全ターンで既出テーマが LLM に見える。
func (o *IdleChatOrchestrator) updateForecastSessionContext(domain ForecastDomain, topic string, coveredThemes []string) {
	if len(coveredThemes) == 0 {
		return
	}
	ctx := fmt.Sprintf(`【%s 議論ガード】話題: %s
以下は既に議論済みの論点です。これらの繰り返しや言い換えは厳禁です。必ず新しい視点・具体例・反論で議論を前に進めてください。

既出論点:
- %s

禁止: 上記の論点を別の言葉で言い直すこと、同じ結論に戻ること。
必須: 毎回、直前の発言に対して「新しい事実」「別の立場からの反論」「具体的な数字や事例」のいずれかを加えること。`,
		domain.Name, topic, strings.Join(coveredThemes, "\n- "))

	o.mu.Lock()
	o.sessionContext = ctx
	o.mu.Unlock()
}

// forecastLLM は未来展望セッション用の LLM を返す。forecastProvider があればそれを、なければ mio を使う。
func (o *IdleChatOrchestrator) forecastLLM() llm.LLMProvider {
	o.mu.Lock()
	p := o.forecastProvider
	o.mu.Unlock()
	if p != nil {
		return p
	}
	return o.providerForSpeaker("mio")
}

// generateForecastTopic はドメイン特化のトピックをLLM生成する。
func (o *IdleChatOrchestrator) generateForecastTopic(domain ForecastDomain, seeds []string) string {
	recentTopics := o.getRecentTopics(12)
	pastTitleThemes := o.getHistoricalTitleThemes(500)

	for attempt := 0; attempt < 3; attempt++ {
		prompt := generateForecastTopicPrompt(domain, seeds, pastTitleThemes)
		messages := []llm.Message{
			{Role: "system", Content: o.getSystemPrompt("mio")},
			{Role: "user", Content: prompt},
		}
		req := llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   420,
			Temperature: 0.9 + float64(attempt)*0.05,
		}
		resp, err := o.forecastLLM().Generate(o.ctx, req)
		if err != nil {
			log.Printf("[Forecast] Topic generation failed: %v", err)
			break
		}
		logIdleRaw(fmt.Sprintf("forecast.topic.generate attempt=%d domain=%s", attempt+1, domain.Name), resp.Content)
		topic := normalizeIdleTopic(resp.Content, false)
		if topic == "" {
			continue
		}
		if topicTooSimilar(topic, recentTopics) {
			log.Printf("[Forecast] Topic too similar, retrying: %s", truncate(topic, 80))
			continue
		}
		if topicTooSimilar(topic, pastTitleThemes) {
			log.Printf("[Forecast] Topic overlaps with past title memory, retrying: %s", truncate(topic, 80))
			continue
		}
		return topic
	}

	// フォールバック
	if len(seeds) > 0 {
		return fmt.Sprintf("%sの最新動向から見る今後の展望", domain.Name)
	}
	return fmt.Sprintf("%sの3年後を考える", domain.Name)
}

// extractForecastKeyword はNHKヘッドラインからドメインに関連する注目キーワードを1つ抽出する。
func (o *IdleChatOrchestrator) extractForecastKeyword(domain ForecastDomain, headlines []string) string {
	if len(headlines) == 0 {
		return domain.Name
	}
	prompt := fmt.Sprintf(`以下は「%s」分野の最新情報です。

%s

この中から、今後の社会に最もインパクトがありそうな検索キーワードを1つだけ抽出してください。
- キーワードのみ出力（説明不要）
- 2〜6語程度の具体的な用語
- 一般的すぎる語（「技術」「問題」等）は避ける`, domain.Name, strings.Join(headlines, "\n"))

	messages := []llm.Message{
		{Role: "system", Content: "あなたはニュース分析の専門家です。"},
		{Role: "user", Content: prompt},
	}
	resp, err := o.forecastLLM().Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   30,
		Temperature: 0.5,
	})
	if err != nil {
		log.Printf("[Forecast] Keyword extraction failed: %v", err)
		return domain.Name
	}
	logIdleRaw("forecast.keyword.generate", resp.Content)
	kw := strings.TrimSpace(resp.Content)
	if kw == "" {
		return domain.Name
	}
	// 改行があれば最初の行だけ
	if i := strings.IndexAny(kw, "\r\n"); i >= 0 {
		kw = strings.TrimSpace(kw[:i])
	}
	return kw
}

// fetchGoogleNewsSeeds はGoogle News RSSからキーワード検索でヘッドラインを取得する。
func fetchGoogleNewsSeeds(keyword string, limit int) []string {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil
	}
	rssURL := fmt.Sprintf("https://news.google.com/rss/search?q=%s&hl=ja&gl=JP&ceid=JP:ja",
		strings.ReplaceAll(keyword, " ", "+"))
	headlines, err := fetchGoogleNewsRSS(rssURL, limit)
	if err != nil {
		log.Printf("[Forecast] Google News RSS failed (q=%s): %v", keyword, err)
		return nil
	}
	return headlines
}

// fetchGoogleNewsRSS はGoogle News RSSをパースしてタイトルを取得する。
func fetchGoogleNewsRSS(rssURL string, limit int) ([]string, error) {
	req, err := http.NewRequest("GET", rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google news rss status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	var headlines []string
	inItem := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<item>") {
			inItem = true
		} else if strings.HasPrefix(line, "</item>") {
			inItem = false
		} else if inItem && strings.HasPrefix(line, "<title>") {
			title := strings.TrimPrefix(line, "<title>")
			if i := strings.Index(title, "</title>"); i >= 0 {
				title = title[:i]
			}
			title = strings.TrimSpace(title)
			// Google News のタイトルは「見出し - ソース名」形式。ソース名を除去
			if i := strings.LastIndex(title, " - "); i > 0 {
				title = strings.TrimSpace(title[:i])
			}
			if title != "" && len(headlines) < limit {
				headlines = append(headlines, title)
			}
		}
	}
	return headlines, nil
}

// --- トレンド収集 (Stage 1) ---

// TrendSourceSet はドメインごとのトレンドソース設定。
type TrendSourceSet struct {
	RedditSubs     []string
	HatenaCategory string
}

type forecastDomainProfile struct {
	Keywords []string
}

var domainTrendSources = map[string]TrendSourceSet{
	"AI技術":  {RedditSubs: []string{"artificial", "MachineLearning"}, HatenaCategory: "it"},
	"その他技術": {RedditSubs: []string{"technology"}, HatenaCategory: "it"},
	"医療":    {HatenaCategory: "social"},
	"社会保障":  {HatenaCategory: "social"},
	"政治":    {HatenaCategory: "economics"},
	"経済":    {RedditSubs: []string{"economics"}, HatenaCategory: "economics"},
}

var forecastDomainProfiles = map[string]forecastDomainProfile{
	"AI技術": {
		Keywords: []string{"ai", "人工知能", "生成ai", "llm", "機械学習", "半導体", "推論", "モデル", "自動運転", "ロボット"},
	},
	"その他技術": {
		Keywords: []string{"量子", "宇宙", "電池", "通信", "ネットワーク", "デバイス", "センサー", "材料", "エネルギー", "クラウド"},
	},
	"医療": {
		Keywords: []string{"医療", "病院", "診療", "治療", "創薬", "薬", "患者", "ワクチン", "手術", "介護"},
	},
	"社会保障": {
		Keywords: []string{"年金", "介護", "保険", "福祉", "少子化", "高齢化", "給付", "負担", "子育て", "生活保護"},
	},
	"政治": {
		Keywords: []string{"政権", "国会", "選挙", "外交", "防衛", "法案", "行政", "自治体", "官僚", "首相"},
	},
	"経済": {
		Keywords: []string{"金利", "為替", "株", "物価", "景気", "投資", "賃金", "雇用", "企業", "貿易"},
	},
}

// TrendCache は1時間TTLのトレンドキャッシュ。
type TrendCache struct {
	Hour              string              // "2006-01-02T15" 形式
	GoogleTrends      []string            // Google Trends JP
	RedditBySubreddit map[string][]string // subreddit → titles
	HatenaByCategory  map[string][]string // category → titles
	FetchedAt         time.Time
}

var (
	trendCache *TrendCache
	trendMu    sync.RWMutex
)

func getTrendCache() *TrendCache {
	trendMu.RLock()
	defer trendMu.RUnlock()
	return trendCache
}

func fetchHourlyTrends() error {
	hour := time.Now().In(jst).Format("2006-01-02T15")

	trendMu.RLock()
	if trendCache != nil && trendCache.Hour == hour {
		trendMu.RUnlock()
		return nil
	}
	trendMu.RUnlock()

	trendMu.Lock()
	defer trendMu.Unlock()
	if trendCache != nil && trendCache.Hour == hour {
		return nil
	}

	log.Printf("[Forecast] Fetching hourly trends for %s...", hour)

	cache := &TrendCache{
		Hour:              hour,
		RedditBySubreddit: make(map[string][]string),
		HatenaByCategory:  make(map[string][]string),
		FetchedAt:         time.Now(),
	}

	// Google Trends JP
	if trends, err := fetchGoogleTrendsJP(20); err != nil {
		log.Printf("[Forecast] Google Trends failed: %v", err)
	} else {
		cache.GoogleTrends = trends
	}

	// Reddit (全ドメインで使うサブレディットを集約)
	allSubs := make(map[string]struct{})
	for _, src := range domainTrendSources {
		for _, sub := range src.RedditSubs {
			allSubs[sub] = struct{}{}
		}
	}
	for sub := range allSubs {
		if titles, err := fetchRedditHot(sub, 10); err != nil {
			log.Printf("[Forecast] Reddit r/%s failed: %v", sub, err)
		} else {
			cache.RedditBySubreddit[sub] = titles
		}
	}

	// はてブ (全ドメインで使うカテゴリを集約)
	allCats := make(map[string]struct{})
	for _, src := range domainTrendSources {
		if src.HatenaCategory != "" {
			allCats[src.HatenaCategory] = struct{}{}
		}
	}
	for cat := range allCats {
		if titles, err := fetchHatenaHotentry(cat, 10); err != nil {
			log.Printf("[Forecast] Hatena %s failed: %v", cat, err)
		} else {
			cache.HatenaByCategory[cat] = titles
		}
	}

	trendCache = cache
	log.Printf("[Forecast] Trends fetched: google=%d reddit_subs=%d hatena_cats=%d",
		len(cache.GoogleTrends), len(cache.RedditBySubreddit), len(cache.HatenaByCategory))
	return nil
}

// fetchTrendSeeds はドメインに対応するトレンド情報を集約して返す。
func fetchTrendSeeds(domain ForecastDomain) []string {
	if err := fetchHourlyTrends(); err != nil {
		log.Printf("[Forecast] Trend fetch error: %v", err)
	}
	cache := getTrendCache()
	if cache == nil {
		return nil
	}

	src := domainTrendSources[domain.Name]
	var all []string

	// Google Trends は全ドメイン共通なので混ぜすぎない。
	if len(cache.GoogleTrends) > 0 {
		gt := cache.GoogleTrends
		if len(gt) > forecastGoogleTrendLimit {
			gt = pickRandom(gt, forecastGoogleTrendLimit)
		}
		all = append(all, gt...)
	}

	// Reddit
	for _, sub := range src.RedditSubs {
		if titles, ok := cache.RedditBySubreddit[sub]; ok {
			picked := titles
			if len(picked) > 5 {
				picked = pickRandom(titles, 5)
			}
			all = append(all, picked...)
		}
	}

	// はてブ
	if src.HatenaCategory != "" {
		if titles, ok := cache.HatenaByCategory[src.HatenaCategory]; ok {
			picked := titles
			if len(picked) > 5 {
				picked = pickRandom(titles, 5)
			}
			all = append(all, picked...)
		}
	}

	// 重複排除
	seen := make(map[string]struct{}, len(all))
	unique := make([]string, 0, len(all))
	for _, h := range all {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			unique = append(unique, h)
		}
	}
	unique = rankForecastSeeds(domain, unique)
	if len(unique) > forecastSeedLimit {
		unique = unique[:forecastSeedLimit]
	}
	return unique
}

func rankForecastSeeds(domain ForecastDomain, seeds []string) []string {
	if len(seeds) < 2 {
		return seeds
	}
	type scoredSeed struct {
		text  string
		score int
	}
	scored := make([]scoredSeed, 0, len(seeds))
	for _, seed := range seeds {
		scored = append(scored, scoredSeed{
			text:  seed,
			score: scoreForecastSeed(domain, seed),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return rand.Intn(2) == 0
	})
	out := make([]string, 0, len(scored))
	for _, item := range scored {
		out = append(out, item.text)
	}
	return out
}

func scoreForecastSeed(domain ForecastDomain, seed string) int {
	s := strings.ToLower(strings.TrimSpace(seed))
	if s == "" {
		return 0
	}
	score := 1
	for _, kw := range forecastDomainProfiles[domain.Name].Keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw == "" {
			continue
		}
		if strings.Contains(s, kw) {
			score += 4
		}
	}
	if strings.Contains(s, strings.ToLower(domain.Name)) {
		score += 3
	}
	// 少し長めで具体性のある見出しを優先する。
	if n := len([]rune(s)); n >= 16 && n <= 48 {
		score += 1
	}
	return score
}

// --- トレンド取得関数 ---

// fetchGoogleTrendsJP は Google Trends JP のRSSからトレンドワードを取得する。
func fetchGoogleTrendsJP(limit int) ([]string, error) {
	rssURL := "https://trends.google.co.jp/trending/rss?geo=JP"
	req, err := http.NewRequest("GET", rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("google trends rss status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	var titles []string
	inItem := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<item>") || strings.HasPrefix(line, "<item ") {
			inItem = true
		} else if strings.HasPrefix(line, "</item>") {
			inItem = false
		} else if inItem && strings.HasPrefix(line, "<title>") {
			title := strings.TrimPrefix(line, "<title>")
			if i := strings.Index(title, "</title>"); i >= 0 {
				title = title[:i]
			}
			title = strings.TrimSpace(title)
			if title != "" && len(titles) < limit {
				titles = append(titles, title)
			}
		}
	}
	return titles, nil
}

// fetchRedditHot は Reddit サブレディットの hot 記事タイトルを取得する。
func fetchRedditHot(subreddit string, limit int) ([]string, error) {
	url := fmt.Sprintf("https://www.reddit.com/r/%s/hot.json?limit=%d", subreddit, limit)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0 (https://github.com/Nyukimin/picoclaw_multiLLM)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("reddit r/%s status %d", subreddit, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data struct {
			Children []struct {
				Data struct {
					Title string `json:"title"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("reddit json parse: %w", err)
	}

	var titles []string
	for _, child := range result.Data.Children {
		t := strings.TrimSpace(child.Data.Title)
		if t != "" {
			titles = append(titles, t)
		}
	}
	return titles, nil
}

// fetchHatenaHotentry ははてなブックマークのホットエントリRSSからタイトルを取得する。
func fetchHatenaHotentry(category string, limit int) ([]string, error) {
	rssURL := fmt.Sprintf("https://b.hatena.ne.jp/hotentry/%s.rss", category)
	req, err := http.NewRequest("GET", rssURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "RenCrow/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("hatena %s rss status %d", category, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	content := string(body)
	var titles []string
	inItem := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<item") {
			inItem = true
		} else if strings.HasPrefix(line, "</item>") {
			inItem = false
		} else if inItem && strings.HasPrefix(line, "<title>") {
			title := strings.TrimPrefix(line, "<title>")
			if i := strings.Index(title, "</title>"); i >= 0 {
				title = title[:i]
			}
			title = strings.TrimSpace(title)
			if title != "" && len(titles) < limit {
				titles = append(titles, title)
			}
		}
	}
	return titles, nil
}
