package idlechat

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const (
	idleCheckInterval = 30 * time.Second
)

// IdleChatOrchestrator はアイドル時のAgent間雑談を管理
type IdleChatOrchestrator struct {
	llmProvider   llm.LLMProvider
	memory        *session.CentralMemory
	participants  []string
	intervalMin   int
	maxTurns      int
	temperature   float64
	personalities map[string]string

	lastActivity time.Time
	chatActive   bool
	chatBusy     bool
	workerBusy   bool
	manualMode   bool

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
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
	ctx, cancel := context.WithCancel(context.Background())
	return &IdleChatOrchestrator{
		llmProvider:   llmProvider,
		memory:        memory,
		participants:  participants,
		intervalMin:   intervalMin,
		maxTurns:      maxTurns,
		temperature:   temperature,
		personalities: personalities,
		lastActivity:  time.Now(),
		ctx:           ctx,
		cancel:        cancel,
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
	o.mu.Unlock()
}

func (o *IdleChatOrchestrator) runChatSession() {
	sessionID := fmt.Sprintf("idle-%d", time.Now().Unix())

	// 最初の発話者をランダムに選択（簡易: participants[0]から）
	currentSpeaker := 0

	for turn := 0; turn < o.maxTurns; turn++ {
		select {
		case <-o.ctx.Done():
			return
		default:
		}

		// タスク到着で中断チェック
		o.mu.Lock()
		if !o.chatActive {
			o.mu.Unlock()
			log.Printf("[IdleChat] Session interrupted at turn %d", turn)
			return
		}
		o.mu.Unlock()

		speaker := o.participants[currentSpeaker]
		nextSpeaker := o.participants[(currentSpeaker+1)%len(o.participants)]

		response, err := o.generateResponse(speaker, nextSpeaker, sessionID, turn)
		if err != nil {
			log.Printf("[IdleChat] Generation error: %v", err)
			return
		}

		// メモリに記録
		msg := domaintransport.NewMessage(speaker, nextSpeaker, sessionID, "", response)
		msg.Type = domaintransport.MessageTypeIdleChat
		o.memory.RecordMessage(msg)

		log.Printf("[IdleChat] [Turn %d] %s→%s: %s", turn, speaker, nextSpeaker,
			truncate(response, 80))

		currentSpeaker = (currentSpeaker + 1) % len(o.participants)
	}

	log.Printf("[IdleChat] Session %s completed (%d turns)", sessionID, o.maxTurns)
}

func (o *IdleChatOrchestrator) generateResponse(speaker, target, sessionID string, turn int) (string, error) {
	systemPrompt := o.getSystemPrompt(speaker)

	// 直近の会話履歴を取得
	recentEntries := o.memory.GetUnifiedView(10)
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
			Content: fmt.Sprintf("（暇な時間です。%sに話しかけてみてください。自由な話題で短く1-2文で。）", target),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("（%sとして返答してください。短く1-2文で。）", speaker),
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
