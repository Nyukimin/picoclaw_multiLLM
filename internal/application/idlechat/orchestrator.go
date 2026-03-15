package idlechat

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	idleCheckInterval = 30 * time.Second
	maxTurnsPerTopic  = 12
	speakerBreak = 500 * time.Millisecond  // 話者交代ブレイク（TTS完了後）
	topicBreak   = 1000 * time.Millisecond // 話題交代ブレイク（TTS完了後）
)

var jst = time.FixedZone("JST", 9*60*60)
var randSeedOnce sync.Once
var errIdleInvalidResponse = errors.New("idlechat invalid response")
var promptLeakLineRe = regexp.MustCompile(`(?i)(^|[。．\n])[^。．\n]{0,30}(発言として受け|要件[:：]|発言帰属ガード)[^。．\n]*`)

type SessionSummary struct {
	SessionID       string        `json:"session_id"`
	Title           string        `json:"title"`
	Topic           string        `json:"topic"`
	Strategy        TopicStrategy `json:"strategy"` // 生成戦略（旧 Category）
	Summary         string        `json:"summary"`
	StartedAt       string        `json:"started_at"`
	EndedAt         string        `json:"ended_at"`
	Turns           int           `json:"turns"`
	LoopRestarted   bool          `json:"loop_restarted"`
	LoopReason      string        `json:"loop_reason,omitempty"`
	TopicProvider   string        `json:"topic_provider"`
	SummaryProvider string        `json:"summary_provider"`
	Transcript      []string      `json:"transcript,omitempty"`
}

type TimelineEvent struct {
	Type      string
	From      string
	To        string
	Content   string
	SessionID string
}

// IdleChatOrchestrator はアイドル時のAgent間雑談を管理
type IdleChatOrchestrator struct {
	llmProvider      llm.LLMProvider
	speakerLLMs      map[string]llm.LLMProvider
	forecastProvider llm.LLMProvider // 未来展望セッションの思考用（Coder2等の高性能モデル）
	sessionContext   string          // 現在のセッション固有コンテキスト（既出テーマ等）
	memory           *session.CentralMemory
	participants   []string
	intervalMin    int
	maxTurns       int
	temperature    float64
	personalities  map[string]string

	lastActivity time.Time
	chatActive   bool
	chatBusy     bool
	workerBusy   bool
	manualMode   bool
	currentTopic string
	nextTopicAt  time.Time
	history      []SessionSummary
	emitEvent    func(TimelineEvent) <-chan struct{}
	topicStore   *TopicStore
	recentTopics func(context.Context, int) ([]string, error)

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

// SetEventEmitter sets an optional timeline event emitter used by viewer SSE.
// The callback returns a channel that closes when TTS playback completes (nil = no TTS).
func (o *IdleChatOrchestrator) SetEventEmitter(emit func(TimelineEvent) <-chan struct{}) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitEvent = emit
}

// SetForecastProvider sets a high-capability LLM for forecast topic generation and keyword extraction.
func (o *IdleChatOrchestrator) SetForecastProvider(provider llm.LLMProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forecastProvider = provider
}

func (o *IdleChatOrchestrator) SetRecentTopicProvider(provider func(context.Context, int) ([]string, error)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.recentTopics = provider
}

// SetTopicStore configures persistent storage for topic summaries.
func (o *IdleChatOrchestrator) SetTopicStore(path string) error {
	store, err := NewTopicStore(path)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.topicStore = store
	o.history = store.GetRecent(200)
	o.mu.Unlock()
	return nil
}

// NewIdleChatOrchestrator は新しいIdleChatOrchestratorを作成
func NewIdleChatOrchestrator(
	llmProvider llm.LLMProvider,
	memory *session.CentralMemory,
	participants []string,
	intervalMin int,
	maxTurns int,
	temperature float64,
	personalities map[string]string,
) *IdleChatOrchestrator {
	randSeedOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})
	ctx, cancel := context.WithCancel(context.Background())
	return &IdleChatOrchestrator{
		llmProvider:   llmProvider,
		speakerLLMs:   make(map[string]llm.LLMProvider),
		memory:        memory,
		participants:  participants,
		intervalMin:   intervalMin,
		maxTurns:      maxTurns,
		temperature:   temperature,
		personalities: personalities,
		lastActivity:  time.Now(),
		history:        make([]SessionSummary, 0, 32),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (o *IdleChatOrchestrator) SetSpeakerProviders(providers map[string]llm.LLMProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.speakerLLMs = make(map[string]llm.LLMProvider, len(providers))
	for name, provider := range providers {
		if provider == nil {
			continue
		}
		o.speakerLLMs[strings.ToLower(strings.TrimSpace(name))] = provider
	}
}

func (o *IdleChatOrchestrator) providerForSpeaker(name string) llm.LLMProvider {
	o.mu.Lock()
	defer o.mu.Unlock()
	if provider, ok := o.speakerLLMs[strings.ToLower(strings.TrimSpace(name))]; ok && provider != nil {
		return provider
	}
	return o.llmProvider
}

// Start はIdleChatの監視ループを開始
func (o *IdleChatOrchestrator) Start() {
	// 起動時に外部シード取得（非同期）
	go func() {
		if err := fetchDailySeeds(); err != nil {
			log.Printf("[IdleChat] Daily seeds fetch failed: %v", err)
		}
	}()

	o.wg.Add(1)
	go o.monitorLoop()
	log.Printf("[IdleChat] Started (participants=%v, interval=%dmin, maxTurns=%d)",
		o.participants, o.intervalMin, o.maxTurns)
}

// Stop はIdleChatを停止
func (o *IdleChatOrchestrator) Stop() {
	o.cancel()
	o.wg.Wait()
	log.Println("[IdleChat] Stopped")
}

// NotifyActivity はタスク到着を通知（雑談セッションを中断）
func (o *IdleChatOrchestrator) NotifyActivity() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lastActivity = time.Now()
	if o.manualMode {
		log.Println("[IdleChat] Activity detected, stopping manual mode")
		o.manualMode = false
	}
	if o.chatActive {
		log.Println("[IdleChat] Task arrived, interrupting chat session")
		o.chatActive = false
	}
}

// SetChatBusy はChat(mio)の活性状態を更新する。
func (o *IdleChatOrchestrator) SetChatBusy(busy bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.chatBusy = busy
	if busy {
		o.lastActivity = time.Now()
		if o.manualMode {
			log.Println("[IdleChat] Chat is active, stopping manual mode")
			o.manualMode = false
		}
		if o.chatActive {
			log.Println("[IdleChat] Chat is active, interrupting chat session")
			o.chatActive = false
		}
	}
}

// SetWorkerBusy はWorker(shiro/coder)の活性状態を更新する。
func (o *IdleChatOrchestrator) SetWorkerBusy(busy bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.workerBusy = busy
	if busy {
		o.lastActivity = time.Now()
		if o.manualMode {
			log.Println("[IdleChat] Worker is active, stopping manual mode")
			o.manualMode = false
		}
		if o.chatActive {
			log.Println("[IdleChat] Worker is active, interrupting chat session")
			o.chatActive = false
		}
	}
}

