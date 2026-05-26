package idlechat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func TestRunChatSessionDoesNotSwitchTopicWithinSingleIdleSession(t *testing.T) {
	responses := []string{"郵便と古書店"}
	for i := 0; i < maxTurnsPerTopic*2; i++ {
		responses = append(responses, fmt.Sprintf("古書店の棚に残った手紙を手がかりに、二人が同じ謎を少しずつ見る返答です。番号%dの具体物で話を前に進めます。", i+1))
	}
	responses = append(responses,
		"一番面白かったのは、古書店の棚に残った手紙を同じ謎として追えた点です。二人が手紙の意味を少しずつ具体化したことで話が前に進みました。次は差出人の選択へ広げられます。",
		"QUALITY: pass\nBORING_CAUSE: 大きな損耗は検出されませんでした。\nINTEREST_HOOK: 古書店の棚に残った手紙\nMISSED_TURN: 手紙を誰が置いたかに絞る余地がありました。\nPROMPT_FIX: INTEREST_HOOKを一つ選び、場面・選択・秘密へ変換する。\nLENGTH_CONTROL: 2文以内。",
	)
	provider := &capturingIdleProvider{
		response:  "追加の話題へ切り替えないための既定応答です。",
		responses: responses,
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, maxTurnsPerTopic+1, 0.7, nil, "")
	o.mu.Lock()
	o.chatActive = true
	o.beginIdleRunLocked()
	o.mu.Unlock()
	defer o.cancelIdleRun()

	o.runChatSession(StrategySingleGenre)
	if len(o.history) != 1 {
		t.Fatalf("history summaries = %d, want 1; records=%+v", len(o.history), o.history)
	}
	if strings.Contains(o.history[0].SessionID, "topic-01") {
		t.Fatalf("single idle session switched topic: %s", o.history[0].SessionID)
	}
	if got := countTopicGenerationRequests(provider.requests); got != 1 {
		t.Fatalf("topic generation requests = %d, want 1", got)
	}
}

func TestRunChatSessionContinuesToTurnLimitAfterLoopWarning(t *testing.T) {
	responses := []string{"映画館に残った鍵"}
	for i := 0; i < maxTurnsPerTopic*2; i++ {
		responses = append(responses, fmt.Sprintf("もし鍵が古い映写機を開ける合図だったら、二人は暗い客席で同じ場面をもう一度見ることになります。番号%dの手がかりが次へ進みます。", i+1))
	}
	responses = append(responses,
		"一番面白かったのは、映画館に残った鍵を最後まで同じ話題として追えた点です。二人が客席と映写機の手がかりを順に重ねました。",
		"QUALITY: pass\nBORING_CAUSE: 大きな損耗は検出されませんでした。\nINTEREST_HOOK: 映画館に残った鍵\nMISSED_TURN: なし\nPROMPT_FIX: \nLENGTH_CONTROL: 2文以内。",
	)
	provider := &capturingIdleProvider{
		response:  "もし鍵が古い映写機を開ける合図だったら、二人は暗い客席で同じ場面をもう一度見ることになります。",
		responses: responses,
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, maxTurnsPerTopic, 0.7, nil, "")
	o.mu.Lock()
	o.chatActive = true
	o.beginIdleRunLocked()
	o.mu.Unlock()
	defer o.cancelIdleRun()

	o.runChatSession(StrategySingleGenre)

	if len(o.history) != 1 {
		t.Fatalf("history summaries = %d, want 1; records=%+v", len(o.history), o.history)
	}
	if got := o.history[0].Turns; got != maxTurnsPerTopic {
		t.Fatalf("turns = %d, want %d", got, maxTurnsPerTopic)
	}
	if o.history[0].LoopRestarted {
		t.Fatalf("loop warning should not mark a completed session as restarted: reason=%q", o.history[0].LoopReason)
	}
}

