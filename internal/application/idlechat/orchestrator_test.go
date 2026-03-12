package idlechat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

// mockLLMProvider はテスト用のモックLLMプロバイダー
type mockLLMProvider struct {
	response  string
	responses []string
	err       error
	callCount int
	delay     time.Duration // Generate呼び出し時の遅延
	lastReq   llm.GenerateRequest
}

func (m *mockLLMProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	m.callCount++
	m.lastReq = req
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.err != nil {
		return llm.GenerateResponse{}, m.err
	}
	if len(m.responses) > 0 {
		idx := m.callCount - 1
		if idx >= len(m.responses) {
			idx = len(m.responses) - 1
		}
		return llm.GenerateResponse{
			Content:      m.responses[idx],
			TokensUsed:   10,
			FinishReason: "stop",
		}, nil
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

func TestIdleChatOrchestrator_TemperatureForSpeaker_MioAndShiroFixed(t *testing.T) {
	provider := &mockLLMProvider{response: "新しい観点を出してみよう。"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.2, nil)

	if _, err := o.generateResponse("mio", "shiro", "idle-temp", 0, "話題"); err != nil {
		t.Fatalf("generateResponse(mio) failed: %v", err)
	}
	if provider.lastReq.Temperature != 0.8 {
		t.Fatalf("expected mio idlechat temperature 0.8, got %v", provider.lastReq.Temperature)
	}

	if _, err := o.generateResponse("shiro", "mio", "idle-temp", 1, "話題"); err != nil {
		t.Fatalf("generateResponse(shiro) failed: %v", err)
	}
	if provider.lastReq.Temperature != 0.8 {
		t.Fatalf("expected shiro idlechat temperature 0.8, got %v", provider.lastReq.Temperature)
	}
}

func TestIdleChatOrchestrator_TemperatureForSpeaker_OthersUseConfiguredValue(t *testing.T) {
	provider := &mockLLMProvider{response: "別の案として考えると面白い。"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"gin", "mio"}, 5, 10, 0.35, nil)

	if _, err := o.generateResponse("gin", "mio", "idle-temp", 0, "話題"); err != nil {
		t.Fatalf("generateResponse(gin) failed: %v", err)
	}
	if provider.lastReq.Temperature != 0.35 {
		t.Fatalf("expected non-mio/shiro idlechat temperature 0.35, got %v", provider.lastReq.Temperature)
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

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 100, 0.8, nil)

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
	if until := time.Until(o.nextTopicAt); until < 4*time.Minute {
		t.Fatalf("expected interruption to apply idle cooldown, got nextTopicAt in %v", until)
	}
}

func TestIdleChatOrchestrator_GenerationErrorAppliesCooldown(t *testing.T) {
	provider := &mockLLMProvider{err: context.DeadlineExceeded}
	memory := session.NewCentralMemory()

	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	o.mu.Lock()
	o.chatActive = true
	o.mu.Unlock()

	o.runChatSession()

	if until := time.Until(o.nextTopicAt); until < 4*time.Minute {
		t.Fatalf("expected generation error to apply idle cooldown, got nextTopicAt in %v", until)
	}
	if len(o.GetHistory(10)) != 0 {
		t.Fatalf("expected no summary history for zero-turn failed session, got %d", len(o.GetHistory(10)))
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

func TestFallbackTopicForStrategy_SingleUsesGenre(t *testing.T) {
	got := fallbackTopicForStrategy(StrategySingleGenre, []string{"昆虫学"}, "", "")
	if !strings.Contains(got, "昆虫学") {
		t.Fatalf("expected single fallback to include genre, got %q", got)
	}
}

func TestFallbackTopicForStrategy_DoubleUsesBothGenres(t *testing.T) {
	got := fallbackTopicForStrategy(StrategyDoubleGenre, []string{"茶道", "歯車"}, "", "")
	if !strings.Contains(got, "茶道") || !strings.Contains(got, "歯車") {
		t.Fatalf("expected double fallback to include both genres, got %q", got)
	}
}

func TestFallbackTopicForStrategy_ExternalUsesSeed(t *testing.T) {
	got := fallbackTopicForStrategy(StrategyExternalStimulus, nil, "Wikipedia:アレクサンドリア", "")
	if !strings.Contains(got, "アレクサンドリア") {
		t.Fatalf("expected external fallback to include seed, got %q", got)
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

func TestIsLooping_DetectsRepeatedSpeakerTemplates(t *testing.T) {
	provider := &mockLLMProvider{response: "hello"}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	transcript := []string{
		"mio: まさに！音色を形にするって、まるで自分の心の風景を立体的に表現していくみたいじゃない？",
		"shiro: [mio]の表現は、非常に的確で、具体的なイメージを喚起するものです。しかし、音の質をどう扱うべきでしょうか。",
		"mio: まさに！感情そのものを具現化するって、まるで音色で自分の心模様を鮮やかに描き出すようなものじゃない？",
		"shiro: [mio]の表現は、非常に興味深いですね。しかし、その表現を成し遂げるには、どのような工夫が必要でしょうか。",
		"mio: まさに！物語を紡ぎ出すって、すごくロマンチックじゃない？",
		"shiro: [mio]の表現は、非常に興味深いですね。しかし、物語のテーマを明確にする必要があるのではないでしょうか。",
	}
	if !o.isLooping(transcript) {
		t.Fatal("expected repeated speaker templates to be detected as loop")
	}
	if reason := detectLoopReason(transcript); reason != "template_repeat" {
		t.Fatalf("expected template_repeat, got %q", reason)
	}
}

func TestAnnotateLoopSummary_AddsReasonNote(t *testing.T) {
	got := annotateLoopSummary("本文", true, "template_repeat")
	want := "注記: テンプレ反復で打ち切り\n\n本文"
	if got != want {
		t.Fatalf("unexpected annotated summary: got %q want %q", got, want)
	}
}

func TestSanitizeIdleResponse_StripsLeadingPunctuation(t *testing.T) {
	got := sanitizeIdleResponse("。「。」なるほど！じゃあ、観察対象を絞ろう。", "話題")
	want := "なるほど！じゃあ、観察対象を絞ろう。"
	if got != want {
		t.Fatalf("sanitizeIdleResponse() = %q, want %q", got, want)
	}
}

func TestInvalidIdleResponse_RejectsLeadingPunctuation(t *testing.T) {
	tests := []string{
		"。",
		"、まるですごろくが戦略を読み解こうとするなんて、めっちゃ面白い！",
		"。「。」なるほど！じゃあ、足切れる場所を特定するために考えよう。",
	}
	for _, input := range tests {
		if !invalidIdleResponse(input) {
			t.Fatalf("expected invalidIdleResponse(%q) to be true", input)
		}
	}
}

func TestGenerateResponse_RetriesInvalidLeadingPunctuation(t *testing.T) {
	provider := &mockLLMProvider{
		responses: []string{
			"。「。」なるほど！じゃあ、足切れる場所を特定するために考えよう。",
			"なるほど！じゃあ、足切れる場所を特定するために、どのマスで失速するか集計してみよう。",
		},
	}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	got, err := o.generateResponse("mio", "shiro", "idle-invalid", 1, "すごろく")
	if err != nil {
		t.Fatalf("generateResponse() failed: %v", err)
	}
	if provider.callCount < 2 {
		t.Fatalf("expected retry on invalid response, got %d calls", provider.callCount)
	}
	if strings.HasPrefix(got, "。") || strings.HasPrefix(got, "、") {
		t.Fatalf("expected sanitized retry result without leading punctuation, got %q", got)
	}
}

func TestHasAwkwardIdleStyle_DetectsShiroCliches(t *testing.T) {
	if !hasAwkwardIdleStyle("shiro", "mioさんのご発言、まさにその通りですね。非常に興味深いですね。") {
		t.Fatal("expected awkward shiro cliche to be detected")
	}
	if hasAwkwardIdleStyle("shiro", "その視点は面白いです。ここで条件を一つ足すと見え方が変わりそうです。") {
		t.Fatal("expected natural shiro response to pass")
	}
}

func TestHasExcessivePhraseRepetition_DetectsRepeatedPhrases(t *testing.T) {
	if !hasExcessivePhraseRepetition("まさに まさに まさに 面白いですね。") {
		t.Fatal("expected repeated token to be detected")
	}
	if !hasExcessivePhraseRepetition("同じ こと を 考える。同じ こと を 考える。") {
		t.Fatal("expected repeated phrase to be detected")
	}
	if hasExcessivePhraseRepetition("その視点は面白いです。条件を変えると結果も動きそうです。") {
		t.Fatal("expected non-repetitive response to pass")
	}
}

func TestGenerateResponse_RetriesAwkwardShiroStyle(t *testing.T) {
	provider := &mockLLMProvider{
		responses: []string{
			"mioさんのご発言、まさにその通りですね。前に自分も触れたように、非常に興味深いですね。",
			"その見方は面白いです。どの条件で差が出るのかを先に切り分けたいですね。",
		},
	}
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.8, nil)

	got, err := o.generateResponse("shiro", "mio", "idle-style", 1, "すごろく")
	if err != nil {
		t.Fatalf("generateResponse() failed: %v", err)
	}
	if provider.callCount < 2 {
		t.Fatalf("expected retry on awkward style, got %d calls", provider.callCount)
	}
	if hasAwkwardIdleStyle("shiro", got) {
		t.Fatalf("expected retried shiro response to avoid awkward style, got %q", got)
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
