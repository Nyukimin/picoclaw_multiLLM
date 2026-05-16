package tts

import (
	"strings"
)

func withIrodoriDefaults(cfg IrodoriConfig) IrodoriConfig {
	if cfg.EndpointPath == "" {
		cfg.EndpointPath = "/api/tts"
	}
	if cfg.Checkpoint == "" {
		cfg.Checkpoint = "Aratako/Irodori-TTS-500M-v2"
	}
	if cfg.ModelDevice == "" {
		cfg.ModelDevice = "mps"
	}
	if cfg.ModelPrecision == "" {
		cfg.ModelPrecision = "fp32"
	}
	if cfg.CodecDevice == "" {
		cfg.CodecDevice = "mps"
	}
	if cfg.CodecPrecision == "" {
		cfg.CodecPrecision = "fp32"
	}
	if cfg.NumSteps <= 0 {
		cfg.NumSteps = 16
	}
	if cfg.NumCandidates <= 0 {
		cfg.NumCandidates = 1
	}
	if cfg.CFGGuidanceMode == "" {
		cfg.CFGGuidanceMode = "independent"
	}
	if cfg.CFGScaleText == 0 {
		cfg.CFGScaleText = 3.0
	}
	if cfg.CFGScaleSpeaker == 0 {
		cfg.CFGScaleSpeaker = 5.0
	}
	if cfg.CFGMinT == 0 {
		cfg.CFGMinT = 0.5
	}
	if cfg.CFGMaxT == 0 {
		cfg.CFGMaxT = 1.0
	}
	if !cfg.ContextKVCache {
		cfg.ContextKVCache = true
	}
	return cfg
}

func resolveIrodoriVoiceID(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shiro", "male", "male_01", "shi-gozaki", "shigozaki":
		return "shiro"
	default:
		return "mio"
	}
}

func resolveIrodoriVoiceName(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shiro", "male", "male_01", "shi-gozaki", "shigozaki":
		return "Shiro"
	case "mio", "female", "female_01", "female_01_mio":
		return "Mio"
	default:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return "Mio"
		}
		return trimmed
	}
}

func resolveIrodoriStyle(emotion EmotionState) string {
	switch strings.ToLower(strings.TrimSpace(emotion.Emotion)) {
	case "alert", "warning", "urgent":
		return "urgent"
	case "serious", "report":
		return "firm"
	case "cheerful", "happy":
		return "bright"
	case "warm", "soft":
		return "soft"
	case "flat":
		return "flat"
	case "calm":
		return "calm"
	}
	if emotion.Intensity == 0 && emotion.Expressiveness == 0 && emotion.Pitch == 0 && emotion.Speed == 0 && strings.TrimSpace(emotion.Pause) == "" {
		return "neutral"
	}
	if emotion.Intensity >= 0.75 {
		return "urgent"
	}
	if emotion.Expressiveness >= 0.65 || emotion.Pitch >= 0.58 {
		return "bright"
	}
	if (emotion.Speed > 0 && emotion.Speed <= 0.42) || emotion.Pause == "long" {
		return "calm"
	}
	return "neutral"
}
