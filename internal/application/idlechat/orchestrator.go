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
	idleCheckInterval         = 30 * time.Second
	maxTurnsPerTopic          = 12
	idleChatResponseMaxTokens = 512
	speakerBreak              = 500 * time.Millisecond  // 話者交代ブレイク（TTS完了後）
	topicBreak                = 1000 * time.Millisecond // 話題交代ブレイク（TTS完了後）
)

var idleChatTTSWaitTimeout = 35 * time.Second
var idleChatLLMGenerateTimeout = 45 * time.Second

var jst = time.FixedZone("JST", 9*60*60)
var randSeedOnce sync.Once
var errIdleInvalidResponse = errors.New("idlechat invalid response")
var promptLeakLineRe = regexp.MustCompile(`(?i)(^|[。．\n])[^。．\n]{0,30}(発言として受け|要件[:：]|発言帰属ガード)[^。．\n]*`)

type SessionSummary struct {
	SessionID         string        `json:"session_id"`
	Title             string        `json:"title"`
	Topic             string        `json:"topic"`
	Strategy          TopicStrategy `json:"strategy"` // 生成戦略（旧 Category）
	Summary           string        `json:"summary"`
	QualityReview     string        `json:"quality_review,omitempty"`
	PromptGuidance    string        `json:"prompt_guidance,omitempty"`
	SourceTitle       string        `json:"source_title,omitempty"`
	RewriteStyle      string        `json:"rewrite_style,omitempty"`
	StoryTitle        string        `json:"story_title,omitempty"`
	StoryText         string        `json:"story_text,omitempty"`
	StoryDraftText    string        `json:"story_draft_text,omitempty"`
	StoryRevisionNote string        `json:"story_revision_note,omitempty"`
	StoryEndingFlavor string        `json:"story_ending_flavor,omitempty"`
	StartedAt         string        `json:"started_at"`
	EndedAt           string        `json:"ended_at"`
	Turns             int           `json:"turns"`
	LoopRestarted     bool          `json:"loop_restarted"`
	LoopReason        string        `json:"loop_reason,omitempty"`
	TopicProvider     string        `json:"topic_provider"`
	SummaryProvider   string        `json:"summary_provider"`
	Transcript        []string      `json:"transcript,omitempty"`
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
	participants     []string
	intervalMin      int
	interval         time.Duration
	maxTurns         int
	temperature      float64
	personalities    map[string]string

	lastActivity  time.Time
	chatActive    bool
	chatBusy      bool
	workerBusy    bool
	manualMode    bool
	sessionMode   string
	currentTopic  string
	promptGuides  []string
	autoStep      int
	forecastStep  int
	nextTopicAt   time.Time
	history       []SessionSummary
	emitEvent     func(TimelineEvent) <-chan struct{}
	topicStore    *TopicStore
	topicStockBuf *forecastTopicStock // 未来展望お題ストック
	recentTopics  func(context.Context, int) ([]string, error)

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

type idleSessionPlan struct {
	mode     string
	strategy TopicStrategy
	domain   *ForecastDomain
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
	o.promptGuides = promptGuidesFromHistory(o.history, 5)
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
	storyDataDir string,
) *IdleChatOrchestrator {
	randSeedOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})
	// LoadStoryData: Complex Story Mode用、現在はアーカイブ済み
	// Simple Story Mode はハードコードされた昔話リストを使用
	_ = storyDataDir // unused
	ctx, cancel := context.WithCancel(context.Background())
	return &IdleChatOrchestrator{
		llmProvider:   llmProvider,
		speakerLLMs:   make(map[string]llm.LLMProvider),
		memory:        memory,
		participants:  participants,
		intervalMin:   intervalMin,
		interval:      time.Duration(intervalMin) * time.Minute,
		maxTurns:      maxTurns,
		temperature:   temperature,
		personalities: personalities,
		lastActivity:  time.Now(),
		history:       make([]SessionSummary, 0, 32),
		ctx:           ctx,
		cancel:        cancel,
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
	log.Printf("[IdleChat] Started (participants=%v, interval=%s, maxTurns=%d)",
		o.participants, o.interval, o.maxTurns)
}

func (o *IdleChatOrchestrator) SetIntervalSeconds(seconds int) {
	if seconds < 1 {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.interval = time.Duration(seconds) * time.Second
	o.intervalMin = (seconds + 59) / 60
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
		o.sessionMode = ""
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
			o.sessionMode = ""
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
			o.sessionMode = ""
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

// StartForecastMode switches from manual idlechat into forecast mode immediately.
func (o *IdleChatOrchestrator) StartForecastMode() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.participants) < 2 {
		return fmt.Errorf("idlechat requires at least 2 participants")
	}
	if o.chatActive {
		return fmt.Errorf("chat session already active")
	}
	o.manualMode = false
	o.chatActive = true
	o.sessionMode = "forecast"
	o.currentTopic = idleChatPendingTopic("forecast")
	o.sessionContext = ""
	o.lastActivity = time.Now()
	log.Println("[Forecast] Forecast mode started")
	return nil
}

// StartStoryMode switches from manual idlechat into story mode immediately.
func (o *IdleChatOrchestrator) StartStoryMode() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.participants) < 2 {
		return fmt.Errorf("idlechat requires at least 2 participants")
	}
	if o.chatActive {
		return fmt.Errorf("chat session already active")
	}
	o.manualMode = false
	o.chatActive = true
	o.sessionMode = "story"
	o.currentTopic = idleChatPendingTopic("story")
	o.sessionContext = ""
	o.lastActivity = time.Now()
	log.Println("[Story] Story mode started")
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
	o.sessionMode = ""
	o.currentTopic = ""
	o.sessionContext = ""
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

