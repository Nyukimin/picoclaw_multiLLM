package orchestrator

import (
	"context"
	"testing"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

func TestSplitTTSChunksSplitsLongFinalText(t *testing.T) {
	text := "今日はいい天気ですね。少し歩いてから、温かいお茶を飲みましょう。最後に明日の予定も確認します。"

	chunks := SplitTTSChunks(text)

	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %#v", len(chunks), chunks)
	}
	want := []string{
		"今日はいい天気ですね。",
		"少し歩いてから、温かいお茶を飲みましょう。",
		"最後に明日の予定も確認します。",
	}
	for i := range want {
		if chunks[i] != want[i] {
			t.Fatalf("chunk[%d] = %q, want %q", i, chunks[i], want[i])
		}
	}
}

func TestSplitTTSChunksForceSplitsTextWithoutBoundaries(t *testing.T) {
	text := "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"

	chunks := SplitTTSChunks(text)

	if len(chunks) != 2 {
		t.Fatalf("expected forced split into 2 chunks, got %d: %#v", len(chunks), chunks)
	}
	if len([]rune(chunks[0])) != ttsChunkMaxRunes {
		t.Fatalf("expected first chunk length %d, got %d", ttsChunkMaxRunes, len([]rune(chunks[0])))
	}
}

func TestTTSStreamForwarderFinalizeSplitsUnemittedFinalText(t *testing.T) {
	bridge := &recordingTTSBridge{}
	forwarder := newTTSStreamForwarder(bridge, "s1", routing.RouteCHAT, "agent.response", "tts:")

	forwarder.Finalize(context.Background(), "今日はいい天気ですね。少し歩いてから、温かいお茶を飲みましょう。")

	if len(bridge.texts) != 2 {
		t.Fatalf("expected 2 pushed chunks, got %d: %#v", len(bridge.texts), bridge.texts)
	}
}

type recordingTTSBridge struct {
	texts []string
}

func (b *recordingTTSBridge) StartSession(context.Context, TTSSessionStart) error {
	return nil
}

func (b *recordingTTSBridge) PushText(_ context.Context, _ string, text string, _ *ttsapp.EmotionState) error {
	b.texts = append(b.texts, text)
	return nil
}

func (b *recordingTTSBridge) EndSession(context.Context, string) error {
	return nil
}