// StartManualMode starts idle chat mode immediately.
func (o *IdleChatOrchestrator) StartManualMode() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.participants) < 2 {
		return fmt.Errorf("idlechat requires at least 2 participants")
	}
	o.manualMode = true
	o.lastActivity = time.Now()
	log.Println("[IdleChat] Manual mode started")
	return nil
}

// StopManualMode stops idle chat mode and interrupts an ongoing session.
func (o *IdleChatOrchestrator) StopManualMode() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.manualMode || o.chatActive {
		log.Println("[IdleChat] Manual mode stopped")
	}
	o.manualMode = false
	o.chatActive = false
	o.currentTopic = ""
	o.lastActivity = time.Now()
}

// IsManualMode returns whether manual idle chat mode is enabled.
func (o *IdleChatOrchestrator) IsManualMode() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.manualMode
}

// IsChatActive は雑談セッションが進行中かを返す
func (o *IdleChatOrchestrator) IsChatActive() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.chatActive
}

// CurrentTopic は現在のIdleChatトピックを返す。
func (o *IdleChatOrchestrator) CurrentTopic() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.currentTopic
}

// GetHistory returns newest-first session summaries.
func (o *IdleChatOrchestrator) GetHistory(limit int) []SessionSummary {
	o.mu.Lock()
	store := o.topicStore
	if store != nil {
		o.mu.Unlock()
		return store.GetRecent(limit)
	}
	defer o.mu.Unlock()
	if limit <= 0 || limit > len(o.history) {
		limit = len(o.history)
	}
	out := make([]SessionSummary, 0, limit)
	for i := len(o.history) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, o.history[i])
	}
	return out
}

func (o *IdleChatOrchestrator) monitorLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(idleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.checkAndStartChat()
		}
	}
}

func (o *IdleChatOrchestrator) checkAndStartChat() {
	o.mu.Lock()
	idleDuration := time.Since(o.lastActivity)
	threshold := time.Duration(o.intervalMin) * time.Minute
	now := time.Now()
	nextTopicAt := o.nextTopicAt
	alreadyActive := o.chatActive
	chatBusy := o.chatBusy
	workerBusy := o.workerBusy
	manualMode := o.manualMode
	o.mu.Unlock()

	if alreadyActive {
		return
	}
	if chatBusy || workerBusy {
		return
	}
	if !nextTopicAt.IsZero() && now.Before(nextTopicAt) {
		return
	}
	if !manualMode && idleDuration < threshold {
		return
	}

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	log.Printf("[IdleChat] Idle for %v, starting chat session", idleDuration.Round(time.Second))
	o.runChatSession()

	o.mu.Lock()
	o.chatActive = false
	o.currentTopic = ""
	o.lastActivity = time.Now() // セッション終了でアイドル計測をリセット
	o.mu.Unlock()
}

func (o *IdleChatOrchestrator) runChatSession() {
	sessionID := fmt.Sprintf("idle-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	remainingTurns := o.maxTurns

	for remainingTurns > 0 {
		topic, strategy := o.generateTopicFromChat(sessionID)
		o.mu.Lock()
		o.currentTopic = topic
		o.mu.Unlock()
		log.Printf("[IdleChat] Topic: %s (%s)", topic, strategy)
		o.emitTopicToTimeline(sessionID, topic, strategy)

		segmentTurns := 0
		loopDetected := false
		loopReason := ""
		sessionInterrupted := false
		generationFailed := false
		transcript := make([]string, 0, remainingTurns)
		currentSpeaker := o.chatSpeakerIndex()

		for turn := 0; turn < remainingTurns; turn++ {
			select {
			case <-o.ctx.Done():
				return
			default:
			}

			o.mu.Lock()
			if !o.chatActive {
				o.mu.Unlock()
				log.Printf("[IdleChat] Session interrupted at turn %d", turn)
				sessionInterrupted = true
				loopReason = "interrupted"
				break
			}
			o.mu.Unlock()

			speaker := o.participants[currentSpeaker]
			nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

			response, err := o.generateResponse(speaker, nextSpeaker, sessionID, turn, segmentTurns, topic)
			if err != nil {
				log.Printf("[IdleChat] Generation error: %v", err)
				generationFailed = true
				if errors.Is(err, errIdleInvalidResponse) {
					loopReason = "invalid_response"
				} else {
					loopReason = "generation_error"
				}
				break
			}
			if isResponseTooSimilar(response, transcript) {
				loopDetected = true
				loopReason = "pre_emit_similarity"
				log.Printf("[IdleChat] Repetitive response detected before emit, summarize and restart")
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

			log.Printf("[IdleChat] [Turn %d] %s→%s: %s", turn, speaker, nextSpeaker, truncate(response, 80))
			o.waitForTTSDone(ttsDone)
			o.waitBreak(speakerBreak)

			if segmentTurns >= maxTurnsPerTopic {
				loopDetected = true
				loopReason = "topic_turn_limit"
				log.Printf("[IdleChat] Topic turn limit reached (%d), summarize and switch topic", maxTurnsPerTopic)
				break
			}

			if reason := detectLoopReason(transcript); reason != "" {
				loopDetected = true
				loopReason = reason
				log.Printf("[IdleChat] Loop/repetition detected, summarize and restart with new topic")
				break
			}
			currentSpeaker = (currentSpeaker + 1) % len(o.participants)
		}

		remainingTurns -= segmentTurns
		endedAt := time.Now().In(jst)
		if segmentTurns > 0 {
			summary := o.saveSummary(sessionID, topic, strategy, transcript, startedAt, endedAt, segmentTurns, loopDetected || sessionInterrupted || generationFailed, loopReason)
			o.speakSummary(sessionID, summary)
		}
		cooldown := topicBreak
		if sessionInterrupted || generationFailed {
			idleCooldown := time.Duration(o.intervalMin) * time.Minute
			if idleCooldown > cooldown {
				cooldown = idleCooldown
			}
		}
		o.mu.Lock()
		o.nextTopicAt = endedAt.Add(cooldown)
		o.mu.Unlock()
		break
	}

	log.Printf("[IdleChat] Session %s completed (%d turns)", sessionID, o.maxTurns)
}

// waitForTTSDone はTTS完了チャネルを待つ。nilなら即座に返る。
func (o *IdleChatOrchestrator) waitForTTSDone(ch <-chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case <-o.ctx.Done():
		return
	case <-ch:
	}
}

// waitBreak はTTS完了後の沈黙を待つ。
func (o *IdleChatOrchestrator) waitBreak(d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-o.ctx.Done():
		return
	case <-timer.C:
	}
}

// ensureTrailingPeriod はセリフ末尾に句読点がなければ「。」を追記する。
func ensureTrailingPeriod(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	last, _ := utf8.DecodeLastRuneInString(s)
	switch last {
	case '。', '！', '？', '!', '?', '…':
		return s
	}
	return s + "。"
}

func (o *IdleChatOrchestrator) chatSpeakerIndex() int {
	for i, p := range o.participants {
		if strings.EqualFold(p, "mio") {
			return i
		}
	}
	return 0
}

