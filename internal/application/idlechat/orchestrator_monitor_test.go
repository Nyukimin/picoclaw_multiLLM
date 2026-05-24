package idlechat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
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

func countTopicGenerationRequests(requests []llm.GenerateRequest) int {
	count := 0
	for _, req := range requests {
		if len(req.Messages) > 0 && req.Messages[0].Role == "system" && req.Messages[0].Content == idleTopicGeneratorSystemPrompt() {
			count++
		}
	}
	return count
}
