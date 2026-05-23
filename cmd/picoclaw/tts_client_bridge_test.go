package main

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func TestBuildTTSClientBridge_Disabled(t *testing.T) {
	cfg := &config.Config{}
	if got := buildTTSClientBridge(cfg, nil, nil, nil); got != nil {
		t.Fatal("expected nil bridge when tts is disabled")
	}
}

func TestBuildTTSClientBridge_Enabled(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			HTTPBaseURL: "http://127.0.0.1:8765",
			VoiceID:     "female_01",
			TimeoutMS:   15000,
		},
	}
	if got := buildTTSClientBridge(cfg, nil, nil, nil); got == nil {
		t.Fatal("expected non-nil bridge when tts is enabled")
	}
}

func TestBuildTTSClientBridge_UsesRenCrowBridge(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			HTTPBaseURL: "http://127.0.0.1:8765",
			VoiceID:     "female_01",
			TimeoutMS:   15000,
		},
	}
	got := buildTTSClientBridge(cfg, nil, nil, nil)
	if got == nil {
		t.Fatal("expected non-nil bridge")
	}
	if _, ok := got.(*ttsinfra.RenCrowTTSBridge); !ok {
		t.Fatalf("expected RenCrowTTSBridge, got %T", got)
	}
}

func TestBuildTTSClientBridge_UsesIrodoriDirectBridge(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:   true,
			OutputDir: t.TempDir(),
			Irodori: config.TTSIrodoriConfig{
				Enabled: true,
				BaseURL: "http://127.0.0.1:7860",
				VoiceID: "mio",
			},
		},
	}
	got := buildTTSClientBridge(cfg, nil, nil, nil)
	if got == nil {
		t.Fatal("expected non-nil bridge")
	}
	if _, ok := got.(*ttsinfra.SBV2TTSBridge); !ok {
		t.Fatalf("expected generic direct TTS bridge, got %T", got)
	}
}

func TestBuildTTSClientBridge_WithoutPlaybackCommands(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			HTTPBaseURL: "http://127.0.0.1:8765",
			VoiceID:     "female_01",
			TimeoutMS:   15000,
		},
	}

	got := buildTTSClientBridge(cfg, nil, nil, nil)
	if got == nil {
		t.Fatal("expected non-nil bridge")
	}
	if _, ok := got.(*ttsinfra.RenCrowTTSBridge); !ok {
		t.Fatalf("expected RenCrowTTSBridge, got %T", got)
	}
}

func TestTTSPublicSessionRouteKeepsLogicalSessionAndGlobalChunkOrder(t *testing.T) {
	ttsPublicSessionMu.Lock()
	ttsPublicSessionRoutes = map[string]*ttsPublicSessionRoute{}
	ttsPublicNextChunk = map[string]int{}
	ttsPublicNextResponse = map[string]int{}
	ttsPublicSessionMu.Unlock()

	registerTTSPublicSession("idle-1-tts-a", "idle-1", "idle-1:0000")
	registerTTSPublicSession("idle-1-tts-b", "idle-1", "idle-1:0001")

	session, chunk := resolveTTSPublicChunk("idle-1-tts-a", 0)
	if session != "idle-1" || chunk != 0 {
		t.Fatalf("first chunk = %s/%d, want idle-1/0", session, chunk)
	}
	session, chunk = resolveTTSPublicChunk("idle-1-tts-a", 1)
	if session != "idle-1" || chunk != 1 {
		t.Fatalf("second chunk = %s/%d, want idle-1/1", session, chunk)
	}
	session, chunk = resolveTTSPublicChunk("idle-1-tts-b", 0)
	if session != "idle-1" || chunk != 2 {
		t.Fatalf("next utterance chunk = %s/%d, want idle-1/2", session, chunk)
	}
	session, chunk = resolveTTSPublicChunk("normal-session", 7)
	if session != "normal-session" || chunk != 7 {
		t.Fatalf("unmapped chunk = %s/%d, want passthrough", session, chunk)
	}
	if got := nextTTSPublicResponseID("idle-1"); got != "idle-1:0000" {
		t.Fatalf("first response id = %q", got)
	}
	if got := nextTTSPublicResponseID("idle-1"); got != "idle-1:0001" {
		t.Fatalf("second response id = %q", got)
	}
}
