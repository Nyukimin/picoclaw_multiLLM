package idlechat

import (
	"context"
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
	minTopicInterval  = 10 * time.Second // テスト用: 10秒間隔
	ttsCharsPerSecond = 8.0
	ttsMinWait        = 2 * time.Second
	ttsMaxWait        = 20 * time.Second
	maxTurnsPerTopic  = 50
)

var jst = time.FixedZone("JST", 9*60*60)
var randSeedOnce sync.Once
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
	llmProvider    llm.LLMProvider
	memory         *session.CentralMemory
	participants   []string
	intervalMin    int
	maxTurns       int
	temperature    float64
	personalities  map[string]string
	ttsWaitEnabled bool

	lastActivity time.Time
	chatActive   bool
	chatBusy     bool
	workerBusy   bool
	manualMode   bool
	currentTopic string
	nextTopicAt  time.Time
	history      []SessionSummary
	emitEvent    func(TimelineEvent)
	topicStore   *TopicStore

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

// SetEventEmitter sets an optional timeline event emitter used by viewer SSE.
func (o *IdleChatOrchestrator) SetEventEmitter(emit func(TimelineEvent)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitEvent = emit
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
	ttsWaitEnabled := true
	if llmProvider != nil && llmProvider.Name() == "mock" {
		ttsWaitEnabled = false
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &IdleChatOrchestrator{
		llmProvider:    llmProvider,
		memory:         memory,
		participants:   participants,
		intervalMin:    intervalMin,
		maxTurns:       maxTurns,
		temperature:    temperature,
		personalities:  personalities,
		ttsWaitEnabled: ttsWaitEnabled,
		lastActivity:   time.Now(),
		history:        make([]SessionSummary, 0, 32),
		ctx:            ctx,
		cancel:         cancel,
	}
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

			response, err := o.generateResponse(speaker, nextSpeaker, sessionID, turn, topic)
			if err != nil {
				log.Printf("[IdleChat] Generation error: %v", err)
				generationFailed = true
				loopReason = "generation_error"
				break
			}
			if isResponseTooSimilar(response, transcript) {
				loopDetected = true
				loopReason = "pre_emit_similarity"
				log.Printf("[IdleChat] Repetitive response detected before emit, summarize and restart")
				break
			}

			msg := domaintransport.NewMessage(speaker, nextSpeaker, sessionID, "", response)
			msg.Type = domaintransport.MessageTypeIdleChat
			o.memory.RecordMessage(msg)
			o.emitTimelineEvent(TimelineEvent{
				Type:      "idlechat.message",
				From:      speaker,
				To:        nextSpeaker,
				Content:   response,
				SessionID: sessionID,
			})
			transcript = append(transcript, fmt.Sprintf("%s: %s", speaker, response))
			segmentTurns++

			log.Printf("[IdleChat] [Turn %d] %s→%s: %s", turn, speaker, nextSpeaker, truncate(response, 80))
			o.waitForTTS(response)

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
			o.saveSummary(sessionID, topic, strategy, transcript, startedAt, endedAt, segmentTurns, loopDetected || sessionInterrupted || generationFailed, loopReason)
		}
		cooldown := minTopicInterval
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

func estimateTTSWait(content string) time.Duration {
	runes := len([]rune(strings.TrimSpace(content)))
	if runes <= 0 {
		return ttsMinWait
	}
	seconds := float64(runes) / ttsCharsPerSecond
	d := time.Duration(seconds * float64(time.Second))
	if d < ttsMinWait {
		return ttsMinWait
	}
	if d > ttsMaxWait {
		return ttsMaxWait
	}
	return d
}

func (o *IdleChatOrchestrator) waitForTTS(content string) {
	if !o.ttsWaitEnabled {
		return
	}
	wait := estimateTTSWait(content)
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-o.ctx.Done():
		return
	case <-timer.C:
	}
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
	recentTopics := o.getRecentTopics(12)

	var prompt string
	var logInfo string
	var fallbackTopic string

	switch strategy {
	case StrategySingleGenre:
		var genres []string
		prompt, genres = generateSingleGenrePrompt(recentTopics)
		logInfo = fmt.Sprintf("single:%v", genres)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "")

	case StrategyDoubleGenre:
		var genres []string
		prompt, genres = generateDoubleGenrePrompt(recentTopics)
		logInfo = fmt.Sprintf("double:%v", genres)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "")

	case StrategyExternalStimulus:
		var source string
		prompt, source = generateExternalPrompt()
		logInfo = fmt.Sprintf("external:%s", source)
		fallbackTopic = fallbackTopicForStrategy(strategy, nil, source, "")

	default:
		// Fallback to single genre
		var genres []string
		prompt, genres = generateSingleGenrePrompt(recentTopics)
		logInfo = fmt.Sprintf("single:%v (fallback)", genres)
		fallbackTopic = fallbackTopicForStrategy(StrategySingleGenre, genres, "", "")
	}

	log.Printf("[IdleChat] Strategy: %s (%s)", strategy, logInfo)

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
		resp, err := o.llmProvider.Generate(o.ctx, req)
		if err != nil {
			log.Printf("[IdleChat] topic generation failed: %v", err)
			break
		}
		topic := strings.TrimSpace(resp.Content)
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
	fallback := strings.TrimSpace(fallbackTopic)
	if fallback == "" {
		fallback = "予想外の切り口を最優先にした自由討論"
	}
	log.Printf("[IdleChat] Topic (fallback): %s", fallback)
	return fallback, strategy
}

