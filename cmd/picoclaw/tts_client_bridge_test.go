package main

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
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
			PlaybackCommands: []config.TTSCommandConfig{
				{Name: "sh", Args: []string{"-c", "true"}},
			},
		},
	}
	if got := buildTTSClientBridge(cfg, nil); got == nil {
		t.Fatal("expected non-nil bridge when tts is enabled")
	}
}