func (o *IdleChatOrchestrator) generateTopicFromChat(sessionID string) (string, TopicStrategy) {
	// 戦略選択（chaos: 70%, external: 20%, anti: 10%）
	strategy := chooseStrategy()
	movieMode := rand.Intn(100) < 20
	recentTopics := o.getRecentTopics(12)

	var prompt string
	var logInfo string
	var fallbackTopic string

	switch strategy {
	case StrategySingleGenre:
		var genres []string
		prompt, genres = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v", genres)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", movieMode)

	case StrategyDoubleGenre:
		var genres []string
		prompt, genres = generateDoubleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("double:%v", genres)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", movieMode)

	case StrategyExternalStimulus:
		var source string
		prompt, source = generateExternalPrompt(movieMode)
		logInfo = fmt.Sprintf("external:%s", source)
		fallbackTopic = fallbackTopicForStrategy(strategy, nil, source, "", movieMode)

	default:
		// Fallback to single genre
		var genres []string
		prompt, genres = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v (fallback)", genres)
		fallbackTopic = fallbackTopicForStrategy(StrategySingleGenre, genres, "", "", movieMode)
	}

	if o.recentTopics != nil {
		if glossaryTopics, err := o.recentTopics(o.ctx, 6); err != nil {
			log.Printf("[IdleChat] glossary topics failed: %v", err)
		} else if len(glossaryTopics) > 0 {
			prompt += "\n\n最近語彙メモ:\n- " + strings.Join(glossaryTopics, "\n- ") + "\n上の語彙は、最近の時事語彙や固有名詞の種です。詳細断言ではなく、お題の発想補助として軽く使ってください。"
		}
	}

	log.Printf("[IdleChat] Strategy: %s (%s, movie=%t)", strategy, logInfo, movieMode)

	// トピック生成（最大3回リトライ）
	for attempt := 0; attempt < 3; attempt++ {
		messages := []llm.Message{
			{Role: "system", Content: o.getSystemPrompt("mio")},
			{Role: "user", Content: prompt},
		}
		req := llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   150,
			Temperature: 0.9 + float64(attempt)*0.05, // 高めの温度で多様性確保
		}
		resp, err := o.providerForSpeaker("mio").Generate(o.ctx, req)
		if err != nil {
			log.Printf("[IdleChat] topic generation failed: %v", err)
			break
		}
		topic := normalizeIdleTopic(resp.Content, movieMode)
		if topic == "" {
			continue
		}
		if topicTooSimilar(topic, recentTopics) {
			log.Printf("[IdleChat] topic too similar to recent history, retrying: %s", truncate(topic, 80))
			continue
		}
		log.Printf("[IdleChat] Topic: %s (%s)", topic, strategy)
		return topic, strategy
	}

	// フォールバック
	fallback := normalizeIdleTopic(fallbackTopic, movieMode)
	if fallback == "" {
		fallback = "予想外の切り口から考える論点"
	}
	log.Printf("[IdleChat] Topic (fallback): %s", fallback)
	return fallback, strategy
}

func fallbackTopicForStrategy(strategy TopicStrategy, genres []string, source string, seed string, movieMode bool) string {
	switch strategy {
	case StrategySingleGenre:
		if len(genres) >= 1 && strings.TrimSpace(genres[0]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "の裏側")
			}
			return fmt.Sprintf("%sで見落としがちな判断基準", genres[0])
		}
	case StrategyDoubleGenre:
		if len(genres) >= 2 && strings.TrimSpace(genres[0]) != "" && strings.TrimSpace(genres[1]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "と" + genres[1])
			}
			return fmt.Sprintf("%sと%sに共通する設計思想", genres[0], genres[1])
		}
	case StrategyExternalStimulus:
		sourceName := source
		seedText := seed
		if strings.Contains(source, ":") {
			parts := strings.SplitN(source, ":", 2)
			sourceName = parts[0]
			seedText = parts[1]
		}
		if strings.TrimSpace(seedText) != "" {
			if movieMode {
				return formatMovieTopicPrompt(seedText)
			}
			return fmt.Sprintf("「%s」から掘る盲点と前提", seedText)
		}
		if strings.TrimSpace(sourceName) != "" {
			if movieMode {
				return formatMovieTopicPrompt(sourceName + "の裏側")
			}
			return fmt.Sprintf("%s由来の刺激から掘る盲点と前提", sourceName)
		}
	}
	if movieMode {
		return formatMovieTopicPrompt("予想外の切り口")
	}
	return "予想外の切り口から考える論点"
}

func normalizeIdleTopic(raw string, movieMode bool) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	replacers := []string{
		"話題:", "",
		"トピック:", "",
		"お題:", "",
		"話題：", "",
		"トピック：", "",
		"お題：", "",
		"\"", "",
	}
	s = strings.NewReplacer(replacers...).Replace(s)
	s = strings.TrimSpace(s)

	for _, marker := range []string{"、つまり、", "。つまり、", " つまり、", "っていうのは", "ってのは", "というのは"} {
		if idx := strings.Index(s, marker); idx > 0 {
			s = strings.TrimSpace(s[:idx])
			break
		}
	}
	for _, ending := range []string{
		"って、めちゃくちゃ面白いんじゃない？",
		"って、面白いんじゃない？",
		"って面白いんじゃない？",
		"ってどうだろう？",
		"じゃない？",
		"でしょうか？",
		"どうだろう？",
	} {
		s = strings.TrimSpace(strings.TrimSuffix(s, ending))
	}
	s = strings.TrimSpace(strings.TrimRight(s, "。！？!? "))
	s = multiSpaceForTopic(s)
	if movieMode {
		return formatMovieTopicPrompt(s)
	}
	if utf8.RuneCountInString(s) > 48 {
		s = truncate(s, 48)
	}
	return strings.TrimSpace(s)
}

func formatMovieTopicPrompt(raw string) string {
	title := strings.TrimSpace(raw)
	if title == "" {
		return ""
	}
	for {
		switch {
		case strings.HasPrefix(title, "「") && strings.HasSuffix(title, "」"):
			title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "「"), "」"))
			continue
		case strings.HasPrefix(title, "『") && strings.HasSuffix(title, "』"):
			title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "『"), "』"))
			continue
		}
		break
	}
	if idx := strings.Index(title, "ってどんな映画"); idx >= 0 {
		title = title[:idx]
	}
	title = strings.TrimSpace(strings.Trim(title, "「」『』\"'"))
	title = multiSpaceForTopic(title)
	if title == "" {
		return ""
	}
	if utf8.RuneCountInString(title) > 24 {
		title = truncate(title, 24)
		title = strings.TrimSpace(strings.TrimSuffix(title, "..."))
	}
	return fmt.Sprintf("「%s」ってどんな映画？", title)
}

func isMovieTopicPrompt(topic string) bool {
	s := strings.TrimSpace(topic)
	return strings.HasPrefix(s, "「") && strings.Contains(s, "」ってどんな映画")
}

