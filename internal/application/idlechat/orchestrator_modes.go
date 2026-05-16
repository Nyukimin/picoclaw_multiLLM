package idlechat

import (
	"fmt"
	"log"
	"strings"
	"time"
)

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
