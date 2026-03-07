package idlechat

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

// mockLLMProvider はテスト用のモックLLMプロバイダー
type mockLLMProvider struct {
	response  string
	err       error
	callCount int
	delay     time.Duration // Generate呼び出し時の遅延
}

func (m *mockLLMProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	m.callCount++
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.err != nil {
		return llm.GenerateResponse{}, m.err
	}
	return llm.GenerateResponse{
		Content:      m.response,
		TokensUsed:   10,
		FinishReason: "stop",
	}, nil
}

func (m *mockLLMProvider) Name() string {
	return "mock"
}

func TestNewIdleChatOrchestrator(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	participants := []string{"mio", "shiro"}

	o := NewIdleChatOrchestrator(provider, memory, participants, 5, 10, 0.8, nil)

	if o.intervalMin != 5 {
		t.Errorf("Expected intervalMin=5, got %d", o.intervalMin)
	}
	if o.maxTurns != 10 {
		t.Errorf("Expected maxTurns=10, got %d", o.maxTurns)
	}
	if o.temperature != 0.8 {
		t.Errorf("Expected temperature=0.8, got %f", o.temperature)
	}
	if len(o.participants) != 2 {
		t.Errorf("Expected 2 participants, got %d", len(o.participants))
	}
}

func TestIdleChatOrchestrator_StartStop(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 60, 3, 0.8, nil)

	o.Start()

	// 即座にStop
	done := make(chan struct{})
	go func() {
		o.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Stop timed out")
	}
}

func TestIdleChatOrchestrator_NotifyActivity(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	// chatActiveを手動で設定してNotifyActivityで中断されることを確認
	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.NotifyActivity()

	if o.IsChatActive() {
		t.Error("Chat should be interrupted after NotifyActivity")
	}
}

func TestIdleChatOrchestrator_IsChatActive(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	if o.IsChatActive() {
		t.Error("Should not be active initially")
	}
}

func TestIdleChatOrchestrator_RunChatSession(t *testing.T) {
	provider := &mockLLMProvider{response: "こんにちは！", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	maxTurns := 3

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 0, maxTurns, 0.8, nil)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.runChatSession()

	// LLMが maxTurns回 呼ばれているはず
	if provider.callCount != maxTurns {
		t.Errorf("Expected %d LLM calls, got %d", maxTurns, provider.callCount)
	}

	// メモリに記録されているはず（重複排除によりmaxTurns以下の場合もある）
	mioMemory := memory.GetOrCreateAgent("mio")
	shiroMemory := memory.GetOrCreateAgent("shiro")
	totalEntries := mioMemory.Count() + shiroMemory.Count()
	if totalEntries < maxTurns {
		t.Errorf("Expected at least %d total entries across agents, got %d", maxTurns, totalEntries)
	}
}

func TestIdleChatOrchestrator_ChatInterrupted(t *testing.T) {
	provider := &mockLLMProvider{response: "response", delay: 5 * time.Millisecond}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 0, 100, 0.8, nil)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	// 別goroutineで少し後に中断（delay=5ms * 数ターン後に到達）
	go func() {
		time.Sleep(30 * time.Millisecond)
		o.NotifyActivity()
	}()

	o.runChatSession()

	// 100ターン全部は実行されていないはず
	if provider.callCount >= 100 {
		t.Error("Chat should have been interrupted before 100 turns")
	}
}

func TestCheckAndStartChat_NotIdleLongEnough(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 60, 3, 0.8, nil)
	// lastActivity は now（アイドル時間が短い）

	o.checkAndStartChat()

	// アイドル時間不足なので雑談は開始しない
	if provider.callCount != 0 {
		t.Errorf("Expected 0 LLM calls (not idle enough), got %d", provider.callCount)
	}
}

func TestCheckAndStartChat_AlreadyActive(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 0, 3, 0.8, nil)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.checkAndStartChat()

	// 既にアクティブなので新しいセッションは開始しない
	if provider.callCount != 0 {
		t.Errorf("Expected 0 LLM calls (already active), got %d", provider.callCount)
	}
}

func TestCheckAndStartChat_StartsSession(t *testing.T) {
	provider := &mockLLMProvider{response: "hello", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 0, 2, 0.8, nil)

	// lastActivity を過去に設定
	o.mu.Lock()
	o.lastActivity = time.Now().Add(-1 * time.Hour)
	o.mu.Unlock()

	o.checkAndStartChat()

	// 雑談セッションが実行されたはず
	if provider.callCount != 2 {
		t.Errorf("Expected 2 LLM calls (maxTurns=2), got %d", provider.callCount)
	}

	// セッション終了後はchatActive=false
	if o.IsChatActive() {
		t.Error("chatActive should be false after session completes")
	}
}

func TestGetSystemPrompt(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio"}, 5, 10, 0.8, nil)

	// 既知のAgent
	prompt := o.getSystemPrompt("mio")
	if prompt == "" {
		t.Error("Expected non-empty prompt for Mio")
	}

	// 未知のAgent
	prompt = o.getSystemPrompt("Unknown")
	if prompt == "" {
		t.Error("Expected fallback prompt for unknown agent")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"こんにちは", 3, "こんに..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}
