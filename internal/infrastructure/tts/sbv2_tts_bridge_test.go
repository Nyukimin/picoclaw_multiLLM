package tts

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestSBV2TTSBridgeSplitsLongTextBeforeSynthesis(t *testing.T) {
	provider := &recordingProvider{}
	sink := &recordingSink{}
	var readyTexts []string
	var readyIndexes []int
	var readyAudioPaths []string
	bridge := NewSBV2TTSBridge(SBV2TTSBridgeConfig{
		Provider:  provider,
		Sink:      sink,
		OutputDir: t.TempDir(),
		OnChunkReady: func(_, _ string, chunkIndex int, _, text, _, audioPath, _ string) {
			readyIndexes = append(readyIndexes, chunkIndex)
			readyTexts = append(readyTexts, text)
			readyAudioPaths = append(readyAudioPaths, audioPath)
		},
	})
	if err := bridge.StartSession(context.Background(), orchestrator.TTSSessionStart{
		SessionID:   "s1",
		CharacterID: "mio",
		VoiceID:     "female_01",
	}); err != nil {
		t.Fatalf("start session: %v", err)
	}

	err := bridge.PushText(context.Background(), "s1", "今日はいい天気ですね。少し歩いてから、温かいお茶を飲みましょう。", nil)
	if err != nil {
		t.Fatalf("push text: %v", err)
	}

	if len(provider.texts) != 2 {
		t.Fatalf("expected 2 provider calls, got %d: %#v", len(provider.texts), provider.texts)
	}
	if provider.texts[0] != "今日はいい天気ですね。" || provider.texts[1] != "少し歩いてから、温かいお茶を飲みましょう。" {
		t.Fatalf("unexpected provider texts: %#v", provider.texts)
	}
	if len(readyTexts) != 2 || len(sink.chunks) != 2 {
		t.Fatalf("expected 2 ready/sink chunks, got ready=%d sink=%d", len(readyTexts), len(sink.chunks))
	}
	if readyIndexes[0] != 0 || readyIndexes[1] != 1 {
		t.Fatalf("unexpected chunk indexes: %#v", readyIndexes)
	}
	if readyAudioPaths[0] != "01.wav" || readyAudioPaths[1] != "02.wav" {
		t.Fatalf("unexpected viewer audio paths: %#v", readyAudioPaths)
	}
}

type recordingProvider struct {
	texts []string
}

func (p *recordingProvider) Name() string {
	return "recording"
}

func (p *recordingProvider) Synthesize(_ context.Context, in SynthesisInput) (SynthesisOutput, error) {
	p.texts = append(p.texts, in.Text)
	return SynthesisOutput{
		Provider:      p.Name(),
		AudioFilePath: fmt.Sprintf("%s/%02d.wav", in.OutputDir, len(p.texts)),
	}, nil
}

type recordingSink struct {
	chunks []audioChunk
}

func (s *recordingSink) SubmitChunk(_ context.Context, _ string, ch audioChunk) error {
	s.chunks = append(s.chunks, ch)
	return nil
}

func (s *recordingSink) CompleteSession(context.Context, string) error {
	return nil
}