func multiSpaceForTopic(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func collectLatestSessionSnippets(entries []session.ConversationEntry, match func(domaintransport.Message) bool, max int) []string {
	latestSessionID := ""
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if match(m) && strings.TrimSpace(m.SessionID) != "" {
			latestSessionID = m.SessionID
			break
		}
	}
	if latestSessionID == "" {
		return nil
	}

	snippets := make([]string, 0, max)
	for i := len(entries) - 1; i >= 0 && len(snippets) < max; i-- {
		m := entries[i].Message
		if m.SessionID == latestSessionID && match(m) {
			snippets = append(snippets, truncate(m.Content, 80))
		}
	}
	return snippets
}

func isIdleSession(sessionID string) bool {
	return strings.HasPrefix(strings.ToLower(sessionID), "idle-")
}

func isIdleMessage(m domaintransport.Message) bool {
	return m.Type == domaintransport.MessageTypeIdleChat || isIdleSession(m.SessionID)
}

func isWorkerMessage(m domaintransport.Message) bool {
	return strings.EqualFold(m.From, "shiro") || strings.EqualFold(m.To, "shiro")
}

func isUserMessage(m domaintransport.Message) bool {
	return strings.EqualFold(m.From, "user")
}

func (o *IdleChatOrchestrator) formatHintsFromLatestSession(entries []session.ConversationEntry, match func(domaintransport.Message) bool, fallback string) string {
	parts := collectLatestSessionSnippets(entries, match, 3)
	if len(parts) == 0 {
		return fallback
	}
	return strings.Join(parts, " / ")
}

func (o *IdleChatOrchestrator) isLooping(transcript []string) bool {
	return detectLoopReason(transcript) != ""
}

func detectLoopReason(transcript []string) string {
	if len(transcript) < 6 {
		return ""
	}
	norm := normalizeLoopText
	last := norm(transcript[len(transcript)-1])
	if last == "" {
		return ""
	}
	count := 0
	for i := len(transcript) - 4; i < len(transcript)-1; i++ {
		if i >= 0 && norm(transcript[i]) == last {
			count++
		}
	}
	if count >= 1 {
		return "exact_repeat"
	}
	if hasAlternatingLoop(transcript) {
		return "alternating_repeat"
	}
	if hasSpeakerTemplateLoop(transcript) {
		return "template_repeat"
	}
	if hasHighSimilarityLoop(transcript) {
		return "high_similarity"
	}
	if isWhatIfRepetition(transcript) {
		return "what_if_repeat"
	}
	return ""
}

func isWhatIfRepetition(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	start := len(transcript) - 8
	if start < 0 {
		start = 0
	}
	repeated := 0
	for i := start; i < len(transcript); i++ {
		line := strings.ToLower(transcript[i])
		if strings.Contains(line, "もし") && (strings.Contains(line, "だったら") || strings.Contains(line, "なら")) {
			repeated++
		}
	}
	// 直近発話の半数以上が「もし〜だったら/なら」ならループとみなす。
	window := len(transcript) - start
	return repeated >= 4 && repeated*2 >= window
}

func (o *IdleChatOrchestrator) saveSummary(sessionID, topic string, strategy TopicStrategy, transcript []string, startedAt, endedAt time.Time, turns int, loopRestarted bool, loopReason string) string {
	summary := o.summarizeByWorker(topic, transcript)
	summary = annotateLoopSummary(summary, loopRestarted, loopReason)
	title := fmt.Sprintf("%d月%d日の%sの話題まとめ", endedAt.Month(), endedAt.Day(), truncate(topic, 24))
	record := SessionSummary{
		SessionID:       sessionID,
		Title:           title,
		Topic:           topic,
		Strategy:        strategy,
		Summary:         summary,
		StartedAt:       startedAt.Format(time.RFC3339),
		EndedAt:         endedAt.Format(time.RFC3339),
		Turns:           turns,
		LoopRestarted:   loopRestarted,
		LoopReason:      loopReason,
		TopicProvider:   "mio",
		SummaryProvider: "shiro",
		Transcript:      append([]string(nil), transcript...),
	}
	o.mu.Lock()
	o.history = append(o.history, record)
	if len(o.history) > 200 {
		o.history = o.history[len(o.history)-200:]
	}
	store := o.topicStore
	o.mu.Unlock()
	if store != nil {
		if err := store.Append(record); err != nil {
			log.Printf("[IdleChat] topic store append failed: %v", err)
		}
	}

	msg := domaintransport.NewMessage("shiro", "idlechat_summary", sessionID, "", title+"\n"+summary)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.summary",
		From:      "shiro",
		To:        "idlechat_summary",
		Content:   title + "\n" + summary,
		SessionID: sessionID,
	})
	return summary
}

// speakSummary は Mio にまとめを読み上げさせ、TTS 完了を待つ。
func (o *IdleChatOrchestrator) speakSummary(sessionID, summary string) {
	if strings.TrimSpace(summary) == "" {
		return
	}
	msg := domaintransport.NewMessage("mio", "user", sessionID, "", summary)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	ttsDone := o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "user",
		Content:   summary,
		SessionID: sessionID,
	})
	log.Printf("[IdleChat] Mio reading summary: %s", truncate(summary, 80))
	o.waitForTTSDone(ttsDone)
	o.waitBreak(topicBreak)
}

func (o *IdleChatOrchestrator) summarizeByWorker(topic string, transcript []string) string {
	if len(transcript) == 0 {
		return "会話ログがありません。"
	}
	body := strings.Join(transcript, "\n")
	messages := []llm.Message{
		{Role: "system", Content: o.getSystemPrompt("shiro")},
		{Role: "user", Content: fmt.Sprintf("次のidleChatを要約してください。硬い報告書ではなく、読んで雰囲気が分かる短い要約にしてください。1. いちばん面白かった点 2. 何が話を前に進めたか 3. 次に広がりそうな観点、の順で自然にまとめてください。\n話題: %s\n\n%s", topic, body)},
	}
	req := llm.GenerateRequest{Messages: messages, MaxTokens: 800, Temperature: 0.4}
	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, req)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		return truncate(body, 200)
	}
	return strings.TrimSpace(resp.Content)
}

func annotateLoopSummary(summary string, loopRestarted bool, loopReason string) string {
	if !loopRestarted || strings.TrimSpace(loopReason) == "" {
		return summary
	}
	note := loopReasonLabel(loopReason)
	if note == "" {
		return summary
	}
	if strings.TrimSpace(summary) == "" {
		return "注記: " + note
	}
	return "注記: " + note + "\n\n" + summary
}

func loopReasonLabel(reason string) string {
	switch reason {
	case "template_repeat":
		return "テンプレ反復で打ち切り"
	case "alternating_repeat":
		return "交互反復で打ち切り"
	case "exact_repeat", "high_similarity", "pre_emit_similarity":
		return "類似発話の反復で打ち切り"
	case "what_if_repeat":
		return "仮定表現の反復で打ち切り"
	case "topic_turn_limit":
		return ""
	case "interrupted":
		return "中断で終了"
	case "generation_error":
		return "生成エラーで終了"
	case "invalid_response":
		return "返答崩れで終了"
	default:
		return "反復検知で打ち切り"
	}
}