func TestRunChatSessionRecordsGenerationErrorInConversationHistory(t *testing.T) {
	provider := &capturingIdleProvider{
		responses: []string{
			"郵便と古書店",
			"",
			"",
			"",
		},
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 2, 0.7, nil, "")
	o.mu.Lock()
	o.chatActive = true
	o.beginIdleRunLocked()
	o.mu.Unlock()
	defer o.cancelIdleRun()

	o.runChatSession(StrategySingleGenre)

	entries := o.memory.GetUnifiedView(20)
	found := false
	for _, entry := range entries {
		if strings.Contains(entry.Message.Content, "生成エラー") {
			found = true
			if !strings.Contains(entry.Message.Content, "応答生成に失敗") {
				t.Fatalf("generation error history is not explicit: %q", entry.Message.Content)
			}
		}
	}
	if !found {
		t.Fatalf("generation error was not recorded in conversation history: %+v", entries)
	}
}

func TestActiveSessionTranscriptReturnsCurrentIdleSessionInOrder(t *testing.T) {
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(nil, memory, []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")
	o.mu.Lock()
	o.activeSessionID = "idle-current"
	o.mu.Unlock()

	old := domaintransport.NewMessage("shiro", "mio", "idle-old", "", "古いセッションです。")
	old.Type = domaintransport.MessageTypeIdleChat
	memory.RecordMessage(old)
	first := domaintransport.NewMessage("mio", "shiro", "idle-current", "", "最初はMioです。")
	first.Type = domaintransport.MessageTypeIdleChat
	memory.RecordMessage(first)
	second := domaintransport.NewMessage("shiro", "mio", "idle-current", "", "次はShiroです。")
	second.Type = domaintransport.MessageTypeIdleChat
	memory.RecordMessage(second)

	sessionID, transcript := o.ActiveSessionTranscript(10)
	if sessionID != "idle-current" {
		t.Fatalf("sessionID = %q, want idle-current", sessionID)
	}
	if len(transcript) != 2 {
		t.Fatalf("transcript len = %d, want 2: %+v", len(transcript), transcript)
	}
	if transcript[0].From != "mio" || transcript[0].Content != "最初はMioです。" {
		t.Fatalf("first transcript entry = %+v", transcript[0])
	}
	if transcript[0].TurnIndex != 1 || transcript[0].MessageID != "idle-current:msg:0001" {
		t.Fatalf("first transcript identity = %+v", transcript[0])
	}
	if transcript[1].From != "shiro" || transcript[1].Content != "次はShiroです。" {
		t.Fatalf("second transcript entry = %+v", transcript[1])
	}
	if transcript[1].TurnIndex != 2 || transcript[1].MessageID != "idle-current:msg:0002" {
		t.Fatalf("second transcript identity = %+v", transcript[1])
	}
}

func TestEmitTopicUsesTopicEventOutsideConversationTurns(t *testing.T) {
	memory := session.NewCentralMemory()
	o := NewIdleChatOrchestrator(nil, memory, []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")
	var emitted []TimelineEvent
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		emitted = append(emitted, ev)
		return nil
	})

	o.emitTopicToTimeline("idle-topic-contract", "記憶と風景の関係", StrategyExternalStimulus)

	if len(emitted) != 1 {
		t.Fatalf("emitted len = %d, want 1", len(emitted))
	}
	if emitted[0].Type != "idlechat.topic" {
		t.Fatalf("topic event type = %q, want idlechat.topic", emitted[0].Type)
	}
	if emitted[0].MessageID != "idle-topic-contract:topic" || emitted[0].TurnIndex != 0 {
		t.Fatalf("topic identity = %+v", emitted[0])
	}
	o.mu.Lock()
	o.activeSessionID = "idle-topic-contract"
	o.mu.Unlock()
	_, transcript := o.ActiveSessionTranscript(10)
	if len(transcript) != 0 {
		t.Fatalf("topic should not be included in active transcript: %+v", transcript)
	}
}

func countTopicGenerationRequests(requests []llm.GenerateRequest) int {
	count := 0
	for _, req := range requests {
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" && req.Messages[0].Content == idleTopicGeneratorSystemPrompt() {
			count++
		}
	}
	return count
}
