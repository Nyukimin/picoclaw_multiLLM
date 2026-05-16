package main

import (
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func buildIrodoriTTSProvider(cfg *config.Config, includeUnavailable bool) (ttsProviderSelection, bool) {
	if cfg != nil && cfg.TTS.Irodori.Enabled && strings.TrimSpace(cfg.TTS.Irodori.BaseURL) != "" {
		provider := ttsinfra.NewIrodoriProvider(ttsinfra.IrodoriConfig{
			BaseURL:               cfg.TTS.Irodori.BaseURL,
			EndpointPath:          cfg.TTS.Irodori.EndpointPath,
			VoiceID:               cfg.TTS.Irodori.VoiceID,
			VoiceName:             cfg.TTS.Irodori.VoiceName,
			ReferenceAudio:        cfg.TTS.Irodori.ReferenceAudio,
			ReferenceAudioURL:     cfg.TTS.Irodori.ReferenceAudioURL,
			Timeout:               time.Duration(cfg.TTS.Irodori.TimeoutSec) * time.Second,
			Checkpoint:            cfg.TTS.Irodori.Checkpoint,
			ModelDevice:           cfg.TTS.Irodori.ModelDevice,
			ModelPrecision:        cfg.TTS.Irodori.ModelPrecision,
			CodecDevice:           cfg.TTS.Irodori.CodecDevice,
			CodecPrecision:        cfg.TTS.Irodori.CodecPrecision,
			EnableWatermark:       cfg.TTS.Irodori.EnableWatermark,
			NumSteps:              cfg.TTS.Irodori.NumSteps,
			NumCandidates:         cfg.TTS.Irodori.NumCandidates,
			SeedRaw:               cfg.TTS.Irodori.SeedRaw,
			CFGGuidanceMode:       cfg.TTS.Irodori.CFGGuidanceMode,
			CFGScaleText:          cfg.TTS.Irodori.CFGScaleText,
			CFGScaleSpeaker:       cfg.TTS.Irodori.CFGScaleSpeaker,
			CFGScaleRaw:           cfg.TTS.Irodori.CFGScaleRaw,
			CFGMinT:               cfg.TTS.Irodori.CFGMinT,
			CFGMaxT:               cfg.TTS.Irodori.CFGMaxT,
			ContextKVCache:        cfg.TTS.Irodori.ContextKVCache,
			TruncationFactorRaw:   cfg.TTS.Irodori.TruncationFactorRaw,
			RescaleKRaw:           cfg.TTS.Irodori.RescaleKRaw,
			RescaleSigmaRaw:       cfg.TTS.Irodori.RescaleSigmaRaw,
			SpeakerKVScaleRaw:     cfg.TTS.Irodori.SpeakerKVScaleRaw,
			SpeakerKVMinTRaw:      cfg.TTS.Irodori.SpeakerKVMinTRaw,
			SpeakerKVMaxLayersRaw: cfg.TTS.Irodori.SpeakerKVMaxLayersRaw,
		})
		return ttsProviderSelection{
			Provider: provider,
			Name:     "irodori",
			BaseURL:  cfg.TTS.Irodori.BaseURL,
			Endpoint: cfg.TTS.Irodori.EndpointPath,
		}, true
	}
	if includeUnavailable {
		return ttsProviderSelection{
			Provider: ttsinfra.NewUnavailableProvider("irodori", "irodori is not configured"),
			Name:     "irodori",
		}, true
	}
	return ttsProviderSelection{}, false
}