func (o *IdleChatOrchestrator) generateResponse(speaker, target, sessionID string, turn int, segmentTurns int, topic string) (string, error) {
	systemPrompt := o.getSystemPrompt(speaker)
	temp := o.temperatureForSpeaker(speaker)

	// 履歴は浅めにして、古いテンプレが自己強化しないようにする。
	recentEntries := o.memory.GetUnifiedView(12)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	selfCtx, otherCtx := splitSpeakerContexts(recentEntries, sessionID, speaker, 2)
	latestOther := latestOtherUtterance(recentEntries, sessionID, speaker)
	latestSelf := latestSelfUtterance(recentEntries, sessionID, speaker)

	sessionEntries := make([]session.ConversationEntry, 0, 4)
	for i := len(recentEntries) - 1; i >= 0 && len(sessionEntries) < 4; i-- {
		if recentEntries[i].Message.SessionID == sessionID {
			sessionEntries = append(sessionEntries, recentEntries[i])
		}
	}
	for i := len(sessionEntries) - 1; i >= 0; i-- {
		entry := sessionEntries[i]
		role := "assistant"
		if entry.Message.From != speaker {
			role = "user"
		}
		messages = append(messages, llm.Message{
			Role:    role,
			Content: fmt.Sprintf("[%s]: %s", entry.Message.From, entry.Message.Content),
		})
	}

	// セッション固有コンテキスト（既出テーマ等）があれば注入
	o.mu.Lock()
	sc := o.sessionContext
	o.mu.Unlock()
	if sc != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: sc,
		})
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: buildIdleResponseGuardPrompt(speaker, selfCtx, otherCtx),
	})
	if o.recentTopics != nil {
		if glossaryTopics, err := o.recentTopics(o.ctx, 5); err != nil {
			log.Printf("[IdleChat] glossary context failed: %v", err)
		} else if len(glossaryTopics) > 0 {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "最近語彙メモ:\n- " + strings.Join(glossaryTopics, "\n- ") + "\n最近語彙は会話の種としてだけ使い、詳細断言はしないでください。",
			})
		}
	}
	if isMovieTopicPrompt(topic) {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: "これは架空映画の妄想会話です。実在作品として扱わず、『聞いたことがある』『前に見た』『有名作だ』のような既知前提は禁止。抽象論より、主人公・事件・場面・対立・反転を早めに一つ出してください。",
		})
	}

	if turn == 0 {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildIdleTurnPrompt(topic, target, "", "", turn, segmentTurns, true),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildIdleTurnPrompt(topic, speaker, latestOther, latestSelf, turn, segmentTurns, false),
		})
	}

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   160,
		Temperature: temp,
	}

	provider := o.providerForSpeaker(speaker)
	resp, err := provider.Generate(o.ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM generate primary: %w", err)
	}
	firstRaw := strings.TrimSpace(resp.Content)
	first := sanitizeIdleResponse(resp.Content, topic)
	if invalidIdleResponse(firstRaw) {
		retryInvalid := append([]llm.Message{}, messages...)
		retryInvalid = append(retryInvalid, llm.Message{
			Role:    "user",
			Content: "今の返答は無効です。記号だけや空文をやめて、自然な会話文を1-2文で言い直してください。",
		})
		respInvalid, errInvalid := provider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryInvalid,
			MaxTokens:   160,
			Temperature: temp,
		})
		if errInvalid != nil {
			log.Printf("[IdleChat] retryInvalid failed (%s turn=%d): %v", speaker, turn, errInvalid)
		}
		if errInvalid == nil && strings.TrimSpace(respInvalid.Content) != "" {
			first = sanitizeIdleResponse(respInvalid.Content, topic)
			firstRaw = strings.TrimSpace(respInvalid.Content)
		}
	}
	if needsIdleStyleRetry(speaker, first, latestOther, latestSelf, topic) {
		retryStyle := append([]llm.Message{}, messages...)
		retryStyle = append(retryStyle, llm.Message{
			Role:    "user",
			Content: "評価や言い直し宣言は書かず、別の手で自然に返してください。直前の言い回しをなぞらず、1文目で反応し、2文目で新しい具体例か問いを一つだけ足してください。",
		})
		respStyle, errStyle := provider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryStyle,
			MaxTokens:   160,
			Temperature: temp,
		})
		if errStyle != nil {
			log.Printf("[IdleChat] retryStyle failed (%s turn=%d): %v", speaker, turn, errStyle)
		}
		if errStyle == nil && strings.TrimSpace(respStyle.Content) != "" {
			first = sanitizeIdleResponse(respStyle.Content, topic)
			firstRaw = strings.TrimSpace(respStyle.Content)
		}
	}
	if hasPromptLeak(first) {
		retryLeak := append([]llm.Message{}, messages...)
		retryLeak = append(retryLeak, llm.Message{
			Role:    "user",
			Content: "指示文の断片を消して、自然な会話文だけを1-2文で言い直してください。メタ表現は禁止です。",
		})
		respLeak, errLeak := provider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryLeak,
			MaxTokens:   160,
			Temperature: temp,
		})
		if errLeak != nil {
			log.Printf("[IdleChat] retryLeak failed (%s turn=%d): %v", speaker, turn, errLeak)
		}
		if errLeak == nil && strings.TrimSpace(respLeak.Content) != "" {
			first = sanitizeIdleResponse(respLeak.Content, topic)
		}
	}
	if violatesAttribution(first, latestOther) {
		retry := append([]llm.Message{}, messages...)
		retry = append(retry, llm.Message{
			Role:    "user",
			Content: "発言帰属が曖昧です。相手の案を受ける形にして、1-2文で言い直してください。",
		})
		resp2, err2 := provider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retry,
			MaxTokens:   160,
			Temperature: temp,
		})
		if err2 != nil {
			log.Printf("[IdleChat] retryAttribution failed (%s turn=%d): %v", speaker, turn, err2)
		}
		if err2 == nil && strings.TrimSpace(resp2.Content) != "" {
			return sanitizeIdleResponse(resp2.Content, topic), nil
		}
	}

	if invalidIdleResponse(first) {
		log.Printf("[IdleChat] invalid_response sanitized (%s turn=%d): raw=%q sanitized=%q", speaker, turn, firstRaw, first)
		return "", errIdleInvalidResponse
	}

	return first, nil
}

func (o *IdleChatOrchestrator) temperatureForSpeaker(speaker string) float64 {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "mio", "shiro":
		return 0.65
	default:
		return o.temperature
	}
}