// CurrentMode returns the current idlechat/forecast mode.
func (o *IdleChatOrchestrator) CurrentMode() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.chatActive && o.sessionMode != "" {
		return o.sessionMode
	}
	if o.manualMode {
		return "manual"
	}
	return ""
}

// CurrentTopic は現在のIdleChatトピックを返す。
func (o *IdleChatOrchestrator) CurrentTopic() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if strings.TrimSpace(o.currentTopic) == "" && o.chatActive {
		return idleChatPendingTopic(o.sessionMode)
	}
	return o.currentTopic
}

func idleChatPendingTopic(mode string) string {
	switch strings.TrimSpace(mode) {
	case "forecast":
		return "未来展望のお題を準備中"
	case "story", "story-simple":
		return "物語のお題を準備中"
	default:
		return "今日のお題を準備中"
	}
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

func (o *IdleChatOrchestrator) getHistoricalTitleThemes(limit int) []string {
	history := o.GetHistory(limit)
	if len(history) == 0 {
		return nil
	}
	themes := make([]string, 0, len(history))
	seen := make(map[string]struct{}, len(history))
	for _, item := range history {
		theme := themeFromSummaryTitle(item.Title)
		if theme == "" {
			continue
		}
		key := normalizeLoopText(theme)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		themes = append(themes, theme)
	}
	return themes
}

func themeFromSummaryTitle(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return ""
	}
	if idx := strings.Index(s, "日の"); idx >= 0 {
		s = strings.TrimSpace(s[idx+len("日の"):])
	}
	for _, suffix := range []string{"の話題まとめ", "のまとめ", "まとめ"} {
		s = strings.TrimSpace(strings.TrimSuffix(s, suffix))
	}
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end >= 0 {
			s = strings.TrimSpace(s[end+1:])
		}
	}
	return normalizeIdleTopic(s, false)
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
			go o.checkAndStartChat()
		}
	}
}

func (o *IdleChatOrchestrator) checkAndStartChat() {
	o.mu.Lock()
	idleDuration := time.Since(o.lastActivity)
	threshold := o.interval
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
	plan := o.nextIdleSessionPlanLocked()
	o.sessionMode = plan.mode
	o.mu.Unlock()

	log.Printf("[IdleChat] Idle for %v, starting %s session", idleDuration.Round(time.Second), plan.mode)
	switch plan.mode {
	case "forecast":
		if plan.domain == nil {
			log.Printf("[Forecast] Missing domain in session plan, skipping")
		} else {
			o.runForecastDomainSession(*plan.domain)
		}
	case "story-simple":
		o.RunSimpleStorySession()
	default:
		o.runChatSession(plan.strategy)
	}

	o.mu.Lock()
	o.chatActive = false
	o.sessionMode = ""
	o.currentTopic = ""
	o.lastActivity = time.Now() // セッション終了でアイドル計測をリセット
	o.mu.Unlock()
}

func (o *IdleChatOrchestrator) nextIdleSessionPlanLocked() idleSessionPlan {
	normalStrategies := []TopicStrategy{
		StrategySingleGenre,
		StrategyDoubleGenre,
		StrategyExternalStimulus,
	}
	if o.autoStep < len(normalStrategies) {
		plan := idleSessionPlan{
			mode:     "idle",
			strategy: normalStrategies[o.autoStep],
		}
		o.autoStep++
		return plan
	}
	domain := forecastDomains[o.forecastStep%len(forecastDomains)]
	o.forecastStep = (o.forecastStep + 1) % len(forecastDomains)
	o.autoStep = 0
	return idleSessionPlan{
		mode:   "forecast",
		domain: &domain,
	}
}

func (o *IdleChatOrchestrator) runChatSession(strategy TopicStrategy) {
	sessionID := fmt.Sprintf("idle-%d", time.Now().Unix())
	startedAt := time.Now().In(jst)
	remainingTurns := o.maxTurns
	totalTurns := 0
	topicIndex := 0

	for remainingTurns > 0 {
		segmentID := fmt.Sprintf("%s-topic-%02d", sessionID, topicIndex)
		topicIndex++
		topic, strategy := o.generateTopicFromChat(segmentID, strategy)
		o.mu.Lock()
		o.currentTopic = topic
		o.mu.Unlock()
		log.Printf("[IdleChat] Topic: %s (%s, session=%s)", topic, strategy, segmentID)
		o.emitTopicToTimeline(segmentID, topic, strategy)

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

			response, err := o.generateResponse(speaker, nextSpeaker, segmentID, turn, segmentTurns, topic)
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

			msg := domaintransport.NewMessage(speaker, nextSpeaker, segmentID, "", response)
			msg.Type = domaintransport.MessageTypeIdleChat
			o.memory.RecordMessage(msg)
			o.emitTimelineEvent(TimelineEvent{
				Type:      "idlechat.message",
				From:      speaker,
				To:        nextSpeaker,
				Content:   response,
				SessionID: segmentID,
			})
			transcript = append(transcript, fmt.Sprintf("%s: %s", speaker, response))
			segmentTurns++

			log.Printf("[IdleChat] [Turn %d] %s→%s: %s", turn, speaker, nextSpeaker, truncate(response, 80))
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
		totalTurns += segmentTurns
		endedAt := time.Now().In(jst)
		if segmentTurns > 0 {
			displayStrategy := TopicStrategy(fmt.Sprintf("%s: %s", strategy, truncate(topic, 30)))
			summary := o.saveSummary(segmentID, topic, displayStrategy, transcript, startedAt, endedAt, segmentTurns, loopDetected || sessionInterrupted || generationFailed, loopReason)
			o.speakSummary(segmentID, summary)
		}
		cooldown := topicBreak
		if sessionInterrupted || generationFailed {
			idleCooldown := o.interval
			if idleCooldown > cooldown {
				cooldown = idleCooldown
			}
		}
		o.mu.Lock()
		o.nextTopicAt = endedAt.Add(cooldown)
		o.mu.Unlock()

		if segmentTurns == 0 || sessionInterrupted || generationFailed || remainingTurns <= 0 {
			break
		}
		log.Printf("[IdleChat] Switching topic after %d turns (%d remaining)", segmentTurns, remainingTurns)
		o.waitBreak(cooldown)
	}

	log.Printf("[IdleChat] Session %s completed (%d turns)", sessionID, totalTurns)
}

