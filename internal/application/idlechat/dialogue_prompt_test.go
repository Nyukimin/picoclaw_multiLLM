package idlechat

import (
	"context"
	"errors"
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
	if len(provider.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(provider.requests))
	}
	last := provider.requests[0].Messages[len(provider.requests[0].Messages)-1].Content
	if !strings.Contains(last, "mioとして、会話の最初の発話を1〜2文") {
		t.Fatalf("first turn prompt should use actual speaker mio:\n%s", last)
	}
	if strings.Contains(last, "shiroとして、会話の最初の発話") {
		t.Fatalf("first turn prompt leaked target as speaker:\n%s", last)
	}
}

func TestGenerateResponseSelectsMoreFunCandidate(t *testing.T) {
	provider := &capturingIdleProvider{responses: []string{
		"その話題は構造を考えると面白いですね。もう少し整理できそうです。",
		"雨の古書店で宛先不明の手紙が棚から落ちたら、隠した人の嘘まで濡れて見えそう。しろなら、その封筒を開ける？",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-fun-candidate", 0, 0, "郵便と古書店")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(provider.requests))
	}
	secondPrompt := provider.requests[1].Messages[len(provider.requests[1].Messages)-1].Content
	if !strings.Contains(secondPrompt, "別候補") || !strings.Contains(secondPrompt, "楽しさ") {
		t.Fatalf("second candidate prompt does not request a fun alternative:\n%s", secondPrompt)
	}
	if !strings.Contains(secondPrompt, "英語だけの応答") {
		t.Fatalf("second candidate prompt does not ban English-only responses:\n%s", secondPrompt)
	}
	if len(provider.requests[1].Messages) < 2 || provider.requests[1].Messages[len(provider.requests[1].Messages)-2].Role != "assistant" {
		t.Fatalf("second candidate request should include first candidate as assistant context: %+v", provider.requests[1].Messages)
	}
	if !strings.Contains(got, "宛先不明の手紙") || !strings.Contains(got, "封筒を開ける") {
		t.Fatalf("more fun candidate was not selected: %q", got)
	}
}

func TestGenerateResponseTenThemesDoNotUseFallback(t *testing.T) {
	topics := []string{
		"郵便と古書店",
		"雨の文化祭前夜",
		"港町の倉庫街",
		"壊れたオルゴール",
		"深夜の自動販売機",
		"古い団地の掲示板",
		"地下鉄の忘れ物",
		"夏祭りの裏通り",
		"閉館後の図書室",
		"朝の市場と手紙",
	}
	responses := make([]string, 0, len(topics)*2)
	for _, topic := range topics {
		responses = append(responses,
			topic+"なら、最初に小さな違和感を一つ置くと話が入りやすいね。誰がそれに気づくかで空気が変わりそう。",
			topic+"で濡れた封筒が一つだけ残っていたら、隠した人の嘘まで見えてきそう。開けるか戻すか、そこで性格が出るよね。",
		)
	}
	provider := &capturingIdleProvider{responses: responses}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	for i, topic := range topics {
		got, err := o.generateResponse("mio", "shiro", "idle-ten-themes", i, i, topic)
		if err != nil {
			t.Fatalf("theme %d %q generateResponse() error = %v", i, topic, err)
		}
		if !strings.Contains(got, topic) {
			t.Fatalf("theme %d %q lost topic context: %q", i, topic, got)
		}
	}
	if len(provider.requests) != len(topics)*2 {
		t.Fatalf("requests = %d, want %d", len(provider.requests), len(topics)*2)
	}
}

func TestIdleFunScorePercentRanksConcreteHooksHigher(t *testing.T) {
	dull := "その話題は構造を考えると面白いですね。もう少し整理できそうです。"
	fun := "雨の古書店で宛先不明の手紙が棚から落ちたら、隠した人の嘘まで濡れて見えそう。しろなら、その封筒を開ける？"

	dullScore := idleFunScorePercent(dull, "", "", "郵便と古書店")
	funScore := idleFunScorePercent(fun, "", "", "郵便と古書店")

	if funScore <= dullScore {
		t.Fatalf("fun score should prefer concrete hook: fun=%d dull=%d", funScore, dullScore)
	}
	if funScore < 70 {
		t.Fatalf("concrete hook should score as clearly fun, got %d", funScore)
	}
}

func TestIdleCompactRetryMessagesBanEnglishOutput(t *testing.T) {
	messages := buildIdleCompactRetryMessages("mio", "郵便と古書店", "", "会話の最初の発話")
	joined := ""
	for _, msg := range messages {
		joined += msg.Content + "\n"
	}
	for _, want := range []string{"自然な日本語", "英語だけの応答", "英語の見出し", "英語での説明"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compact retry messages do not contain %q:\n%s", want, joined)
		}
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

func TestGenerateResponseRejectsEnglishReasoningLeak(t *testing.T) {
	provider := &capturingIdleProvider{
		responses: []string{
			"Okay, let's see. The user is asking me to respond as Shiro in two sentences. The task is to acknowledge the point and add a concrete example.",
			"その見方なら、道具の傷は使い手の失敗も直した回数も一緒に残している感じがあります。例えば修理屋の定規なら、削れた端だけで前の持ち主の癖まで見えてきそうです。",
		},
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("shiro", "mio", "idle-english-leak", 3, 3, "使い古された道具の魂")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if strings.Contains(strings.ToLower(got), "okay") || strings.Contains(strings.ToLower(got), "the user is asking") {
		t.Fatalf("english reasoning leaked into response: %q", got)
	}
	if !strings.Contains(got, "道具") {
		t.Fatalf("retry response was not used: %q", got)
	}
	if len(provider.requests) < 2 {
		t.Fatalf("expected retry after English reasoning leak, requests=%d", len(provider.requests))
	}
}

func TestHasInternalReasoningLeakDetectsEnglishReasoning(t *testing.T) {
	raw := "Okay, let's see. The user is asking me to respond as Shiro in two sentences. The task is to acknowledge the point and add a concrete example."
	if !hasInternalReasoningLeak(raw) {
		t.Fatalf("English reasoning leak was not detected: %q", raw)
	}
}

func TestGenerateResponseReturnsErrorWhenInternalReasoningPersists(t *testing.T) {
	provider := &capturingIdleProvider{
		response: "channel>thought\nユーザーは私（Mio）に対して、会話の制約を課している。\n1. **キャラクター**: Mio\n2. **目標**: 自然な返答。",
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")

	got, err := o.generateResponse("mio", "shiro", "idle-leak-fallback", 0, 0, "小さな映画館の音響空間")
	if err == nil {
		t.Fatal("generateResponse() error = nil, want invalid response")
	}
	if !errors.Is(err, errIdleInvalidResponse) {
		t.Fatalf("expected errIdleInvalidResponse, got %v", err)
	}
	if got != "" {
		t.Fatalf("generateResponse() = %q, want empty", got)
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

func TestSanitizeIdleResponseExtractsFinalAnswerAfterEnglishThink(t *testing.T) {
	raw := "Okay, let's think this through step by step.\nThe user wants a short Japanese reply.\nFinal answer: それ、閉館間際の温室で温度計だけが先に嘘をつく瞬間かも。shiroなら、その嘘を最初に誰が見破ると思う？"
	got := sanitizeIdleResponse(raw, "閉館間際の温室")
	if strings.Contains(strings.ToLower(got), "okay, let's") || strings.Contains(strings.ToLower(got), "final answer") {
		t.Fatalf("reasoning leaked after sanitize: %q", got)
	}
	if !strings.Contains(got, "温度計") {
		t.Fatalf("final answer was not extracted: %q", got)
	}
}

func TestSanitizeIdleResponseExtractsTrailingJapaneseBlock(t *testing.T) {
	raw := "Okay, let's reason carefully.\nThe user asks for a concise response.\n\nその鍵穴、雨で膨らんだ木が一晩だけ塞いでいたのかも。mioなら、朝一番でどこを触って確かめる？"
	got := sanitizeIdleResponse(raw, "古書店")
	if strings.Contains(strings.ToLower(got), "okay, let's") {
		t.Fatalf("reasoning leaked after sanitize: %q", got)
	}
	if !strings.Contains(got, "鍵穴") {
		t.Fatalf("trailing japanese block was not extracted: %q", got)
	}
}

func TestSanitizeIdleSummaryResponseExtractsFinalAnswer(t *testing.T) {
	raw := "Okay, let's see. I need to summarize.\nFinal answer: 一番面白かったのは、フィルムと筐体の記号が秘密の合図みたいに同期していた点です。記号の時代背景を軸にすると、次の論点が自然に広がります。"
	got := sanitizeIdleSummaryResponse(raw, "映写")
	if strings.Contains(strings.ToLower(got), "okay, let's") || strings.Contains(strings.ToLower(got), "final answer") {
		t.Fatalf("reasoning leaked after summary sanitize: %q", got)
	}
	if !strings.Contains(got, "一番面白かった") {
		t.Fatalf("summary body was not extracted: %q", got)
	}
}

func TestSanitizeIdleSummaryResponseRejectsReasoningOnly(t *testing.T) {
	raw := "Okay, let's see. The user wants me to summarize in Japanese with three points."
	got := sanitizeIdleSummaryResponse(raw, "映写")
	if got != "" {
		t.Fatalf("expected empty summary for reasoning-only output, got %q", got)
	}
}

func TestSanitizeIdleSummaryResponseDropsLeadingReasoningParagraphs(t *testing.T) {
	raw := "Okay, so the user wants me to summarize this idle chat in three points.\n\n一番面白かったのは、フィルムと筐体の記号同期が秘密の合図みたいに見えた点です。記号の時代背景を追うと、次の議論が自然に広がりそうです。"
	got := sanitizeIdleSummaryResponse(raw, "映写")
	if strings.Contains(strings.ToLower(got), "okay, so") {
		t.Fatalf("leading reasoning paragraph remained: %q", got)
	}
	if !strings.Contains(got, "一番面白かった") {
		t.Fatalf("summary body was not preserved: %q", got)
	}
}

func TestSanitizeIdleSummaryResponseDropsEnglishMetaReasoningAndKeepsBody(t *testing.T) {
	raw := "Wait, the user provided an IdleChat log and wants me to evaluate it. First, I need to understand the output format and the required fields.\n\n1. 錆びた鍵に隠された秘密のドアが気になる。"
	got := sanitizeIdleSummaryResponse(raw, "路地")
	if strings.Contains(strings.ToLower(got), "the user provided") || strings.Contains(strings.ToLower(got), "output format") {
		t.Fatalf("english meta reasoning remained: %q", got)
	}
	if !strings.Contains(got, "錆びた鍵") {
		t.Fatalf("summary body was not preserved: %q", got)
	}
}

func TestBuildIdleTurnPromptRequiresDialogueResponse(t *testing.T) {
	got := buildIdleTurnPrompt("郵便と古書店", "shiro", "古書店に届く宛先不明の手紙って、誰かの記憶みたいだね。", "配達記録が鍵になりそうです。", 1, 1, false)

	for _, want := range []string{
		"直前の相手発言",
		"直前の相手発言を受けて1〜2文",
		"具体物・理由・問い",
		"自然な日本語だけ",
		"英語や説明は書かない",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt does not contain %q:\n%s", want, got)
		}
	}
}

func TestBuildIdleResponseGuardPromptBansEnglishOutput(t *testing.T) {
	got := buildIdleResponseGuardPrompt("mio", nil, nil)
	for _, want := range []string{"自然な日本語", "英語だけの応答", "英語の見出し", "英語での説明"} {
		if !strings.Contains(got, want) {
			t.Fatalf("guard prompt does not contain %q:\n%s", want, got)
		}
	}
}

func TestGetSystemPromptPutsRuntimePolicyBeforeCommonPrompt(t *testing.T) {
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, map[string]string{
		"shiro": "COMMON SHIRO SYSTEM PROMPT\n\n---\n\n# IdleChat補正\n\nIDLECHAT SHIRO CORRECTION",
	}, "")

	got := o.getSystemPrompt("shiro")
	commonIdx := strings.Index(got, "COMMON SHIRO SYSTEM PROMPT")
	correctionIdx := strings.Index(got, "IDLECHAT SHIRO CORRECTION")
	runtimeIdx := strings.Index(got, "この会話はidleChatです")
	if commonIdx < 0 || correctionIdx < 0 || runtimeIdx < 0 {
		t.Fatalf("system prompt missing common, idle correction, or runtime policy:\n%s", got)
	}
	if !(runtimeIdx < commonIdx && commonIdx < correctionIdx) {
		t.Fatalf("system prompt should be runtime policy first, then common Shiro prompt, then IdleChat correction:\n%s", got)
	}
}

func TestGenerateResponseKeepsOnlyFirstMessageAsSystem(t *testing.T) {
	provider := &capturingIdleProvider{responses: []string{
		"古書店の棚から宛先不明の手紙が落ちるなら、まず隠した人の癖が見えます。封筒を開ける前に、誰の字かだけ決めたいですね。",
		"雨でにじんだ宛名だけ先に読めるなら、隠した人より受け取るはずだった人が気になります。そこを一人に絞ると話が動きますね。",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, map[string]string{
		"shiro": "COMMON SHIRO SYSTEM PROMPT\n\n# IdleChat補正\n\nIDLECHAT SHIRO CORRECTION",
	}, "")
	o.sessionContext = "SESSION CONTEXT SHOULD STAY IN FIRST SYSTEM"
	o.SetRecentTopicProvider(func(context.Context, int) ([]string, error) {
		return []string{"RECENT TOPIC SHOULD STAY IN FIRST SYSTEM"}, nil
	})

	_, err := o.generateResponse("shiro", "mio", "idle-system-role-layout", 1, 1, "郵便と古書店")
	if err != nil {
		t.Fatalf("generateResponse() error = %v", err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(provider.requests))
	}
	for reqIndex, req := range provider.requests {
		for msgIndex, msg := range req.Messages {
			if msgIndex == 0 {
				if msg.Role != "system" {
					t.Fatalf("request %d first role = %q, want system", reqIndex, msg.Role)
				}
				if !strings.Contains(msg.Content, "COMMON SHIRO SYSTEM PROMPT") ||
					!strings.Contains(msg.Content, "IDLECHAT SHIRO CORRECTION") ||
					!strings.Contains(msg.Content, "SESSION CONTEXT SHOULD STAY IN FIRST SYSTEM") ||
					!strings.Contains(msg.Content, "RECENT TOPIC SHOULD STAY IN FIRST SYSTEM") {
					t.Fatalf("request %d first system did not contain all persistent context:\n%s", reqIndex, msg.Content)
				}
				continue
			}
			if msg.Role == "system" {
				t.Fatalf("request %d message %d should not be system: %#v", reqIndex, msgIndex, msg)
			}
		}
	}
	secondReq := provider.requests[1]
	if len(secondReq.Messages) < 2 {
		t.Fatalf("second request too short: %#v", secondReq.Messages)
	}
	last := secondReq.Messages[len(secondReq.Messages)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "別候補") {
		t.Fatalf("fun candidate prompt should be a user message, got role=%q content=%q", last.Role, last.Content)
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

func TestGenerateResponseReturnsErrorWhenIdleLLMTimesOut(t *testing.T) {
	provider := &capturingIdleProvider{block: true}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil, "")
	old := idleChatLLMGenerateTimeout
	idleChatLLMGenerateTimeout = 10 * time.Millisecond
	defer func() { idleChatLLMGenerateTimeout = old }()

	got, err := o.generateResponse("shiro", "mio", "idle-dialogue-timeout", 1, 1, "郵便と古書店")
	if err == nil {
		t.Fatal("generateResponse() error = nil, want timeout error")
	}
	if !errors.Is(err, errIdleGenerationFailed) {
		t.Fatalf("expected errIdleGenerationFailed, got %v", err)
	}
	if got != "" {
		t.Fatalf("generateResponse() = %q, want empty", got)
	}
}