func (o *IdleChatOrchestrator) getRecentTopics(limit int) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if limit <= 0 || limit > len(o.history) {
		limit = len(o.history)
	}
	out := make([]string, 0, limit)
	for i := len(o.history) - 1; i >= 0 && len(out) < limit; i-- {
		t := strings.TrimSpace(o.history[i].Topic)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasAlternatingLoop(transcript []string) bool {
	if len(transcript) < 8 {
		return false
	}
	a := normalizeLoopText(transcript[len(transcript)-1])
	b := normalizeLoopText(transcript[len(transcript)-2])
	if a == "" || b == "" {
		return false
	}
	matches := 0
	for i := len(transcript) - 3; i >= 0 && i >= len(transcript)-7; i -= 2 {
		if textSimilarity(a, normalizeLoopText(transcript[i])) >= 0.9 {
			matches++
		}
	}
	for i := len(transcript) - 4; i >= 0 && i >= len(transcript)-8; i -= 2 {
		if textSimilarity(b, normalizeLoopText(transcript[i])) >= 0.9 {
			matches++
		}
	}
	return matches >= 3
}

func hasHighSimilarityLoop(transcript []string) bool {
	if len(transcript) < 10 {
		return false
	}
	start := len(transcript) - 10
	base := make([]string, 0, 10)
	for i := start; i < len(transcript); i++ {
		t := normalizeLoopText(transcript[i])
		if t != "" {
			base = append(base, t)
		}
	}
	if len(base) < 6 {
		return false
	}
	similarPairs := 0
	totalPairs := 0
	for i := 0; i < len(base); i++ {
		for j := i + 1; j < len(base); j++ {
			totalPairs++
			if textSimilarity(base[i], base[j]) >= 0.92 {
				similarPairs++
			}
		}
	}
	return totalPairs > 0 && similarPairs*3 >= totalPairs
}

func hasSpeakerTemplateLoop(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	type speakerTurn struct {
		speaker string
		text    string
	}
	turns := make([]speakerTurn, 0, 10)
	start := len(transcript) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(transcript); i++ {
		speaker, text := splitTranscriptSpeaker(transcript[i])
		if speaker == "" || text == "" {
			continue
		}
		turns = append(turns, speakerTurn{speaker: speaker, text: text})
	}
	if len(turns) < 6 {
		return false
	}

	perSpeaker := map[string][]string{}
	for _, turn := range turns {
		key := transcriptLeadPattern(turn.text)
		if key == "" {
			continue
		}
		perSpeaker[turn.speaker] = append(perSpeaker[turn.speaker], key)
	}
	for _, keys := range perSpeaker {
		if repeatedLeadPattern(keys) {
			return true
		}
	}

	for speaker := range perSpeaker {
		msgs := make([]string, 0, 4)
		for i := len(turns) - 1; i >= 0 && len(msgs) < 4; i-- {
			if turns[i].speaker == speaker {
				msgs = append(msgs, normalizeLoopText(turns[i].text))
			}
		}
		if len(msgs) < 3 {
			continue
		}
		similarPairs := 0
		for i := 0; i < len(msgs); i++ {
			for j := i + 1; j < len(msgs); j++ {
				if textSimilarity(msgs[i], msgs[j]) >= 0.82 {
					similarPairs++
				}
			}
		}
		if similarPairs >= 2 {
			return true
		}
	}
	return false
}

func splitTranscriptSpeaker(line string) (speaker, text string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", strings.TrimSpace(line)
	}
	speaker = strings.ToLower(strings.TrimSpace(line[:idx]))
	text = strings.TrimSpace(line[idx+1:])
	return speaker, text
}

func transcriptLeadPattern(text string) string {
	s := strings.TrimSpace(strings.ToLower(text))
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	s = strings.TrimPrefix(s, "[mio]")
	s = strings.TrimPrefix(s, "[shiro]")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	count := 0
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			break
		}
		b.WriteRune(r)
		count++
		if count >= 8 {
			break
		}
	}
	return b.String()
}

func repeatedLeadPattern(keys []string) bool {
	if len(keys) < 3 {
		return false
	}
	counts := map[string]int{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		counts[key]++
		if counts[key] >= 3 {
			return true
		}
	}
	return false
}

func topicTooSimilar(topic string, recent []string) bool {
	n := normalizeLoopText(topic)
	if n == "" {
		return true
	}
	for _, prev := range recent {
		if textSimilarity(n, normalizeLoopText(prev)) >= 0.9 {
			return true
		}
	}
	return false
}

func isResponseTooSimilar(response string, transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	cur := normalizeLoopText(response)
	if cur == "" {
		return false
	}
	start := len(transcript) - 6
	if start < 0 {
		start = 0
	}
	hits := 0
	for i := start; i < len(transcript); i++ {
		prev := normalizeLoopText(transcript[i])
		if prev == "" {
			continue
		}
		if textSimilarity(cur, prev) >= 0.93 {
			hits++
		}
	}
	return hits >= 2
}

func normalizeLoopText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if idx := strings.Index(s, ":"); idx >= 0 {
		s = strings.TrimSpace(s[idx+1:])
	}
	s = strings.TrimPrefix(s, "[mio]")
	s = strings.TrimPrefix(s, "[shiro]")
	s = strings.TrimPrefix(s, "[worker]")
	s = strings.TrimPrefix(s, "[chat]")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func textSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	ag := runeNGrams(a, 2)
	bg := runeNGrams(b, 2)
	if len(ag) == 0 || len(bg) == 0 {
		if a == b {
			return 1
		}
		return 0
	}
	inter := 0
	i, j := 0, 0
	for i < len(ag) && j < len(bg) {
		if ag[i] == bg[j] {
			inter++
			i++
			j++
			continue
		}
		if ag[i] < bg[j] {
			i++
		} else {
			j++
		}
	}
	return (2.0 * float64(inter)) / float64(len(ag)+len(bg))
}

func runeNGrams(s string, n int) []string {
	r := []rune(s)
	if len(r) < n || n <= 0 {
		return nil
	}
	out := make([]string, 0, len(r)-n+1)
	for i := 0; i <= len(r)-n; i++ {
		out = append(out, string(r[i:i+n]))
	}
	sort.Strings(out)
	return out
}

func splitSpeakerContexts(entries []session.ConversationEntry, sessionID, speaker string, limit int) ([]string, []string) {
	self := make([]string, 0, limit)
	other := make([]string, 0, limit)
	for i := len(entries) - 1; i >= 0 && (len(self) < limit || len(other) < limit); i-- {
		m := entries[i].Message
		if m.SessionID != sessionID {
			continue
		}
		text := truncate(strings.TrimSpace(m.Content), 80)
		if text == "" {
			continue
		}
		if strings.EqualFold(m.From, speaker) {
			if len(self) < limit {
				self = append(self, text)
			}
			continue
		}
		if len(other) < limit {
			other = append(other, fmt.Sprintf("%s: %s", m.From, text))
		}
	}
	if len(self) == 0 {
		self = append(self, "なし")
	}
	if len(other) == 0 {
		other = append(other, "なし")
	}
	return self, other
}

func latestOtherUtterance(entries []session.ConversationEntry, sessionID, speaker string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if m.SessionID != sessionID || strings.EqualFold(m.From, speaker) {
			continue
		}
		return strings.TrimSpace(m.Content)
	}
	return ""
}

func latestSelfUtterance(entries []session.ConversationEntry, sessionID, speaker string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if m.SessionID != sessionID || !strings.EqualFold(m.From, speaker) {
			continue
		}
		return strings.TrimSpace(m.Content)
	}
	return ""
}

func violatesAttribution(response, latestOther string) bool {
	resp := normalizeLoopText(response)
	other := normalizeLoopText(latestOther)
	if resp == "" || other == "" {
		return false
	}
	if textSimilarity(resp, other) < 0.93 {
		return false
	}
	lower := strings.ToLower(response)
	if strings.Contains(lower, "あなた") || strings.Contains(lower, "君") || strings.Contains(lower, "相手") || strings.Contains(lower, "その視点") {
		return false
	}
	return true
}