// waitForTTSDone はTTS完了チャネルを待つ。nilなら即座に返る。
func (o *IdleChatOrchestrator) waitForTTSDone(ch <-chan struct{}) {
	if ch == nil {
		return
	}
	timeout := idleChatTTSWaitTimeout
	if timeout <= 0 {
		select {
		case <-o.ctx.Done():
		case <-ch:
		}
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-o.ctx.Done():
		return
	case <-ch:
	case <-timer.C:
		log.Printf("[IdleChat] TTS completion wait timed out after %s; continuing conversation", timeout)
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

func (o *IdleChatOrchestrator) generateTopicFromChat(sessionID string, strategy TopicStrategy) (string, TopicStrategy) {
	movieMode := rand.Intn(100) < 20
	recentTopics := o.getRecentTopics(12)

	var prompt string
	var logInfo string
	var fallbackTopic string

	switch strategy {
	case StrategySingleGenre:
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v anchor=%s", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", anchor, movieMode)

	case StrategyDoubleGenre:
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateDoubleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("double:%v anchor=%s", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", anchor, movieMode)

	case StrategyExternalStimulus:
		var source string
		prompt, source = generateExternalPrompt(movieMode)
		logInfo = fmt.Sprintf("external:%s", source)
		fallbackTopic = fallbackTopicForStrategy(strategy, nil, source, "", topicAnchor{}, movieMode)

	default:
		// Fallback to single genre
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v anchor=%s (fallback)", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(StrategySingleGenre, genres, "", "", anchor, movieMode)
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
			{Role: "system", Content: idleTopicGeneratorSystemPrompt()},
			{Role: "user", Content: prompt},
		}
		req := llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   420,
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

func fallbackTopicForStrategy(strategy TopicStrategy, genres []string, source string, seed string, anchor topicAnchor, movieMode bool) string {
	anchorValue := strings.TrimSpace(anchor.Value)
	switch strategy {
	case StrategySingleGenre:
		if len(genres) >= 1 && strings.TrimSpace(genres[0]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "の裏側")
			}
			if anchorValue != "" {
				return fmt.Sprintf("%sを%sの視点から考える", genres[0], anchorValue)
			}
			return fmt.Sprintf("%sで見落としがちな判断基準", genres[0])
		}
	case StrategyDoubleGenre:
		if len(genres) >= 2 && strings.TrimSpace(genres[0]) != "" && strings.TrimSpace(genres[1]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "と" + genres[1])
			}
			if anchorValue != "" {
				return fmt.Sprintf("%sと%sを%sでつなぐ", genres[0], genres[1], anchorValue)
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
	s := strings.TrimSpace(extractVisibleLLMAnswer(raw))
	if s == "" {
		return ""
	}
	if hasPromptLeak(s) || hasInternalReasoningLeak(s) {
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
	s = extractTopicTitleFromConversationalText(s)

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
	if s == "" || hasPromptLeak(s) || hasInternalReasoningLeak(s) || strings.HasPrefix(strings.TrimSpace(s), "<") || looksTruncatedIdleTopic(s) {
		return ""
	}
	if movieMode {
		return formatMovieTopicPrompt(s)
	}
	return strings.TrimSpace(s)
}

func looksTruncatedIdleTopic(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.HasSuffix(s, "、") || strings.HasSuffix(s, ",") {
		return true
	}
	for _, suffix := range []string{"そして", "また", "から", "ため", "との", "への", "取り", "取"} {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	if idx := strings.LastIndexAny(s, "、,"); idx >= 0 {
		tail := []rune(strings.TrimSpace(s[idx+len("、"):]))
		if len(tail) > 0 && len(tail) <= 2 {
			return true
		}
	}
	return false
}

func idleTopicGeneratorSystemPrompt() string {
	return `あなたはRenCrowのidleChat用お題生成器です。
キャラクターとして会話せず、感想・相づち・呼びかけ・絵文字を出さないでください。
出力はユーザーが指定した条件に合う「お題」本文だけを1行で返してください。`
}

func extractTopicTitleFromConversationalText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "「」『』\"' ")
	s = trimLeadingTopicReaction(s)
	for _, marker := range []string{"って組み合わせ", "という組み合わせ"} {
		if idx := strings.Index(s, marker); idx > 0 {
			return strings.TrimSpace(strings.Trim(s[:idx], "「」『』\"' "))
		}
	}
	for _, marker := range []string{"めっちゃ", "すごく", "なんか物語", "物語になりそう", "エモい"} {
		if idx := strings.Index(s, marker); idx > 0 {
			s = strings.TrimSpace(strings.TrimRight(s[:idx], "、。！？!? "))
			break
		}
	}
	return strings.TrimSpace(strings.Trim(s, "「」『』\"' "))
}

func trimLeadingTopicReaction(s string) string {
	for {
		trimmed := strings.TrimSpace(s)
		cut := -1
		for _, mark := range []string{"！", "!", "？", "?"} {
			if idx := strings.Index(trimmed, mark); idx >= 0 && utf8.RuneCountInString(trimmed[:idx]) < 40 {
				if cut == -1 || idx < cut {
					cut = idx
				}
			}
		}
		if cut < 0 {
			return trimmed
		}
		prefix := trimmed[:cut]
		if !containsAny(prefix, "えー", "うーん", "わあ", "おお", "なるほど", "たしかに") {
			return trimmed
		}
		s = strings.TrimSpace(trimmed[cut+len(string([]rune(trimmed[cut:])[0])):])
	}
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
	if reason := detectShortLoopReason(transcript); reason != "" {
		return reason
	}
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

func detectShortLoopReason(transcript []string) string {
	if len(transcript) < 4 {
		return ""
	}
	if hasShortAlternatingLoop(transcript) {
		return "short_alternating_repeat"
	}
	if hasShortSpeakerTemplateLoop(transcript) {
		return "short_template_repeat"
	}
	if hasShortHighSimilarityLoop(transcript) {
		return "short_high_similarity"
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
	qualityReview, promptGuidance := o.reviewSessionEnd(topic, string(strategy), transcript, summary, loopReason)
	title := fmt.Sprintf("%d月%d日の%sの話題まとめ", endedAt.Month(), endedAt.Day(), truncate(topic, 24))
	record := SessionSummary{
		SessionID:       sessionID,
		Title:           title,
		Topic:           topic,
		Strategy:        strategy,
		Summary:         summary,
		QualityReview:   qualityReview,
		PromptGuidance:  promptGuidance,
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
	o.addPromptGuideLocked(promptGuidance)
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

// speakSummary は Mio にまとめを読み上げさせる。会話進行は TTS 完了を待たない。
func (o *IdleChatOrchestrator) speakSummary(sessionID, summary string) {
	if strings.TrimSpace(summary) == "" {
		return
	}
	o.waitBreak(topicBreak)
	spokenSummary := "今回のまとめです。\n" + strings.TrimSpace(summary)
	msg := domaintransport.NewMessage("mio", "user", sessionID, "", spokenSummary)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "user",
		Content:   spokenSummary,
		SessionID: sessionID,
	})
	log.Printf("[IdleChat] Mio reading summary: %s", truncate(spokenSummary, 80))
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
	case "short_template_repeat":
		return "短周期テンプレ反復で即打ち切り"
	case "short_alternating_repeat":
		return "短周期の交互反復で即打ち切り"
	case "short_high_similarity":
		return "短周期の類似反復で即打ち切り"
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
	topic = o.resolveDialogueTopic(sessionID, speaker, topic)
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
			Content: buildIdleTurnPrompt(topic, speaker, "", "", turn, segmentTurns, true),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildIdleTurnPrompt(topic, speaker, latestOther, latestSelf, turn, segmentTurns, false),
		})
	}

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   idleChatResponseMaxTokens,
		Temperature: temp,
	}

	provider := o.providerForSpeaker(speaker)
	resp, err := o.generateIdleLLM(provider, req)
	if err != nil {
		log.Printf("[IdleChat] LLM generate primary failed (%s turn=%d): %v", speaker, turn, err)
		return fallbackIdleResponse(speaker, topic, latestOther, turn), nil
	}
	firstRaw := strings.TrimSpace(resp.Content)
	first := sanitizeIdleResponse(resp.Content, topic)
	firstTruncated := finishReasonLooksTruncated(resp.FinishReason)
	if firstTruncated || unusableIdleResponse(firstRaw, first) {
		retryInvalid := append([]llm.Message{}, messages...)
		retryInvalid = append(retryInvalid, llm.Message{
			Role:    "user",
			Content: "今の返答は無効です。内部推論、<|channel|>形式、箇条書き、指示文、自己説明、途中で切れた文を一切書かず、発話としてそのまま読める自然な会話文だけを1-2文で言い直してください。",
		})
		respInvalid, errInvalid := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryInvalid,
			MaxTokens:   idleChatResponseMaxTokens,
			Temperature: temp,
		})
		if errInvalid != nil {
			log.Printf("[IdleChat] retryInvalid failed (%s turn=%d): %v", speaker, turn, errInvalid)
		}
		if errInvalid == nil && strings.TrimSpace(respInvalid.Content) != "" {
			first = sanitizeIdleResponse(respInvalid.Content, topic)
			firstRaw = strings.TrimSpace(respInvalid.Content)
			firstTruncated = finishReasonLooksTruncated(respInvalid.FinishReason)
		}
	}
	if needsIdleStyleRetry(speaker, first, latestOther, latestSelf, topic) {
		retryStyle := append([]llm.Message{}, messages...)
		retryStyle = append(retryStyle, llm.Message{
			Role:    "user",
			Content: "評価や言い直し宣言は書かず、別の手で自然に返してください。直前の言い回しをなぞらず、1文目で反応し、2文目で新しい具体例か問いを一つだけ足してください。",
		})
		respStyle, errStyle := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryStyle,
			MaxTokens:   idleChatResponseMaxTokens,
			Temperature: temp,
		})
		if errStyle != nil {
			log.Printf("[IdleChat] retryStyle failed (%s turn=%d): %v", speaker, turn, errStyle)
		}
		if errStyle == nil && strings.TrimSpace(respStyle.Content) != "" {
			first = sanitizeIdleResponse(respStyle.Content, topic)
			firstRaw = strings.TrimSpace(respStyle.Content)
			firstTruncated = finishReasonLooksTruncated(respStyle.FinishReason)
		}
	}
	if hasPromptLeak(firstRaw) || hasPromptLeak(first) || hasInternalReasoningLeak(firstRaw) || hasInternalReasoningLeak(first) {
		retryLeak := append([]llm.Message{}, messages...)
		retryLeak = append(retryLeak, llm.Message{
			Role:    "user",
			Content: "指示文や内部推論の断片を消して、自然な会話文だけを1-2文で言い直してください。メタ表現、箇条書き、分析文は禁止です。",
		})
		respLeak, errLeak := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryLeak,
			MaxTokens:   idleChatResponseMaxTokens,
			Temperature: temp,
		})
		if errLeak != nil {
			log.Printf("[IdleChat] retryLeak failed (%s turn=%d): %v", speaker, turn, errLeak)
		}
		if errLeak == nil && strings.TrimSpace(respLeak.Content) != "" {
			first = sanitizeIdleResponse(respLeak.Content, topic)
			firstRaw = strings.TrimSpace(respLeak.Content)
			firstTruncated = finishReasonLooksTruncated(respLeak.FinishReason)
		}
	}
	if violatesAttribution(first, latestOther) {
		retry := append([]llm.Message{}, messages...)
		retry = append(retry, llm.Message{
			Role:    "user",
			Content: "発言帰属が曖昧です。相手の案を受ける形にして、1-2文で言い直してください。",
		})
		resp2, err2 := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retry,
			MaxTokens:   idleChatResponseMaxTokens,
			Temperature: temp,
		})
		if err2 != nil {
			log.Printf("[IdleChat] retryAttribution failed (%s turn=%d): %v", speaker, turn, err2)
		}
		if err2 == nil && strings.TrimSpace(resp2.Content) != "" {
			candidateRaw := strings.TrimSpace(resp2.Content)
			candidate := sanitizeIdleResponse(resp2.Content, topic)
			if finishReasonLooksTruncated(resp2.FinishReason) || unusableIdleResponse(candidateRaw, candidate) {
				log.Printf("[IdleChat] retryAttribution unusable (%s turn=%d): raw=%q sanitized=%q", speaker, turn, truncate(candidateRaw, 180), truncate(candidate, 180))
				return fallbackOrStopIdleResponse(speaker, topic, latestOther, turn)
			}
			return candidate, nil
		}
	}

	if firstTruncated || unusableIdleResponse(firstRaw, first) {
		log.Printf("[IdleChat] unusable response rejected (%s turn=%d): raw=%q sanitized=%q", speaker, turn, truncate(firstRaw, 180), truncate(first, 180))
		return fallbackOrStopIdleResponse(speaker, topic, latestOther, turn)
	}

	return first, nil
}

