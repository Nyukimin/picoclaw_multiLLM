package main

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func TestBuildTTSClientBridge_Disabled(t *testing.T) {
	cfg := &config.Config{}
	if got := buildTTSClientBridge(cfg, nil); got != nil {
		t.Fatal("expected nil bridge when tts is disabled")
	}
}

func TestBuildTTSClientBridge_Enabled(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			HTTPBaseURL: "http://127.0.0.1:8765",
			WSURL:       "ws://127.0.0.1:8765/sessions",
			VoiceID:     "female_01",
			SpeechMode:  "conversational",
		},
	}
	if got := buildTTSClientBridge(cfg, nil); got == nil {
		t.Fatal("expected non-nil bridge when tts is enabled")
	}
}

func TestBuildTTSClientBridge_RoutingBridge(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			HTTPBaseURL: "http://127.0.0.1:8765",
			WSURL:       "ws://127.0.0.1:8765/sessions",
			VoiceID:     "female_01",
			SpeechMode:  "conversational",
			VoiceServers: map[string]config.TTSVoiceServerConfig{
				"male_01": {
					HTTPBaseURL: "http://127.0.0.1:8766",
					WSURL:       "ws://127.0.0.1:8766/sessions",
				},
			},
		},
	}
	got := buildTTSClientBridge(cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil bridge when voice_servers configured")
	}
	if _, ok := got.(*ttsinfra.RoutingTTSBridge); !ok {
		t.Fatalf("expected RoutingTTSBridge, got %T", got)
	}
}

func TestBuildTTSClientBridge_SBV2DirectWithoutPlaybackCommands(t *testing.T) {
	cfg := &config.Config{
		TTS: config.TTSConfig{
			Enabled:     true,
			OutputDir:   t.TempDir(),
			HTTPBaseURL: "http://127.0.0.1:8765",
			WSURL:       "ws://127.0.0.1:8765/sessions",
			VoiceID:     "female_01",
			SpeechMode:  "conversational",
			SBV2: config.TTSSBV2Config{
				Enabled: true,
				BaseURL: "http://127.0.0.1:8000/api/synthesis",
				VoiceID: "mio",
			},
		},
	}

	got := buildTTSClientBridge(cfg, nil)
	if got == nil {
		t.Fatal("expected non-nil bridge for sbv2 direct browser-only mode")
	}
	if _, ok := got.(*ttsinfra.SBV2DirectBridge); !ok {
		t.Fatalf("expected sbv2 direct bridge, got %T", got)
	}
}
