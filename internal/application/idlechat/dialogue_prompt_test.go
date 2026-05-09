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

type capturingIdleProvider struct {
	response      string
	block         bool
	requests      []llm.GenerateRequest
	responses     []string
	finishReasons []string
}

func (p *capturingIdleProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	p.requests = append(p.requests, req)
	if p.block {
		<-ctx.Done()
		return llm.GenerateResponse{}, ctx.Err()
	}
	if len(p.responses) > 0 {
		response := p.responses[0]
		p.responses = p.responses[1:]
		finish := ""
		if len(p.finishReasons) > 0 {
			finish = p.finishReasons[0]
			p.finishReasons = p.finishReasons[1:]
		}
		return llm.GenerateResponse{Content: response, FinishReason: finish}, nil
	}
	return llm.GenerateResponse{Content: p.response}, nil
}

func (p *capturingIdleProvider) Name() string { return "capturing-idle" }

func TestGenerateResponseFirstTurnUsesActualSpeaker(t *testing.T) {
	provider := &capturingIdleProvider{response: "郵便配達員が古書店の棚で宛先不明の手紙を見つける入口がよさそう。しろなら、誰が隠したと思う？"}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	_, err := o.generateResponse("mio", "shiro", "idle-dialogue-first", 0, 0, "郵便と古書店")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if len(provider.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(provider.requests))
	}
	last := provider.requests[0].Messages[len(provider.requests[0].Messages)-1].Content
	if !strings.Contains(last, "mioとして1-2文で始めてください") {
		t.Fatalf("first turn prompt should use actual speaker mio:\n%s", last)
	}
	if strings.Contains(last, "shiroとして1-2文で始めてください") {
		t.Fatalf("first turn prompt leaked target as speaker:\n%s", last)
	}
}