func unusableIdleResponse(raw, sanitized string) bool {
	return invalidIdleResponse(sanitized) ||
		hasPromptLeak(raw) ||
		hasPromptLeak(sanitized) ||
		hasInternalReasoningLeak(raw) ||
		hasInternalReasoningLeak(sanitized)
}

func finishReasonLooksTruncated(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

func fallbackOrStopIdleResponse(speaker, topic, latestOther string, turn int) (string, error) {
	if turn >= 8 {
		return "", errIdleInvalidResponse
	}
	return fallbackIdleResponse(speaker, topic, latestOther, turn), nil
}

func (o *IdleChatOrchestrator) resolveDialogueTopic(sessionID, speaker, topic string) string {
	if normalized := strings.TrimSpace(topic); normalized != "" {
		return normalized
	}
	if sessionID != "" {
		for _, entry := range o.memory.GetUnifiedView(24) {
			if entry.Message.SessionID != sessionID {
				continue
			}
			if extracted := extractIdleTopicText(entry.Message.Content); extracted != "" {
				log.Printf("[IdleChat] Empty dialogue topic recovered from session memory: session=%s topic=%q", sessionID, truncate(extracted, 80))
				return extracted
			}
		}
	}
	o.mu.Lock()
	currentTopic := strings.TrimSpace(o.currentTopic)
	o.mu.Unlock()
	if currentTopic != "" && !strings.Contains(currentTopic, "準備中") {
		log.Printf("[IdleChat] Empty dialogue topic recovered from current topic: session=%s topic=%q", sessionID, truncate(currentTopic, 80))
		return currentTopic
	}
	log.Printf("[IdleChat] Empty dialogue topic; using emergency fallback: session=%s speaker=%s", sessionID, speaker)
	return "この会話の現在のお題"
}

func extractIdleTopicText(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	prefixes := []string{"今日のお題", "お題"}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		if idx := strings.IndexAny(s, ":：、,"); idx >= 0 && idx+1 < len(s) {
			return strings.TrimSpace(s[idx+1:])
		}
	}
	return ""
}

