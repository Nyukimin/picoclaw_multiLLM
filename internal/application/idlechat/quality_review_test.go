package idlechat

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

type queuedQualityProvider struct {
	responses []string
	requests  []llm.GenerateRequest
}

func (p *queuedQualityProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	p.requests = append(p.requests, req)
	if len(p.responses) == 0 {
		return llm.GenerateResponse{Content: "ok"}, nil
	}
	out := p.responses[0]
	p.responses = p.responses[1:]
	return llm.GenerateResponse{Content: out}, nil
}

func (p *queuedQualityProvider) Name() string {
	return "queued-quality"
}

func TestSaveSummaryReviewsQualityAndAppliesPromptGuidance(t *testing.T) {
	provider := &queuedQualityProvider{responses: []string{
		"会話の要約です。",
		"QUALITY: fail\nISSUES:\n- テンプレ反復で聞き手の楽しみが落ちた\nPROMPT_FIX: テンプレ入口を避け、毎回別の場面か反論から始める。",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil, "")
	transcript := []string{
		"mio: もし猫が市長だったら面白いよね。",
		"shiro: もし市長なら予算配分が変わりますね。",
		"mio: もし猫なら会議も変わるかも。",
		"shiro: もしそうなら制度設計が重要です。",
	}

	summary := o.saveSummary("s1", "猫市長", TopicStrategy("manual"), transcript, time.Now(), time.Now(), len(transcript), true, "template_repeat")

	if !strings.Contains(summary, "注記: テンプレ反復で打ち切り") {
		t.Fatalf("summary should keep loop note, got %q", summary)
	}
	if len(o.history) != 1 {
		t.Fatalf("history len = %d, want 1", len(o.history))
	}
	record := o.history[0]
	if !strings.Contains(record.QualityReview, "QUALITY: fail") {
		t.Fatalf("quality review not recorded: %q", record.QualityReview)
	}
	if !strings.Contains(record.PromptGuidance, "テンプレ入口を避け") {
		t.Fatalf("prompt guidance not recorded: %q", record.PromptGuidance)
	}
	if got := o.getSystemPrompt("mio"); !strings.Contains(got, "聞き手体験レビュー") || !strings.Contains(got, "テンプレ入口を避け") {
		t.Fatalf("system prompt does not include guidance: %q", got)
	}
}

func TestPromptGuidesFromHistoryLoadsRecentGuidance(t *testing.T) {
	history := []SessionSummary{
		{PromptGuidance: "古い補正"},
		{PromptGuidance: "新しい補正"},
		{PromptGuidance: "新しい補正"},
	}
	got := promptGuidesFromHistory(history, 2)
	if len(got) != 2 || got[0] != "古い補正" || got[1] != "新しい補正" {
		t.Fatalf("unexpected guides: %#v", got)
	}
}
