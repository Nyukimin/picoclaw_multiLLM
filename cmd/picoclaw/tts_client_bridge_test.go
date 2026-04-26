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
