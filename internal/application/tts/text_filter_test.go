package ttsapp

import "testing"

func TestFilterSpeakableText_IgnoresThinking(t *testing.T) {
	got := FilterSpeakableText("agent.thinking", "CHAT", "途中")
	if got != "" {
		t.Fatalf("expected empty for thinking, got %q", got)
	}
}

func TestFilterSpeakableText_StripsNotesAndURL(t *testing.T) {
	in := "（internal note）おはようございます。\nhttps://example.com"
	got := FilterSpeakableText("agent.response", "CHAT", in)
	if got != "おはようございます。" {
		t.Fatalf("unexpected filtered text: %q", got)
	}
}

func TestFilterSpeakableText_StripsAckPrefix(t *testing.T) {
	in := "はい、承知いたしました。おはようございます！"
	got := FilterSpeakableText("agent.response", "CHAT", in)
	if got != "おはようございます！" {
		t.Fatalf("unexpected filtered text: %q", got)
	}
}
