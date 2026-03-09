package idlechat

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
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

func TestIdleChatOrchestrator_SetChatBusy_UpdatesLastActivityOnRelease(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	old := time.Now().Add(-2 * time.Hour)
	o.mu.Lock()
	o.lastActivity = old
	o.mu.Unlock()

	o.SetChatBusy(false)

	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.lastActivity.After(old) {
		t.Fatal("lastActivity should be updated when chat becomes idle")
	}
}

func TestIdleChatOrchestrator_SetWorkerBusy_UpdatesLastActivityOnRelease(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	old := time.Now().Add(-2 * time.Hour)
	o.mu.Lock()
	o.lastActivity = old
	o.mu.Unlock()

	o.SetWorkerBusy(false)

	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.lastActivity.After(old) {
		t.Fatal("lastActivity should be updated when worker becomes idle")
	}
}

func TestIdleChatOrchestrator_ManualMode_StartStop(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	if err := o.StartManualMode(); err != nil {
		t.Fatalf("StartManualMode failed: %v", err)
	}
	if !o.IsManualMode() {
		t.Fatal("manual mode should be enabled")
	}

	o.StopManualMode()
	if o.IsManualMode() {
		t.Fatal("manual mode should be disabled")
	}
}

func TestIdleChatOrchestrator_ManualMode_StopsOnActivity(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	if err := o.StartManualMode(); err != nil {
		t.Fatalf("StartManualMode failed: %v", err)
	}
	o.NotifyActivity()
	if o.IsManualMode() {
		t.Fatal("manual mode should stop after activity")
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

	// 話題生成1回 + 会話maxTurns回 + 要約1回
	minExpectedCalls := maxTurns + 2
	if provider.callCount < minExpectedCalls {
		t.Errorf("Expected at least %d LLM calls, got %d", minExpectedCalls, provider.callCount)
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
	if provider.callCount < 4 {
		t.Errorf("Expected at least 4 LLM calls (topic + maxTurns + summary), got %d", provider.callCount)
	}

	// セッション終了後はchatActive=false
	if o.IsChatActive() {
		t.Error("chatActive should be false after session completes")
	}
}

func TestCheckAndStartChat_RequiresMinimumTenMinuteCooldown(t *testing.T) {
	provider := &mockLLMProvider{response: "hello", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 1, 2, 0.8, nil)

	o.mu.Lock()
	o.lastActivity = time.Now().Add(-9 * time.Minute)
	o.mu.Unlock()

	o.checkAndStartChat()
	if provider.callCount != 0 {
		t.Fatalf("Expected 0 calls before 10-minute cooldown, got %d", provider.callCount)
	}

	o.mu.Lock()
	o.lastActivity = time.Now().Add(-11 * time.Minute)
	o.mu.Unlock()

	o.checkAndStartChat()
	if provider.callCount < 4 {
		t.Fatalf("Expected session to start after cooldown, got %d calls", provider.callCount)
	}
}

func TestRunChatSession_SetsTenMinuteTopicCooldown(t *testing.T) {
	provider := &mockLLMProvider{response: "hello", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 1, 2, 0.8, nil)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.runChatSession()

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.nextTopicAt.Sub(o.lastActivity) < 10*time.Minute {
		t.Fatalf("expected nextTopicAt to be at least 10 minutes after lastActivity, got %v", o.nextTopicAt.Sub(o.lastActivity))
	}
}

func TestCheckAndStartChat_ManualMode_StartsWithoutIdleThreshold(t *testing.T) {
	provider := &mockLLMProvider{response: "hello", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 60, 2, 0.8, nil)

	if err := o.StartManualMode(); err != nil {
		t.Fatalf("StartManualMode failed: %v", err)
	}

	o.checkAndStartChat()
	if provider.callCount < 4 {
		t.Fatalf("Expected at least 4 LLM calls in manual mode (topic + maxTurns + summary), got %d", provider.callCount)
	}
}

func TestCheckAndStartChat_RespectsMinTopicInterval(t *testing.T) {
	provider := &mockLLMProvider{response: "hello", delay: 1 * time.Millisecond}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 0, 2, 0.8, nil)

	o.mu.Lock()
	o.lastActivity = time.Now().Add(-1 * time.Hour)
	o.nextTopicAt = time.Now().Add(5 * time.Minute)
	o.mu.Unlock()

	o.checkAndStartChat()
	if provider.callCount != 0 {
		t.Fatalf("Expected 0 calls while within topic interval, got %d", provider.callCount)
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

func TestIsLooping_DetectsAlternatingSimilarity(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	transcript := []string{
		"mio: 世界の法則が変わるRPGって面白いよね",
		"shiro: その変化を倫理と戦略の両方で扱うのが核心ですね",
		"mio: 世界の法則が変わるRPGって面白いよね！",
		"shiro: その変化を倫理と戦略の両方で扱うのが核心です",
		"mio: 世界の法則が変わるRPGって面白いよね",
		"shiro: その変化を倫理と戦略の両方で扱うのが核心ですね",
		"mio: 世界の法則が変わるRPGって面白いよね！",
		"shiro: その変化を倫理と戦略の両方で扱うのが核心です",
	}
	if !o.isLooping(transcript) {
		t.Fatal("expected alternating repetitive transcript to be detected as loop")
	}
}

func TestTopicTooSimilar(t *testing.T) {
	recent := []string{
		"人生をRPG化するならどんな世界観がいいか",
		"月面都市の建設競争とAI設計の未来",
	}
	if !topicTooSimilar("人生をRPG化するならどんな世界観が良いか？", recent) {
		t.Fatal("expected near-duplicate topic to be considered similar")
	}
	if topicTooSimilar("量子通信が一般家庭に来たときの意外な副作用", recent) {
		t.Fatal("expected clearly different topic to be accepted")
	}
}

func TestIsResponseTooSimilar(t *testing.T) {
	transcript := []string{
		"mio: 世界の調律師という設定が面白い",
		"shiro: 調和と混沌の選択が主題になります",
		"mio: 運命のカードで分岐を増やしたい",
		"shiro: カードと行動の連動が鍵ですね",
		"mio: 世界の調律師という設定が面白い",
		"shiro: 調和と混沌の選択が主題になります",
	}
	if !isResponseTooSimilar("世界の調律師という設定が面白い！", transcript) {
		t.Fatal("expected repetitive response to be detected")
	}
	if isResponseTooSimilar("都市インフラを音楽理論で最適化する話に広げよう", transcript) {
		t.Fatal("expected fresh response not to be detected as repetitive")
	}
}

func TestSplitSpeakerContexts(t *testing.T) {
	mem := session.NewCentralMemory()
	sid := "idle-ctx"
	mem.RecordMessage(domaintransport.Message{From: "mio", To: "shiro", SessionID: sid, Content: "最初の提案"})
	mem.RecordMessage(domaintransport.Message{From: "shiro", To: "mio", SessionID: sid, Content: "その提案の補足"})
	mem.RecordMessage(domaintransport.Message{From: "mio", To: "shiro", SessionID: sid, Content: "別観点の追加"})

	entries := mem.GetUnifiedView(20)
	self, other := splitSpeakerContexts(entries, sid, "mio", 5)
	if len(self) == 0 || len(other) == 0 {
		t.Fatal("expected both self/other contexts")
	}
	if self[0] != "別観点の追加" {
		t.Fatalf("expected latest self context first, got %q", self[0])
	}
	if other[0] == "なし" {
		t.Fatal("expected other context to include shiro utterance")
	}
}

func TestViolatesAttribution(t *testing.T) {
	other := "世界の調律師という設定はどう？"
	if !violatesAttribution("世界の調律師という設定はどう？", other) {
		t.Fatal("expected direct reuse without attribution to be flagged")
	}
	if violatesAttribution("あなたの『世界の調律師』案を受けると、次は倫理分岐を入れたい", other) {
		t.Fatal("expected explicit attribution to pass")
	}
}