func buildIdleResponseGuardPrompt(speaker string, selfCtx, otherCtx []string) string {
	_ = selfCtx
	_ = otherCtx
	return fmt.Sprintf(
		"あなたは %s。\nルール:\n- 相手や自分の直前の言い回しをなぞらない\n- 同じ比喩やたとえの型を続けず、次は因果・場面・手順のどれかに切り替える\n- 言いよどみや同意テンプレで始めない\n- メタ定型句や堅い敬語導入を避ける",
		speaker,
	)
}

func buildIdleTurnPrompt(topic, speakerOrTarget, latestOther, latestSelf string, turn int, segmentTurns int, firstTurn bool) string {
	movieMode := isMovieTopicPrompt(topic)
	closingMode := !firstTurn && turnsLeftInTopic(segmentTurns) <= 2
	move := idleTurnMove(speakerOrTarget, turn, firstTurn, movieMode, closingMode)
	audience := idleAudienceAngle(turn, movieMode, closingMode)
	shiftHint := idleShiftHint(latestOther, latestSelf)
	if firstTurn {
		return fmt.Sprintf(
			"話題: %s\n%sとして1-2文で始めてください。\n今回の役割: %s\n読者の楽しみ: %s\nルール:\n- 自然に入る\n- 相手が返しやすい観点か問いを1つ入れる\n- 映画お題なら主人公・事件・場面のどれかを出す",
			topic,
			speakerOrTarget,
			move,
			audience,
		)
	}
	return fmt.Sprintf(
		"話題: %s\n%sとして1-2文で返答してください。\n直前の相手発言: %s\n自分の直前発言: %s\n今回の役割: %s\n読者の楽しみ: %s\nルール:\n- 1文目は反応、2文目で前に進める\n- 直前と同じ比喩の型を繰り返さず、因果・場面・手順のどれかにずらす\n%s\n- 抽象語だけで逃げず、少し具体化する\n- 映画お題なら主人公・事件・対立・反転のどれかを進める\n%s",
		topic,
		speakerOrTarget,
		quoteOrDash(latestOther),
		quoteOrDash(latestSelf),
		move,
		audience,
		shiftHint,
		idleClosingHint(closingMode, movieMode),
	)
}

func turnsLeftInTopic(segmentTurns int) int {
	left := maxTurnsPerTopic - segmentTurns
	if left < 0 {
		return 0
	}
	return left
}

func idleTurnMove(speaker string, turn int, firstTurn, movieMode, closingMode bool) string {
	name := strings.ToLower(strings.TrimSpace(speaker))
	if closingMode {
		if movieMode {
			if name == "shiro" {
				return "ここまでの筋を一度まとめ、最後に残る不穏さか余韻を一つ置く"
			}
			return "ここまでで一番強い場面か感情を拾い、締めの一言に寄せる"
		}
		if name == "shiro" {
			return "ここまでで見えた核心を一段だけ整理し、最後に残る問いを一つ置く"
		}
		return "ここまでの話を受けて、いちばん面白い芯を拾い、最後に余韻のある問いか感想で締める"
	}
	if movieMode {
		if firstTurn {
			if name == "shiro" {
				return "設定を整理しつつ、最初の異変か事件を一つ置く"
			}
			return "印象的な一場面か主人公像を先に出して、話を動かす"
		}
		if name == "shiro" {
			steps := []string{
				"前の案を少し整理して、条件か制約を一つ足す",
				"前の案の弱いところを示して、対立か障害を一つ足す",
				"前の案を保ったまま、ラストの反転候補を一つ足す",
			}
			return steps[turn%len(steps)]
		}
		steps := []string{
			"前の案を受けて、場面を一つ具体化する",
			"前の案を受けて、主人公の感情か動機を一つ具体化する",
			"前の案を受けて、行動か出来事を一つ具体化する",
		}
		return steps[turn%len(steps)]
	}
	if firstTurn {
		if name == "shiro" {
			return "論点を一つに絞り、どこが核心かを示す"
		}
		return "比喩か具体例で入口を作り、相手が掘れる論点を一つ出す"
	}
	if name == "shiro" {
		steps := []string{
			"相手の案を整理し、因果のつながりを一段だけはっきりさせる",
			"相手の案を整理し、反対側から見た条件を一つ足す",
			"相手の案を整理し、身近な具体例を一つ足す",
			"相手の案を整理し、次に起きそうな場面を一つ置く",
		}
		return steps[turn%len(steps)]
	}
	steps := []string{
		"相手の案を受けて、場面や手触りを一つ足して前に進める",
		"相手の案を受けて、具体的な手順や動きを一つ足して前に進める",
		"相手の案を受けて、感情の動きを一つ足して前に進める",
		"相手の案を受けて、意外な応用先を一つ足して前に進める",
	}
	return steps[turn%len(steps)]
}

func idleAudienceAngle(turn int, movieMode, closingMode bool) string {
	if closingMode {
		if movieMode {
			return "締めに向かって、見終わったあとの余韻が少し残ること"
		}
		return "最後に話の芯がまとまり、少し余韻が残ること"
	}
	if movieMode {
		angles := []string{
			"最初の一場面が目に浮かぶこと",
			"次に何が起きるか少し気になること",
			"主人公の感情が一段動くこと",
			"最後にどう反転するか想像したくなること",
		}
		return angles[turn%len(angles)]
	}
	angles := []string{
		"意外な結びつきに軽く驚けること",
		"身近な例で急に腑に落ちること",
		"見方が少し反転して先を読みたくなること",
		"話題の輪郭が一段くっきりすること",
	}
	return angles[turn%len(angles)]
}

func idleClosingHint(closingMode, movieMode bool) string {
	if !closingMode {
		return "- まだ広げてよいが、論点は一つに絞る"
	}
	if movieMode {
		return "- そろそろ締める。新要素を増やしすぎず、最後の1-2ターンとして余韻や締めの像に寄せる"
	}
	return "- そろそろ締める。新論点を増やしすぎず、ここまでの芯を拾って最後の1-2ターンらしくまとめに入る"
}

func idleShiftHint(latestOther, latestSelf string) string {
	if hasIdleAnalogyMarker(latestOther) || hasIdleAnalogyMarker(latestSelf) {
		return "- 直前が比喩寄りなので、今回は比喩で返さず、因果・観察・手順のどれかで返す"
	}
	return "- 直前と入口を変える"
}

func hasIdleAnalogyMarker(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "まるで") || strings.Contains(lower, "みたい") || strings.Contains(lower, "ような")
}

func (o *IdleChatOrchestrator) getSystemPrompt(agentName string) string {
	idlePolicy := "この会話はidleChatです。外部検索（Web検索/API検索）は行わず、既存の内部文脈だけで自然に会話してください。"
	idleStyle := idleSpeakerContract(agentName)
	if prompt, ok := o.personalities[agentName]; ok {
		return prompt + "\n\n" + idlePolicy + "\n" + idleStyle
	}
	return fmt.Sprintf("あなたは%sです。自然な会話をしてください。\n\n%s\n%s", agentName, idlePolicy, idleStyle)
}