func TestGenerateResponseRecoversEmptyTopicFromSessionMemory(t *testing.T) {
	provider := &capturingIdleProvider{response: "郵便と古書店なら、宛先不明の手紙を最初の手がかりにすると入れそう。しろなら、その手紙を誰が隠したと思う？"}
	memory := session.NewCentralMemory()
	memory.RecordMessage(domaintransport.NewMessage("user", "mio", "idle-empty-topic", "", "今日のお題（external）: 郵便と古書店"))
	o := NewIdleChatOrchestrator(provider, memory, []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	_, err := o.generateResponse("mio", "shiro", "idle-empty-topic", 0, 0, "")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if len(provider.requests) == 0 {
		t.Fatal("provider was not called")
	}
	last := provider.requests[0].Messages[len(provider.requests[0].Messages)-1].Content
	if !strings.Contains(last, "話題: 郵便と古書店") {
		t.Fatalf("empty topic was not recovered from memory:\n%s", last)
	}
	if strings.Contains(last, "話題: \n") {
		t.Fatalf("empty topic reached prompt:\n%s", last)
	}
}

func TestGenerateResponseNeverPassesEmptyTopicToProvider(t *testing.T) {
	provider := &capturingIdleProvider{response: "この会話の現在のお題なら、まず具体的な入口を一つ決めると話しやすいです。みおなら、どの場面から始めますか？"}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	_, err := o.generateResponse("shiro", "mio", "idle-empty-topic-fallback", 0, 0, "")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if len(provider.requests) == 0 {
		t.Fatal("provider was not called")
	}
	last := provider.requests[0].Messages[len(provider.requests[0].Messages)-1].Content
	if !strings.Contains(last, "話題: この会話の現在のお題") {
		t.Fatalf("fallback topic was not injected:\n%s", last)
	}
	if strings.Contains(last, "話題: \n") {
		t.Fatalf("empty topic reached prompt:\n%s", last)
	}
}

func TestGenerateResponseRejectsInternalReasoningLeak(t *testing.T) {
	provider := &capturingIdleProvider{
		responses: []string{
			"channel>thought\nユーザーは私（Mio）に対して、会話の制約を課している。\n1. **キャラクター**: Mio\n2. **目標**: 自然な返答。",
			"えー、その映写室で音が少し遅れて聞こえる瞬間、秘密の入口っぽくて気になるよ。shiroなら、そのズレを誰が仕込んだと思う？",
		},
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-leak", 0, 0, "小さな映画館の音響空間")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if strings.Contains(got, "channel>thought") || strings.Contains(got, "ユーザーは私") || strings.Contains(got, "制約") {
		t.Fatalf("internal reasoning leaked into response: %q", got)
	}
	if len(provider.requests) < 2 {
		t.Fatalf("expected retry after leaked response, requests=%d", len(provider.requests))
	}
}

func TestGenerateResponseFallsBackWhenInternalReasoningPersists(t *testing.T) {
	provider := &capturingIdleProvider{
		response: "channel>thought\nユーザーは私（Mio）に対して、会話の制約を課している。\n1. **キャラクター**: Mio\n2. **目標**: 自然な返答。",
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-leak-fallback", 0, 0, "小さな映画館の音響空間")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if strings.Contains(got, "channel>thought") || strings.Contains(got, "ユーザーは私") || strings.Contains(got, "制約") {
		t.Fatalf("internal reasoning leaked into fallback response: %q", got)
	}
	if !strings.Contains(got, "小さな映画館の音響空間") {
		t.Fatalf("fallback should keep topic context: %q", got)
	}
}

func TestGenerateResponseStopsFallbackBeforeRepeatingCycle(t *testing.T) {
	provider := &capturingIdleProvider{
		response: "channel>thought\nユーザーは私（Mio）に対して、会話の制約を課している。",
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-leak-stop", 8, 8, "映画館の映写担当")
	if err == nil {
		t.Fatal("generateResponse() error = nil, want invalid response after fallback cycle")
	}
	if got != "" {
		t.Fatalf("generateResponse() = %q, want empty", got)
	}
}

func TestGenerateResponseRetriesTruncatedFinishReason(t *testing.T) {
	provider := &capturingIdleProvider{
		responses: []string{
			"小さな映画館の映写室で、音のズレに気づいた主人公が",
			"その音のズレ、映写室の床下に誰かが隠した古いスピーカーのせいかもね。shiroなら、最初にどこを調べる？",
		},
		finishReasons: []string{"length", "stop"},
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-truncated", 0, 0, "小さな映画館の音響空間")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if strings.Contains(got, "主人公が") {
		t.Fatalf("truncated response was accepted: %q", got)
	}
	if !strings.Contains(got, "古いスピーカー") {
		t.Fatalf("retry response was not used: %q", got)
	}
	if len(provider.requests) < 2 {
		t.Fatalf("expected retry after truncated response, requests=%d", len(provider.requests))
	}
}

func TestNormalizeIdleTopicRejectsInternalReasoningLeak(t *testing.T) {
	raw := "channel>thought\nユーザーは私（Mio）に対して、お題生成の制約を確認している。"
	if got := normalizeIdleTopic(raw, false); got != "" {
		t.Fatalf("normalizeIdleTopic() = %q, want empty", got)
	}
}

func TestNormalizeIdleTopicExtractsFinalChannel(t *testing.T) {
	raw := "<|channel>thought\nまず候補を考える。\n<|channel>final\n古書店の棚に残る時間の質感"
	if got := normalizeIdleTopic(raw, false); got != "古書店の棚に残る時間の質感" {
		t.Fatalf("normalizeIdleTopic() = %q", got)
	}
}

func TestNormalizeIdleTopicRejectsTruncatedTopic(t *testing.T) {
	raw := "壊れたオルゴールの音色と麻雀卓の静寂が紡ぐ、取り"
	if got := normalizeIdleTopic(raw, true); got != "" {
		t.Fatalf("normalizeIdleTopic() = %q, want empty", got)
	}
}

func TestSanitizeIdleResponseExtractsFinalChannel(t *testing.T) {
	raw := "<|channel>thought\n制約を確認する。\n<|channel>final\nえー、その古書の紙の匂いから前の持ち主が見えてくる感じ、すごくいいね。shiroなら、最初にどの本を開く？"
	got := sanitizeIdleResponse(raw, "古書店")
	if strings.Contains(got, "thought") || strings.Contains(got, "制約") {
		t.Fatalf("reasoning leaked after sanitize: %q", got)
	}
	if !strings.Contains(got, "紙の匂い") {
		t.Fatalf("final answer was not extracted: %q", got)
	}
}

func TestBuildIdleTurnPromptRequiresDialogueResponse(t *testing.T) {
	got := buildIdleTurnPrompt("郵便と古書店", "shiro", "古書店に届く宛先不明の手紙って、誰かの記憶みたいだね。", "配達記録が鍵になりそうです。", 1, 1, false)

	for _, want := range []string{
		"これは独白ではなく二人の対話です",
		"直前の相手発言の論点・疑問・具体語のどれかを必ず受けてから返す",
		"1文目は相手の発言への反応",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, got)
		}
	}
}

func TestLatestOtherUtteranceUsesPreviousSpeakerLine(t *testing.T) {
	memory := session.NewCentralMemory()
	memory.RecordMessage(domaintransport.NewMessage("user", "mio", "idle-dialogue-context", "", "今日のお題（external）: 郵便と古書店"))
	memory.RecordMessage(domaintransport.NewMessage("mio", "shiro", "idle-dialogue-context", "", "古書店に届く宛先不明の手紙って、誰かの記憶みたいだね。"))

	got := latestOtherUtterance(memory.GetUnifiedView(10), "idle-dialogue-context", "shiro")
	if got != "古書店に届く宛先不明の手紙って、誰かの記憶みたいだね。" {
		t.Fatalf("latestOtherUtterance() = %q", got)
	}
}

func TestGenerateResponseFallsBackWhenIdleLLMTimesOut(t *testing.T) {
	provider := &capturingIdleProvider{block: true}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")
	old := idleChatLLMGenerateTimeout
	idleChatLLMGenerateTimeout = 10 * time.Millisecond
	defer func() { idleChatLLMGenerateTimeout = old }()

	got, err := o.generateResponse("shiro", "mio", "idle-dialogue-timeout", 1, 1, "郵便と古書店")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if !strings.Contains(got, "郵便と古書店") {
		t.Fatalf("fallback response should keep topic context: %q", got)
	}
}
