package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

func TestParseAudioChunk_WithAudioURLOnly(t *testing.T) {
	ch, ok := parseAudioChunk(map[string]any{
		"type":        "audio_chunk_ready",
		"chunk_index": 1,
		"text":        "hello",
		"audio_url":   "/cache/s1.wav",
	})
	if !ok {
		t.Fatal("expected chunk to be accepted with audio_url")
	}
	if ch.ChunkIndex != 1 || ch.AudioURL != "/cache/s1.wav" {
		t.Fatalf("unexpected chunk: %+v", ch)
	}
}

func TestParseAudioChunk_WithAudioPath(t *testing.T) {
	ch, ok := parseAudioChunk(map[string]any{
		"type":        "audio_chunk_ready",
		"chunk_index": 2,
		"audio_path":  `cache\\s2.wav`,
	})
	if !ok {
		t.Fatal("expected chunk to be accepted with audio_path")
	}
	if ch.ChunkIndex != 2 || ch.AudioPath != `cache\\s2.wav` {
		t.Fatalf("unexpected chunk: %+v", ch)
	}
}

func TestClientBridge_PushText_FallbackSynthesize(t *testing.T) {
	var got map[string]any

	sink := &sinkStub{}
	gotChunk := false
	b := NewClientBridge(ClientConfig{
		HTTPBaseURL: "http://tts.local",
		OnChunkReady: func(sessionID string, chunkIndex int, text, audioPath, audioURL string) {
			gotChunk = sessionID == "s1" && chunkIndex == 0 && audioURL == "http://tts.local/cache/x.wav"
		},
	}, sink)
	b.client = &http.Client{
		Transport: clientBridgeRoundTripper(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/synthesize" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"text":"hi","audio_path":"cache\\x.wav","audio_url":"/cache/x.wav"}`)),
			}, nil
		}),
	}

	emotion := &ttsapp.EmotionState{PrimaryEmotion: "warm"}
	if err := b.PushText(context.Background(), "s1", "hello", emotion); err != nil {
		t.Fatalf("expected fallback synth success, got %v", err)
	}
	if got["emotion_state"] == nil {
		t.Fatal("expected emotion_state in fallback request")
	}
	if !gotChunk {
		t.Fatal("expected on chunk callback")
	}
	if sink.calls != 1 {
		t.Fatalf("expected sink submit once, got %d", sink.calls)
	}
}

type clientBridgeRoundTripper func(*http.Request) (*http.Response, error)

func (f clientBridgeRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type sinkStub struct {
	calls int
}

func (s *sinkStub) SubmitChunk(_ context.Context, _ string, _ audioChunk) error {
	s.calls++
	return nil
}

func (s *sinkStub) CompleteSession(_ context.Context, _ string) error {
	return nil
}
