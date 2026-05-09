package idlechat

import (
	"strings"
	"testing"
)

func TestNormalizeForecastDisplayTopicFallsBackWhenEmpty(t *testing.T) {
	domain := ForecastDomain{Name: "技術"}
	got := normalizeForecastDisplayTopic(domain, "")
	if got != "技術の3年後を考える" {
		t.Fatalf("fallback topic = %q", got)
	}
}

func TestBuildForecastLLMTopicNeverStartsWithEmptyTopic(t *testing.T) {
	domain := ForecastDomain{Name: "社会"}
	got := buildForecastLLMTopic(domain, "   ", nil)
	if !strings.Contains(got, "【社会 未来展望】社会の3年後を考える") {
		t.Fatalf("LLM topic did not include fallback display topic:\n%s", got)
	}
}