func (o *IdleChatOrchestrator) generateIdleLLM(provider llm.LLMProvider, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	if provider == nil {
		return llm.GenerateResponse{}, fmt.Errorf("idlechat LLM provider is nil")
	}
	timeout := idleChatLLMGenerateTimeout
	if timeout <= 0 {
		return provider.Generate(o.ctx, req)
	}
	ctx, cancel := context.WithTimeout(o.ctx, timeout)
	defer cancel()
	return provider.Generate(ctx, req)
}

func fallbackIdleResponse(speaker, topic, latestOther string, turn int) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "この話題"
	}
	topicShort := truncate(topic, 42)
	other := truncate(strings.TrimSpace(latestOther), 34)
	isShiro := strings.EqualFold(strings.TrimSpace(speaker), "shiro")
	variants := turn % 8
	if isShiro {
		switch variants {
		case 0:
			return fmt.Sprintf("そのお題なら、まず%sを一つの場面に絞ると話が進みます。棚や通路みたいな具体物を置くと、見え方が安定します。", topicShort)
		case 1:
			if other != "" {
				return fmt.Sprintf("今の「%s」は入口として使えますね。次は誰が何を見落としたのか、一点だけ決めると輪郭が出ます。", other)
			}
			return fmt.Sprintf("%sは抽象のままだと散るので、最初の発見を一つ決めたいです。小さな違和感から始めるのがよさそうです。", topicShort)
		case 2:
			return fmt.Sprintf("%sでは、人物の動きより先に場所のルールを決めると整理できます。何が普通で、何が一度だけズレたのかを見たいですね。", topicShort)
		case 3:
			return fmt.Sprintf("ここは結論を急がず、%sを触れる物に落としましょう。音、匂い、置き場所のどれか一つが次の手がかりになります。", topicShort)
		case 4:
			return fmt.Sprintf("%sを追うなら、誰か一人の習慣を決めるのがよさそうです。同じ動きが一度だけ崩れると、会話の焦点になります。", topicShort)
		case 5:
			return fmt.Sprintf("視点を少し狭めると、%sは観察記録として扱えます。最初に残る痕跡を一つ選ぶと、次の問いが自然に出ます。", topicShort)
		case 6:
			return fmt.Sprintf("その方向なら、場所の明るさや足音の変化を使えます。%sを説明ではなく、誰かが気づく瞬間に寄せたいですね。", topicShort)
		default:
			return fmt.Sprintf("いま必要なのは、%sの中で変化する一点を決めることです。人、物、時間のどれが先にズレるかで展開が変わります。", topicShort)
		}
	}
	switch variants {
	case 0:
		return fmt.Sprintf("えー、%sって、最初に小さな違和感を一つ置くと一気に見えそうだね。たとえば誰かがいつもと違う場所で立ち止まる場面から始めたいな。", topicShort)
	case 1:
		if other != "" {
			return fmt.Sprintf("その「%s」って手がかり、けっこう効きそう。じゃあ次は、それを最初に見つける人の表情から決めてみない？", other)
		}
		return fmt.Sprintf("いいじゃん、%sなら人の癖が見える瞬間から入りたいな。何気ない動きが、あとで意味を持つ感じにしたい。", topicShort)
	case 2:
		return fmt.Sprintf("%s、ただ説明するより一場面で見せたいね。古い照明が一瞬だけ揺れる、みたいな合図があると話が動きそう。", topicShort)
	case 3:
		return fmt.Sprintf("気になるのは、%sの中で誰が最初に違和感へ気づくかだね。そこを決めたら、会話も自然に前へ進みそう。", topicShort)
	case 4:
		return fmt.Sprintf("いいね、%sなら最初の手がかりをすごく小さくしたいな。落ちている紙片とか、匂いが一瞬変わるとか、そのくらいが効きそう。", topicShort)
	case 5:
		return fmt.Sprintf("%sって、誰かのいつもの癖がズレるだけで話になりそう。そこから『今日は何か違う』って空気を作りたい。", topicShort)
	case 6:
		return fmt.Sprintf("それなら、%sを一人の目線で追うのがよさそうだね。見慣れた場所の一箇所だけが変わっていて、そこから会話が動く感じ。", topicShort)
	default:
		return fmt.Sprintf("じゃあ%sは、最後に大きな説明を置くより、最初に触れる物を決めたいな。その物が誰の記憶につながるかで広げられそう。", topicShort)
	}
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