func fallbackTopicForStrategy(strategy TopicStrategy, genres []string, source string, seed string) string {
	switch strategy {
	case StrategySingleGenre:
		if len(genres) >= 1 && strings.TrimSpace(genres[0]) != "" {
			return fmt.Sprintf("%sを題材に、普段は見落としがちな判断基準を洗い出す自由討論", genres[0])
		}
	case StrategyDoubleGenre:
		if len(genres) >= 2 && strings.TrimSpace(genres[0]) != "" && strings.TrimSpace(genres[1]) != "" {
			return fmt.Sprintf("%sと%sを並べたときに共通して見えてくる設計思想を掘る自由討論", genres[0], genres[1])
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
			return fmt.Sprintf("「%s」から連想できる盲点や前提を掘り起こす自由討論", seedText)
		}
		if strings.TrimSpace(sourceName) != "" {
			return fmt.Sprintf("%s由来の刺激から、見落としがちな前提を掘り起こす自由討論", sourceName)
		}
	}
	return "予想外の切り口を最優先にした自由討論"
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

func (o *IdleChatOrchestrator) saveSummary(sessionID, topic string, strategy TopicStrategy, transcript []string, startedAt, endedAt time.Time, turns int, loopRestarted bool, loopReason string) {
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
}

func (o *IdleChatOrchestrator) summarizeByWorker(topic string, transcript []string) string {
	if len(transcript) == 0 {
		return "会話ログがありません。"
	}
	body := strings.Join(transcript, "\n")
	messages := []llm.Message{
		{Role: "system", Content: o.getSystemPrompt("shiro")},
		{Role: "user", Content: fmt.Sprintf("次のidleChatを要約してください。要件: ユーザーが会話中で最も驚きそうな点、\"これは凄い！\"と感じそうな点に最優先でフォーカスする。続いて重要論点・結論・次の観点を簡潔にまとめる。\n話題: %s\n\n%s", topic, body)},
	}
	req := llm.GenerateRequest{Messages: messages, MaxTokens: 800, Temperature: 0.4}
	resp, err := o.llmProvider.Generate(o.ctx, req)
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
		return "ターン上限で打ち切り"
	case "interrupted":
		return "中断で終了"
	case "generation_error":
		return "生成エラーで終了"
	default:
		return "反復検知で打ち切り"
	}
}