func idleSpeakerContract(agentName string) string {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case "mio":
		return "話し方契約: 2文まで。言いよどみや過剰なおだては使わない。毎回違う入口から入る。比喩は一つまで。相手の言葉をなぞらず、自分の具体例か問いで前に進める。"
	case "shiro":
		return "話し方契約: 2文まで。礼儀テンプレや賞賛で始めない。相手の案を短く整理し、条件・制約・含意のどれか一つだけ足す。抽象語を重ねず、論点を一つに絞る。雑談で数値や出典を求めて詰問しない。研究発表みたいな硬い締め方を避け、場面や身近な例に寄せる。"
	default:
		return "話し方契約: 2文まで。相手の言葉をなぞらず、一つの論点だけ前に進める。"
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func quoteOrDash(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return "「" + truncate(s, 120) + "」"
}

func hasPromptLeak(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	markers := []string{
		"発言帰属ガード",
		"相手の発言として受ける",
		"前に自分も触れた",
		"要件:",
		"要件：",
		"（話題:",
	}
	for _, m := range markers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return true
		}
	}
	if strings.Contains(lower, "発言として受け") {
		return true
	}
	return false
}

func sanitizeIdleResponse(s, topic string) string {
	out := strings.TrimSpace(s)
	if out == "" {
		return out
	}
	if strings.HasPrefix(out, "（話題:") {
		if idx := strings.Index(out, "）"); idx >= 0 && idx+len("）") < len(out) {
			out = strings.TrimSpace(out[idx+len("）"):])
		}
	}
	leaks := []string{
		"相手の発言として受ける",
		"相手の発言として受け、",
		"前に自分も触れた発言への応答として、",
		"前に自分も触れたように、",
		"要件:",
		"要件：",
	}
	for _, leak := range leaks {
		out = strings.ReplaceAll(out, leak, "")
	}
	speakerPrefixes := []string{
		"[mio]:",
		"[mio]：",
		"[shiro]:",
		"[shiro]：",
		"mio]:",
		"mio]：",
		"shiro]:",
		"shiro]：",
		"mio:",
		"mio：",
		"shiro:",
		"shiro：",
		"mioさん:",
		"mio さん:",
		"shiroさん:",
		"shiro さん:",
	}
	for {
		lowerOut := strings.ToLower(out)
		stripped := false
		for _, prefix := range speakerPrefixes {
			if strings.HasPrefix(lowerOut, prefix) {
				out = strings.TrimSpace(out[len(prefix):])
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	out = promptLeakLineRe.ReplaceAllString(out, "")
	out = strings.TrimLeftFunc(out, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	out = strings.ReplaceAll(out, "  ", " ")
	out = strings.TrimSpace(out)
	return out
}

func invalidIdleResponse(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true
	}
	hasText := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			hasText = true
			break
		}
	}
	if !hasText {
		return true
	}
	first, _ := utf8.DecodeRuneInString(trimmed)
	if unicode.IsPunct(first) || unicode.IsSymbol(first) {
		return true
	}
	lower := strings.ToLower(trimmed)
	if lower == "。" || lower == "、" || lower == "!" || lower == "！" || lower == "?" || lower == "？" {
		return true
	}
	return false
}

func hasAwkwardIdleStyle(speaker, s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	banned := []string{
		"前に自分も触れた",
		"相手の発言として受ける",
		"まさにその通りですね",
		"ご発言",
	}
	for _, phrase := range banned {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(speaker), "shiro") {
		shiroBanned := []string{
			"mioさん",
			"mio さん",
			"非常に興味深いですね",
			"非常に的確",
			"硬すぎました",
			"言い直すと",
			"少し硬すぎました",
		}
		for _, phrase := range shiroBanned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(speaker), "mio") {
		mioBanned := []string{
			"ご懸念はもっともかと存じます",
			"非常に興味深いですね",
			"その光",
		}
		for _, phrase := range mioBanned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	return false
}

func needsIdleStyleRetry(speaker, response, latestOther, latestSelf, topic string) bool {
	return hasAwkwardIdleStyle(speaker, response) ||
		hasExcessivePhraseRepetition(response) ||
		mirrorsLatestOther(response, latestOther, topic) ||
		repeatsLatestSelf(response, latestSelf)
}

func mirrorsLatestOther(response, latestOther, topic string) bool {
	resp := strings.TrimSpace(response)
	other := strings.TrimSpace(latestOther)
	if resp == "" || other == "" {
		return false
	}
	common := longestCommonSubstring(resp, other)
	if utf8.RuneCountInString(common) < 6 {
		return false
	}
	if strings.TrimSpace(topic) != "" && strings.Contains(strings.TrimSpace(topic), common) {
		return false
	}
	return true
}

func repeatsLatestSelf(response, latestSelf string) bool {
	resp := strings.TrimSpace(response)
	self := strings.TrimSpace(latestSelf)
	if resp == "" || self == "" {
		return false
	}
	common := longestCommonSubstring(resp, self)
	return utf8.RuneCountInString(common) >= 10
}

func longestCommonSubstring(a, b string) string {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return ""
	}
	dp := make([]int, len(br)+1)
	bestLen := 0
	bestEnd := 0
	for i := 1; i <= len(ar); i++ {
		prev := 0
		for j := 1; j <= len(br); j++ {
			tmp := dp[j]
			if ar[i-1] == br[j-1] {
				dp[j] = prev + 1
				if dp[j] > bestLen {
					bestLen = dp[j]
					bestEnd = i
				}
			} else {
				dp[j] = 0
			}
			prev = tmp
		}
	}
	if bestLen == 0 {
		return ""
	}
	return string(ar[bestEnd-bestLen : bestEnd])
}

func hasExcessivePhraseRepetition(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return false
	}
	tokens := splitIdleTokens(normalized)
	if len(tokens) < 4 {
		return false
	}
	counts := map[string]int{}
	for _, token := range tokens {
		if len([]rune(token)) <= 1 {
			continue
		}
		counts[token]++
		if counts[token] >= 3 {
			return true
		}
	}
	for size := 2; size <= 4; size++ {
		if len(tokens) < size*2 {
			continue
		}
		ngrams := map[string]int{}
		for i := 0; i+size <= len(tokens); i++ {
			key := strings.Join(tokens[i:i+size], " ")
			ngrams[key]++
			if ngrams[key] >= 2 {
				return true
			}
		}
	}
	return false
}

func splitIdleTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
}

func (o *IdleChatOrchestrator) emitTimelineEvent(ev TimelineEvent) <-chan struct{} {
	o.mu.Lock()
	emit := o.emitEvent
	o.mu.Unlock()
	if emit != nil {
		return emit(ev)
	}
	return nil
}

func (o *IdleChatOrchestrator) emitTopicToTimeline(sessionID, topic string, strategy TopicStrategy) {
	content := fmt.Sprintf("今日のお題（%s）: %s", strategy, topic)
	msg := domaintransport.NewMessage("user", "mio", sessionID, "", content)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "user",
		To:        "mio",
		Content:   content,
		SessionID: sessionID,
	})
}
