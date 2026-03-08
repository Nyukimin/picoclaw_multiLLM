package idlechat

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	idleCheckInterval = 30 * time.Second
	ttsCharsPerSecond = 8.0
	ttsMinWait        = 2 * time.Second
	ttsMaxWait        = 20 * time.Second
)

var jst = time.FixedZone("JST", 9*60*60)
var randSeedOnce sync.Once

type TopicCategory string

const (
	TopicUserRelated TopicCategory = "user_related"
	TopicCurrent     TopicCategory = "current_events"
	TopicTech        TopicCategory = "tech"
	TopicWorkerDB    TopicCategory = "worker_db"
	TopicRandom      TopicCategory = "random"
)

type SessionSummary struct {
	SessionID       string        `json:"session_id"`
	Title           string        `json:"title"`
	Topic           string        `json:"topic"`
	Category        TopicCategory `json:"category"`
	Summary         string        `json:"summary"`
	StartedAt       string        `json:"started_at"`
	EndedAt         string        `json:"ended_at"`
	Turns           int           `json:"turns"`
	LoopRestarted   bool          `json:"loop_restarted"`
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
	history      []SessionSummary
	categoryBag  []TopicCategory
	emitEvent    func(TimelineEvent)

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
		topic, category := o.generateTopicFromChat(sessionID)
		o.mu.Lock()
		o.currentTopic = topic
		o.mu.Unlock()
		log.Printf("[IdleChat] Topic: %s (%s)", topic, category)

		segmentTurns := 0
		loopDetected := false
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
				return
			}
			o.mu.Unlock()

			speaker := o.participants[currentSpeaker]
			nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

			response, err := o.generateResponse(speaker, nextSpeaker, sessionID, turn, topic)
			if err != nil {
				log.Printf("[IdleChat] Generation error: %v", err)
				return
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

			if o.isLooping(transcript) {
				loopDetected = true
				log.Printf("[IdleChat] Loop detected, summarize and restart with new topic")
				break
			}
			currentSpeaker = (currentSpeaker + 1) % len(o.participants)
		}

		remainingTurns -= segmentTurns
		o.saveSummary(sessionID, topic, category, transcript, startedAt, time.Now().In(jst), segmentTurns, loopDetected)

		if !loopDetected || remainingTurns <= 0 {
			break
		}
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

func (o *IdleChatOrchestrator) generateTopicFromChat(sessionID string) (string, TopicCategory) {
	category := o.chooseTopicCategory()
	contextHints := o.buildTopicHints(category)
	messages := []llm.Message{
		{Role: "system", Content: o.getSystemPrompt("mio")},
		{Role: "user", Content: fmt.Sprintf("idleChatの話題を1つだけ提案してください。カテゴリ=%s。要件: 深く考察でき、かつエンターテイメント性がある具体的な話題。回答は話題1文のみ。参考情報: %s", category, contextHints)},
	}
	req := llm.GenerateRequest{Messages: messages, MaxTokens: 120, Temperature: 0.8}
	resp, err := o.llmProvider.Generate(o.ctx, req)
	if err != nil {
		log.Printf("[IdleChat] topic generation failed: %v", err)
		return o.fallbackTopic(category), category
	}
	topic := strings.TrimSpace(resp.Content)
	if topic == "" {
		topic = o.fallbackTopic(category)
	}
	return topic, category
}

func (o *IdleChatOrchestrator) chooseTopicCategory() TopicCategory {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.categoryBag) == 0 {
		o.categoryBag = []TopicCategory{
			TopicUserRelated, TopicUserRelated, TopicUserRelated,
			TopicCurrent, TopicCurrent,
			TopicTech, TopicTech,
			TopicWorkerDB, TopicWorkerDB,
			TopicRandom,
		}
		rand.Shuffle(len(o.categoryBag), func(i, j int) {
			o.categoryBag[i], o.categoryBag[j] = o.categoryBag[j], o.categoryBag[i]
		})
	}
	next := o.categoryBag[0]
	o.categoryBag = o.categoryBag[1:]
	return next
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