func hasShortAlternatingLoop(transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	a := normalizeLoopText(transcript[len(transcript)-1])
	b := normalizeLoopText(transcript[len(transcript)-2])
	c := normalizeLoopText(transcript[len(transcript)-3])
	d := normalizeLoopText(transcript[len(transcript)-4])
	if a == "" || b == "" || c == "" || d == "" {
		return false
	}
	return textSimilarity(a, c) >= 0.9 && textSimilarity(b, d) >= 0.9
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

func hasShortHighSimilarityLoop(transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	start := len(transcript) - 4
	base := make([]string, 0, 4)
	for i := start; i < len(transcript); i++ {
		t := normalizeLoopText(transcript[i])
		if t != "" {
			base = append(base, t)
		}
	}
	if len(base) < 4 {
		return false
	}
	similarPairs := 0
	for i := 0; i < len(base); i++ {
		for j := i + 1; j < len(base); j++ {
			if textSimilarity(base[i], base[j]) >= 0.94 {
				similarPairs++
			}
		}
	}
	return similarPairs >= 3
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

func hasShortSpeakerTemplateLoop(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	type speakerTurn struct {
		speaker string
		text    string
	}
	// 直近6ターンを検査。同一話者3ターン連続一致で発火。
	// 2ターン一致（4ターン窓）は深い議論での誤発火が多いため閾値を上げる。
	turns := make([]speakerTurn, 0, 6)
	for i := len(transcript) - 6; i < len(transcript); i++ {
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
		// 同一話者3ターン分が揃い、かつ最後の3ターンすべて同一パターン
		if len(keys) >= 3 && keys[len(keys)-1] == keys[len(keys)-2] && keys[len(keys)-2] == keys[len(keys)-3] {
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
	// 5文字未満は「確かに」「なるほど」等の短い同意接頭辞。
	// 構造的テンプレートとはみなさず、誤検知を防ぐ。
	if b.Len() < 5 {
		return ""
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
	interest := idleInterestProfileForTopic(topic)
	closingMode := !firstTurn && turnsLeftInTopic(segmentTurns) <= 2
	move := idleTurnMove(speakerOrTarget, turn, firstTurn, movieMode, closingMode)
	audience := idleAudienceAngleForProfile(turn, movieMode, closingMode, interest)
	shiftHint := idleShiftHint(latestOther, latestSelf)
	if firstTurn {
		return fmt.Sprintf(
			"話題: %s\n話題タイプ: %s\n面白さの狙い: %s\n%sとして1-2文で始めてください。\n今回の役割: %s\n読者の楽しみ: %s\n面白さの出し方: %s\nルール:\n- これは独白ではなく二人の対話です。相手が次に返したくなる未完の観点か問いを1つ残す\n- 自然に入る\n- 相手が返しやすい観点か問いを1つ入れる\n- 面白さの狙いから外れる要素を混ぜすぎない\n- 映画お題なら主人公・事件・場面のどれかを出す",
			topic,
			interest.TopicType,
			interest.Name,
			speakerOrTarget,
			move,
			audience,
			interest.Instruction,
		)
	}
	return fmt.Sprintf(
		"話題: %s\n話題タイプ: %s\n面白さの狙い: %s\n%sとして1-2文で返答してください。\n直前の相手発言: %s\n自分の直前発言: %s\n今回の役割: %s\n読者の楽しみ: %s\n面白さの出し方: %s\nルール:\n- これは独白ではなく二人の対話です。直前の相手発言の論点・疑問・具体語のどれかを必ず受けてから返す\n- 1文目は相手の発言への反応、2文目で新しい具体例・理由・問いのどれかを一つだけ足す\n- 直前と同じ比喩の型を繰り返さず、因果・場面・手順のどれかにずらす\n%s\n- 抽象語だけで逃げず、少し具体化する\n- 面白さの狙いから外れる要素を混ぜすぎない\n- 映画お題なら主人公・事件・対立・反転のどれかを進める\n%s",
		topic,
		interest.TopicType,
		interest.Name,
		speakerOrTarget,
		quoteOrDash(latestOther),
		quoteOrDash(latestSelf),
		move,
		audience,
		interest.Instruction,
		shiftHint,
		idleClosingHint(closingMode, movieMode),
	)
}

type idleInterestProfile struct {
	TopicType   string
	Name        string
	Instruction string
	Angles      []string
}

func idleInterestProfileForTopic(topic string) idleInterestProfile {
	normalized := strings.ToLower(strings.TrimSpace(topic))
	if isMovieTopicPrompt(topic) || containsAny(normalized, "映画", "物語", "ストーリー", "脚本", "主人公", "事件", "ラスト", "伏線") {
		return idleInterestProfile{
			TopicType:   "物語・映画",
			Name:        "展開と感情",
			Instruction: "次に何が起きるか気になる要素を一つ置き、人物の感情か場面を少し動かす。",
			Angles: []string{
				"最初の一場面が目に浮かぶこと",
				"次に何が起きるか少し気になること",
				"主人公の感情が一段動くこと",
				"前の要素が後で効きそうに見えること",
			},
		}
	}
	if containsAny(normalized, "技術", "実装", "設計", "運用", "障害", "cli", "api", "repo", "git", "コード", "テスト", "ビルド", "デプロイ", "プロンプト") {
		return idleInterestProfile{
			TopicType:   "技術・運用",
			Name:        "構造と対比",
			Instruction: "原因・分岐点・別案との差のどれか一つを整理し、判断しやすい形にする。",
			Angles: []string{
				"構造が見えて判断しやすくなること",
				"似た案との差が一つはっきりすること",
				"どこが分岐点か見えること",
				"実際に動かす時の落とし穴が一つ見えること",
			},
		}
	}
	if containsAny(normalized, "ニュース", "未来", "予測", "市場", "社会", "政治", "経済", "ai", "生成ai", "トレンド", "来年", "今後") {
		return idleInterestProfile{
			TopicType:   "ニュース・未来予測",
			Name:        "因果と生活への影響",
			Instruction: "大きな話をそのまま語らず、何が変わるかを個人・現場・社会のどれかに落とす。",
			Angles: []string{
				"大きな変化が身近な場面に落ちること",
				"原因と結果のつながりが一段見えること",
				"賛否や勝ち負けの条件が一つ見えること",
				"数か月後の生活や現場が少し想像できること",
			},
		}
	}
	if containsAny(normalized, "日常", "生活", "ごはん", "料理", "睡眠", "散歩", "部屋", "仕事帰り", "休日", "疲れ", "飲み", "雑談") {
		return idleInterestProfile{
			TopicType:   "日常・雑談",
			Name:        "具体と小さな意外性",
			Instruction: "身近な場面や手触りを一つ出し、少しだけ意外な見方か感情を添える。",
			Angles: []string{
				"その場面がすぐ浮かぶこと",
				"小さな違和感や発見で少し笑えること",
				"自分にもありそうだと感じられること",
				"何気ないものの見え方が少し変わること",
			},
		}
	}
	if containsAny(normalized, "架空", "妄想", "もし", "魔法", "異世界", "宇宙", "妖怪", "都市伝説", "存在しない") {
		return idleInterestProfile{
			TopicType:   "架空設定・妄想",
			Name:        "破綻寸前の納得感",
			Instruction: "変な設定を出してよいが、条件や絵面を一つ置いて筋が通るようにする。",
			Angles: []string{
				"変だけど筋は通っていると感じること",
				"一枚絵として強い場面が浮かぶこと",
				"制約があるせいで逆に面白くなること",
				"次の展開を見たくなる不穏さが残ること",
			},
		}
	}
	return idleInterestProfile{
		TopicType:   "探索・一般",
		Name:        "発見と具体化",
		Instruction: "知らなかった見方か意外な接続を一つ出し、抽象論で終わらせず具体例に落とす。",
		Angles: []string{
			"意外な結びつきに軽く驚けること",
			"身近な例で急に腑に落ちること",
			"見方が少し反転して先を読みたくなること",
			"話題の輪郭が一段くっきりすること",
		},
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
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

func idleAudienceAngleForProfile(turn int, movieMode, closingMode bool, profile idleInterestProfile) string {
	if closingMode {
		if movieMode {
			return "締めに向かって、見終わったあとの余韻が少し残ること"
		}
		return "最後に話の芯がまとまり、少し余韻が残ること"
	}
	if len(profile.Angles) == 0 {
		return idleAudienceAngle(turn, movieMode, closingMode)
	}
	return profile.Angles[turn%len(profile.Angles)]
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

	o.mu.Lock()
	mode := o.sessionMode
	promptGuidance := formatPromptGuidance(o.promptGuides)
	o.mu.Unlock()

	var idleStyle string
	if mode == "forecast" {
		idleStyle = forecastSpeakerContract(agentName)
	} else {
		idleStyle = idleSpeakerContract(agentName)
	}

	if prompt, ok := o.personalities[agentName]; ok {
		return prompt + "\n\n" + idlePolicy + "\n" + idleStyle + promptGuidance
	}
	return fmt.Sprintf("あなたは%sです。自然な会話をしてください。\n\n%s\n%s%s", agentName, idlePolicy, idleStyle, promptGuidance)
}

func forecastSpeakerContract(agentName string) string {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case "mio":
		return "話し方契約（未来展望モード）: 3文まで。「確かに」「なるほど」で始めない。具体的な事例・数字・場面を一つ必ず使う。「まるで〜のような」比喩は禁止し、実例か問いで進める。「そんな見方があったのか」と思わせる角度から入る。語尾はタメ口（〜だよね・〜じゃん・〜なんだよね）。「〜です」「〜ます」は禁止。"
	case "shiro":
		return "話し方契約（未来展望モード）: 3文まで。「確かに」「なるほど」「そうですね」で始めない。相手の論点を「それは」「その点は」「別の角度から見ると」などで1文で受ける（直前の自分の発言で使った語句をそのまま主語・書き出しに流用しない）。賛否の対比・条件・具体的な数字のいずれかを一つ加える。抽象論は避け、現場・個人・社会への具体的な影響を述べる。締めは場面の描写か問いかけで終える。"
	default:
		return "話し方契約（未来展望モード）: 3文まで。「確かに」「なるほど」で始めない。具体的な事実・事例・数字を一つ加えて議論を前に進める。"
	}
}

func idleSpeakerContract(agentName string) string {
	switch strings.ToLower(strings.TrimSpace(agentName)) {
	case "mio":
		return "話し方契約: 2〜3文まで。語尾はタメ口（〜だね・〜だよ・〜じゃん・〜なの・〜かも・〜かな・〜っていいよね・〜なんだよね）。「〜です」「〜ます」は絶対禁止。「確かに」「なるほど」「そうだよね」で文を始めない。驚き・共感・好奇心のリアクション（えー！・いいじゃん・それすごくない？・わかる・気になる）を適度に使ってよい。自分の小さな気持ちを1文以内で素直に見せてよい。毎回違う入口から入る。比喩は一つまで。相手の言葉をなぞらず、自分の具体例か問いで前に進める。"
	case "shiro":
		return "話し方契約: 2文まで。「確かに」「なるほど」「そうですね」で文を始めない。礼儀テンプレや賞賛で始めない。相手の案を「それは」「その点は」などで短く受け、条件・制約・含意のどれか一つだけ足す。抽象語を重ねず、論点を一つに絞る。雑談で数値や出典を求めて詰問しない。研究発表みたいな硬い締め方を避け、場面や身近な例に寄せる。「〜は、まるで〜のように」の書き出しは禁止。直前の自分の発言と同じ書き出し・主語で始めない。"
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
		"<|",
		"|>",
		"channel>thought",
		"channel=analysis",
		"analysis to=",
		"assistant to=",
		"発言帰属ガード",
		"相手の発言として受ける",
		"前に自分も触れた",
		"要件:",
		"要件：",
		"（話題:",
		"現在の状況",
		"目標:",
		"目標：",
		"制約事項",
		"会話の制約",
		"システムプロンプト",
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

func extractVisibleLLMAnswer(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	finalMarkers := []string{
		"<|channel|>final",
		"<|channel>final",
		"channel>final",
		"channel=final",
	}
	for _, marker := range finalMarkers {
		if idx := strings.LastIndex(lower, marker); idx >= 0 {
			return trimHarmonyTail(strings.TrimSpace(s[idx+len(marker):]))
		}
	}
	if strings.Contains(lower, "<|channel") || strings.Contains(lower, "channel>thought") || strings.Contains(lower, "channel=analysis") {
		return ""
	}
	return trimHarmonyTail(s)
}

func trimHarmonyTail(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, marker := range []string{"<|end|>", "<|return|>", "<|message|>", "<|endoftext|>"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
			lower = strings.ToLower(s)
		}
	}
	return s
}

func hasInternalReasoningLeak(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	markers := []string{
		"ユーザーは私",
		"私はmioとして",
		"私はshiroとして",
		"mioとして、",
		"shiroとして、",
		"必要がある",
		"遵守する必要",
		"以下の点",
		"会話の制約",
		"キャラクター（",
		"**現在の状況**",
		"**目標**",
		"1. **",
		"2. **",
	}
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 3 {
		bullets := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || regexp.MustCompile(`^\d+[.)．]\s*`).MatchString(line) {
				bullets++
			}
		}
		if bullets >= 2 {
			return true
		}
	}
	return false
}

func sanitizeIdleResponse(s, topic string) string {
	out := strings.TrimSpace(extractVisibleLLMAnswer(s))
	if out == "" {
		return out
	}
	for _, marker := range []string{"<|channel", "channel>thought", "channel=analysis"} {
		if idx := strings.Index(strings.ToLower(out), strings.ToLower(marker)); idx >= 0 {
			out = strings.TrimSpace(out[:idx])
			break
		}
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
		// "Assistant: [speaker]:" 形式（LLMのプロンプトリーク）
		"assistant: [mio]:",
		"assistant: [mio]：",
		"assistant: [shiro]:",
		"assistant: [shiro]：",
		"assistant: mio:",
		"assistant: mio：",
		"assistant: shiro:",
		"assistant: shiro：",
		"assistant:",
		// 通常の speaker prefix
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
