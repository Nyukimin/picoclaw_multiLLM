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
	response string
	block    bool
	requests []llm.GenerateRequest
}

func (p *capturingIdleProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	p.requests = append(p.requests, req)
	if p.block {
		<-ctx.Done()
		return llm.GenerateResponse{}, ctx.Err()
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
