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
	participants := []string{"Mio", "Shiro"}

	o := NewIdleChatOrchestrator(provider, memory, participants, 5, 10, 0.8)

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

	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio", "Shiro"}, 60, 3, 0.8)

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

	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio", "Shiro"}, 5, 10, 0.8)

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

	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio", "Shiro"}, 5, 10, 0.8)

	if o.IsChatActive() {
		t.Error("Should not be active initially")
	}
}

func TestIdleChatOrchestrator_RunChatSession(t *testing.T) {
	provider := &mockLLMProvider{response: "こんにちは！", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	maxTurns := 3

	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio", "Shiro"}, 0, maxTurns, 0.8)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.runChatSession()

	// LLMが maxTurns回 呼ばれているはず
	if provider.callCount != maxTurns {
		t.Errorf("Expected %d LLM calls, got %d", maxTurns, provider.callCount)
	}

	// メモリに記録されているはず（重複排除によりmaxTurns以下の場合もある）
	mioMemory := memory.GetOrCreateAgent("Mio")
	shiroMemory := memory.GetOrCreateAgent("Shiro")
	totalEntries := mioMemory.Count() + shiroMemory.Count()
	if totalEntries < maxTurns {
		t.Errorf("Expected at least %d total entries across agents, got %d", maxTurns, totalEntries)
	}
}

func TestIdleChatOrchestrator_ChatInterrupted(t *testing.T) {
	provider := &mockLLMProvider{response: "response", delay: 5 * time.Millisecond}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio", "Shiro"}, 0, 100, 0.8)

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

func TestGetSystemPrompt(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"Mio"}, 5, 10, 0.8)

	// 既知のAgent
	prompt := o.getSystemPrompt("Mio")
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
