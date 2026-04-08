package tts

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestSBV2DirectBridge_PushTextSynthesizesChunk(t *testing.T) {
	tmpDir := t.TempDir()
	provider := NewSBV2Provider(SBV2Config{
		BaseURL: "http://sbv2.local/api/synthesis",
		VoiceID: "jvnv-F1-jp",
	})
	provider.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/models_info":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"name":"jvnv-F1-jp","files":["model_assets\\jvnv-F1-jp\\voice.safetensors"],"speakers":["jvnv-F1-jp"]}]`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/g2p":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`[{"mora":"コ","tone":0}]`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/synthesis":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("RIFFchunk")),
				Header:     http.Header{"Content-Type": []string{"audio/wav"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}

	sink := &sinkStub{}
	var gotChunk audioChunk
	bridge := NewSBV2DirectBridge(SBV2DirectBridgeConfig{
		Provider:  provider,
		Sink:      sink,
		OutputDir: tmpDir,
		OnChunkReady: func(sessionID string, chunkIndex int, characterID, text, audioPath, audioURL string) {
			gotChunk = audioChunk{
				ChunkIndex: chunkIndex,
				Text:       text,
				AudioPath:  audioPath,
				AudioURL:   audioURL,
			}
		},
	})

	if err := bridge.StartSession(context.Background(), orchestrator.TTSSessionStart{
		SessionID:   "s1",
		CharacterID: "mio",
		VoiceID:     "jvnv-F1-jp",
	}); err != nil {
		t.Fatalf("start session failed: %v", err)
	}
	if err := bridge.PushText(context.Background(), "s1", "こんにちは", nil); err != nil {
		t.Fatalf("push text failed: %v", err)
	}
	if gotChunk.ChunkIndex != 0 {
		t.Fatalf("unexpected chunk index: %+v", gotChunk)
	}
	if gotChunk.Text != "こんにちは。" {
		t.Fatalf("expected punctuated chunk text, got %q", gotChunk.Text)
	}
	if gotChunk.AudioPath == "" {
		t.Fatalf("expected local wav path, got %+v", gotChunk)
	}
	if sink.calls != 1 {
		t.Fatalf("expected sink submit once, got %d", sink.calls)
	}
	if err := bridge.EndSession(context.Background(), "s1"); err != nil {
		t.Fatalf("end session failed: %v", err)
	}
}
