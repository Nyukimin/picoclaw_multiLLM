package ttsapp

import (
	"strings"
	"testing"
)

func TestEnsureEmotionPrefixAddsPaletteEmoji(t *testing.T) {
	text := EnsureEmotionPrefix("こんにちは。", &EmotionState{
		PrimaryEmotion: "cheerful",
		EmotionVector:  EmotionVector{Cheerfulness: 0.8},
	})
	if !strings.HasPrefix(text, "😆") {
		t.Fatalf("expected cheerful emoji prefix, got %q", text)
	}
	if !HasEmotionPrefix(text) {
		t.Fatalf("expected palette prefix to be detected: %q", text)
	}
}

func TestEnsureEmotionPrefixDoesNotDuplicatePaletteEmoji(t *testing.T) {
	text := EnsureEmotionPrefix("🤔 もう少し考えます。", &EmotionState{PrimaryEmotion: "calm"})
	if strings.Count(text, "🤔") != 1 {
		t.Fatalf("expected existing prefix to be preserved without duplication, got %q", text)
	}
}

func TestEnsureEmotionPrefixUsesDefaultWhenEmotionMissing(t *testing.T) {
	text := EnsureEmotionPrefix("本文です。", nil)
	if !strings.HasPrefix(text, "😌") {
		t.Fatalf("expected default calm prefix, got %q", text)
	}
}

func TestEnsureEmotionPrefixForCharacterUsesMioBrightBaseAndLargeSwing(t *testing.T) {
	base := EnsureEmotionPrefixForCharacter("今日は普通の話です。", nil, "mio")
	if !strings.HasPrefix(base, "😊") {
		t.Fatalf("expected Mio bright base prefix, got %q", base)
	}
	swung := EnsureEmotionPrefixForCharacter("最高！すごいね！", nil, "mio")
	if !strings.HasPrefix(swung, "😆") {
		t.Fatalf("expected Mio to swing brightly for joyful text, got %q", swung)
	}
}

func TestEnsureEmotionPrefixForCharacterKeepsShiroCalmUnlessStrong(t *testing.T) {
	base := EnsureEmotionPrefixForCharacter("少し考えてみよう。", nil, "shiro")
	if !strings.HasPrefix(base, "😇") {
		t.Fatalf("expected Shiro calm base prefix, got %q", base)
	}
	swung := EnsureEmotionPrefixForCharacter("エラーです、すぐ注意してください！", nil, "shiro")
	if !strings.HasPrefix(swung, "😮") {
		t.Fatalf("expected Shiro to swing only on strong emotion, got %q", swung)
	}
}
