package tts

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

func TestEnsureEmotionPrefixForCharacterKeepsMioWarmStateBright(t *testing.T) {
	text := EnsureEmotionPrefixForCharacter("その瞬間、乗り手の心の中もきっと大きな変化があったんじゃないかな。", &EmotionState{
		PrimaryEmotion: "warm",
		EmotionVector:  EmotionVector{Warmth: 0.80, Cheerfulness: 0.40},
	}, "mio")
	if !strings.HasPrefix(text, "😊") {
		t.Fatalf("expected Mio warm state to keep bright base, got %q", text)
	}
}

func TestEnsureEmotionPrefixForCharacterUsesMioAffectionOnlyForExplicitAffection(t *testing.T) {
	ordinarySupport := EnsureEmotionPrefixForCharacter("生徒のなぜに寄り添うような対話が鍵になりそうだよ。", nil, "mio")
	if !strings.HasPrefix(ordinarySupport, "🤔") {
		t.Fatalf("expected ordinary supportive thinking text not to become affection, got %q", ordinarySupport)
	}
	explicitAffection := EnsureEmotionPrefixForCharacter("その気持ち、すごく大切で好きだよ。", nil, "mio")
	if !strings.HasPrefix(explicitAffection, "🥰") {
		t.Fatalf("expected explicit affection to use affection prefix, got %q", explicitAffection)
	}
}

func TestEnsureEmotionPrefixForTopicAnnouncementUsesBrightPrefix(t *testing.T) {
	text := EnsureEmotionPrefixForCharacter("きょうのおだい、車輪の軌跡と乗り手の皮膚感覚。", &EmotionState{
		PrimaryEmotion: "warm",
		EmotionVector:  EmotionVector{Warmth: 0.85},
	}, "user")
	if !strings.HasPrefix(text, "😊きょうのおだい") {
		t.Fatalf("expected topic announcement to start with bright prefix, got %q", text)
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