func (o *IdleChatOrchestrator) buildTopicHints(category TopicCategory) string {
	entries := o.memory.GetUnifiedView(120)
	switch category {
	case TopicUserRelated:
		return o.formatHintsFromLatestSession(entries, func(m domaintransport.Message) bool {
			return isUserMessage(m) && !isIdleMessage(m)
		}, "ユーザー発言履歴なし")
	case TopicWorkerDB:
		return o.formatHintsFromLatestSession(entries, func(m domaintransport.Message) bool {
			return isWorkerMessage(m) && !isIdleMessage(m)
		}, "worker関連履歴なし")
	default:
		return "内部履歴を踏まえて選定"
	}
}

func (o *IdleChatOrchestrator) fallbackTopic(category TopicCategory) string {
	switch category {
	case TopicUserRelated:
		return "最近のユーザー要望から見える優先課題"
	case TopicCurrent:
		return "最近の社会・技術トレンドが開発運用に与える影響"
	case TopicTech:
		return "今のアーキテクチャで改善余地が大きい技術ポイント"
	case TopicWorkerDB:
		return "workerの実行履歴データから見える改善機会"
	default:
		return "最近気になったことを起点にした自由討論"
	}
}

func (o *IdleChatOrchestrator) isLooping(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	norm := func(s string) string {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "　", "")
		s = strings.ReplaceAll(s, "。", "")
		s = strings.ReplaceAll(s, "、", "")
		return s
	}
	last := norm(transcript[len(transcript)-1])
	if last == "" {
		return false
	}
	count := 0
	for i := len(transcript) - 4; i < len(transcript)-1; i++ {
		if i >= 0 && norm(transcript[i]) == last {
			count++
		}
	}
	return count >= 1
}

func (o *IdleChatOrchestrator) saveSummary(sessionID, topic string, category TopicCategory, transcript []string, startedAt, endedAt time.Time, turns int, loopRestarted bool) {
	summary := o.summarizeByWorker(topic, transcript)
	title := fmt.Sprintf("%d月%d日の%sの話題まとめ", endedAt.Month(), endedAt.Day(), truncate(topic, 24))
	record := SessionSummary{
		SessionID:       sessionID,
		Title:           title,
		Topic:           topic,
		Category:        category,
		Summary:         summary,
		StartedAt:       startedAt.Format(time.RFC3339),
		EndedAt:         endedAt.Format(time.RFC3339),
		Turns:           turns,
		LoopRestarted:   loopRestarted,
		TopicProvider:   "mio",
		SummaryProvider: "shiro",
		Transcript:      append([]string(nil), transcript...),
	}
	o.mu.Lock()
	o.history = append(o.history, record)
	if len(o.history) > 100 {
		o.history = o.history[len(o.history)-100:]
	}
	o.mu.Unlock()

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
	req := llm.GenerateRequest{Messages: messages, MaxTokens: 240, Temperature: 0.4}
	resp, err := o.llmProvider.Generate(o.ctx, req)
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		return truncate(body, 200)
	}
	return strings.TrimSpace(resp.Content)
}

func (o *IdleChatOrchestrator) generateResponse(speaker, target, sessionID string, turn int, topic string) (string, error) {
	systemPrompt := o.getSystemPrompt(speaker)

	// 直近の会話履歴を取得
	recentEntries := o.memory.GetUnifiedView(50)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}

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

	if turn == 0 {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("（話題: %s）%sに会話を始めてください。要件: 深く考察しつつエンターテイメント性も出す。相手へ問い返しや新しい観点を必ず1つ入れる。自分の名前プレフィックス（例: [mio]:）は出力しない。短く1-2文。", topic, target),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("（話題: %s）%sとして返答してください。要件: 深く考察しつつエンターテイメント性も出す。相手へ問い返しや新しい観点を必ず1つ入れる。自分の名前プレフィックス（例: [mio]:）は出力しない。短く1-2文。", topic, speaker),
		})
	}

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   256,
		Temperature: o.temperature,
	}

	resp, err := o.llmProvider.Generate(o.ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM generate: %w", err)
	}

	return resp.Content, nil
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

func (o *IdleChatOrchestrator) emitTimelineEvent(ev TimelineEvent) {
	o.mu.Lock()
	emit := o.emitEvent
	o.mu.Unlock()
	if emit != nil {
		emit(ev)
	}
}