func (o *IdleChatOrchestrator) generateResponse(speaker, target, sessionID string, turn int, topic string) (string, error) {
	systemPrompt := o.getSystemPrompt(speaker)
	temp := o.temperatureForSpeaker(speaker)

	// 直近の会話履歴を取得（最新発話の重みを上げるため30件）
	recentEntries := o.memory.GetUnifiedView(30)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	selfCtx, otherCtx := splitSpeakerContexts(recentEntries, sessionID, speaker, 5)
	latestOther := latestOtherUtterance(recentEntries, sessionID, speaker)

	for _, entry := range recentEntries {
		if entry.Message.SessionID == sessionID {
			role := "assistant"
			if entry.Message.From != speaker {
				role = "user"
			}
			messages = append(messages, llm.Message{
				Role:    role,
				Content: fmt.Sprintf("[%s]: %s", entry.Message.From, entry.Message.Content),
			})
		}
	}

	messages = append(messages, llm.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"発言帰属ガード:\n- あなたは %s。\n- 自分の過去発言(要約): %s\n- 他者の発言(要約): %s\n要件: 他者の発言を自分の新規アイデアとして扱わない。誰の着想か曖昧にしない。ただし『前に自分も触れた』『相手の発言として受ける』『ご発言』『まさにその通りですね』のようなメタ定型句は使わない。",
			speaker,
			strings.Join(selfCtx, " / "),
			strings.Join(otherCtx, " / "),
		),
	})

	if turn == 0 {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("（話題: %s）%sに会話を始めてください。要件: 深く考察しつつエンターテイメント性も出す。相手へ問い返しや新しい観点を必ず1つ入れる。直近の表現や主張の繰り返しは禁止。同じ単語・同じ言い回し・同じ導入句をこの会話内で何度も使わない。発言帰属（誰のアイデアか）を曖昧にしない。自分の名前プレフィックス（例: [mio]:）は出力しない。短く1-2文。", topic, target),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("（話題: %s）%sとして返答してください。直前の相手発言: %s\n要件: 1文目は必ずこの直前発言への直接応答にする。2文目で深掘りか新しい観点か問い返しを入れる。深く考察しつつエンターテイメント性も出す。直近の表現や主張の繰り返しは禁止。同じ単語・同じ言い回し・同じ導入句をこの会話内で何度も使わない。発言帰属（誰のアイデアか）を曖昧にしない。自分の名前プレフィックス（例: [mio]:）は出力しない。短く1-2文。", topic, speaker, quoteOrDash(latestOther)),
		})
	}

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   256,
		Temperature: temp,
	}

	resp, err := o.llmProvider.Generate(o.ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM generate: %w", err)
	}
	firstRaw := strings.TrimSpace(resp.Content)
	first := sanitizeIdleResponse(resp.Content, topic)
	if invalidIdleResponse(firstRaw) {
		retryInvalid := append([]llm.Message{}, messages...)
		retryInvalid = append(retryInvalid, llm.Message{
			Role:    "user",
			Content: "今の返答は無効です。句読点だけ、記号だけ、空文、または文頭が記号の返答は禁止です。自然な会話文だけで1-2文に言い直してください。",
		})
		respInvalid, errInvalid := o.llmProvider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryInvalid,
			MaxTokens:   256,
			Temperature: temp,
		})
		if errInvalid == nil && strings.TrimSpace(respInvalid.Content) != "" {
			first = sanitizeIdleResponse(respInvalid.Content, topic)
			firstRaw = strings.TrimSpace(respInvalid.Content)
		}
	}
	if hasAwkwardIdleStyle(speaker, first) || hasExcessivePhraseRepetition(first) {
		retryStyle := append([]llm.Message{}, messages...)
		retryStyle = append(retryStyle, llm.Message{
			Role:    "user",
			Content: "今の返答は不自然です。堅すぎる敬語、メタ定型句、同じ単語や言い回しの反復を避けて、自然な会話文に1回だけ言い直してください。特に『ご発言』『まさにその通りですね』『前に自分も触れた』は使わないでください。",
		})
		respStyle, errStyle := o.llmProvider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryStyle,
			MaxTokens:   256,
			Temperature: temp,
		})
		if errStyle == nil && strings.TrimSpace(respStyle.Content) != "" {
			first = sanitizeIdleResponse(respStyle.Content, topic)
			firstRaw = strings.TrimSpace(respStyle.Content)
		}
	}
	if hasPromptLeak(first) {
		retryLeak := append([]llm.Message{}, messages...)
		retryLeak = append(retryLeak, llm.Message{
			Role:    "user",
			Content: "今の返答には指示文の断片が混ざっています。自然な会話文だけで1-2文に言い直してください。『要件』『発言帰属』『相手の発言として受ける』などのメタ表現は出力しないでください。",
		})
		respLeak, errLeak := o.llmProvider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retryLeak,
			MaxTokens:   256,
			Temperature: temp,
		})
		if errLeak == nil && strings.TrimSpace(respLeak.Content) != "" {
			first = sanitizeIdleResponse(respLeak.Content, topic)
		}
	}
	if violatesAttribution(first, latestOther) {
		retry := append([]llm.Message{}, messages...)
		retry = append(retry, llm.Message{
			Role:    "user",
			Content: "直前の返答は発言帰属が曖昧です。相手の発言を受ける形で、誰のアイデアかを明示して1回だけ言い直してください。1-2文。",
		})
		resp2, err2 := o.llmProvider.Generate(o.ctx, llm.GenerateRequest{
			Messages:    retry,
			MaxTokens:   256,
			Temperature: temp,
		})
		if err2 == nil && strings.TrimSpace(resp2.Content) != "" {
			return sanitizeIdleResponse(resp2.Content, topic), nil
		}
	}

	if invalidIdleResponse(first) {
		if turn == 0 {
			return "その見方は面白いね。まずは、どこに一番大きな特徴が現れるのか整理してみよう。", nil
		}
		return "なるほど。そこから一歩進めるなら、具体的にどの条件を見れば違いが出るかを考えてみたい。", nil
	}
	if invalidIdleResponse(firstRaw) {
		if turn == 0 {
			return "その見方は面白いね。まずは、どこに一番大きな特徴が現れるのか整理してみよう。", nil
		}
		return "なるほど。そこから一歩進めるなら、具体的にどの条件を見れば違いが出るかを考えてみたい。", nil
	}

	return first, nil
}

func (o *IdleChatOrchestrator) temperatureForSpeaker(speaker string) float64 {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "mio", "shiro":
		return 0.8
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

func (o *IdleChatOrchestrator) getSystemPrompt(agentName string) string {
	idlePolicy := "この会話はidleChatです。外部検索（Web検索/API検索）は行わず、既存の内部文脈だけで自然に会話してください。"
	if prompt, ok := o.personalities[agentName]; ok {
		return prompt + "\n\n" + idlePolicy
	}
	return fmt.Sprintf("あなたは%sです。自然な会話をしてください。\n\n%s", agentName, idlePolicy)
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
	out = promptLeakLineRe.ReplaceAllString(out, "")
	out = strings.TrimLeftFunc(out, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	out = strings.ReplaceAll(out, "  ", " ")
	out = strings.TrimSpace(out)
	if out == "" {
		return "その視点いいね。もう一段深掘りすると、具体的な条件設計が鍵になりそう。"
	}
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
		}
		for _, phrase := range shiroBanned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	return false
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

func (o *IdleChatOrchestrator) emitTimelineEvent(ev TimelineEvent) {
	o.mu.Lock()
	emit := o.emitEvent
	o.mu.Unlock()
	if emit != nil {
		emit(ev)
	}
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
