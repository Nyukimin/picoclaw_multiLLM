package main

import (
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

// This file is the integration boundary for RenCrow_TTS.
// Keep provider selection and config translation here so callers do not know
// about Irodori implementation details.

type ttsProviderSelection struct {
	Provider ttsinfra.Provider
	Name     string
	BaseURL  string
	Endpoint string
}

func buildPrimaryTTSProvider(cfg *config.Config) (ttsProviderSelection, bool) {
	if cfg == nil || !cfg.TTS.Enabled {
		return ttsProviderSelection{}, false
	}
	for _, name := range ttsProviderPriority(cfg) {
		sel, ok := buildTTSProviderByName(cfg, name, false)
		if ok {
			return sel, true
		}
	}
	return ttsProviderSelection{}, false
}

func buildFallbackTTSSynthesizer(cfg *config.Config) *ttsinfra.FallbackSynthesizer {
	providers := buildTTSProviders(cfg, true)
	if len(providers) == 0 {
		return nil
	}
	return ttsinfra.NewFallbackSynthesizer(providers...)
}

func buildTTSProviders(cfg *config.Config, includeUnavailable bool) []ttsinfra.Provider {
	if cfg == nil || !cfg.TTS.Enabled {
		return nil
	}
	priority := ttsProviderPriority(cfg)
	providers := make([]ttsinfra.Provider, 0, len(priority))
	for _, name := range priority {
		sel, ok := buildTTSProviderByName(cfg, name, includeUnavailable)
		if ok {
			providers = append(providers, sel.Provider)
		}
	}
	return providers
}

func ttsProviderPriority(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	if len(cfg.TTS.ProviderPriority) > 0 {
		return cfg.TTS.ProviderPriority
	}
	return []string{"irodori"}
}

func buildTTSProviderByName(cfg *config.Config, name string, includeUnavailable bool) (ttsProviderSelection, bool) {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "irodori":
		return buildIrodoriTTSProvider(cfg, includeUnavailable)
	case "azure", "eleven":
		if includeUnavailable {
			return ttsProviderSelection{
				Provider: ttsinfra.NewUnavailableProvider(normalized, normalized+" provider is not configured yet"),
				Name:     normalized,
			}, true
		}
	}
	return ttsProviderSelection{}, false
}

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

func buildTTSCommandSpecs(cfg *config.Config) []ttsinfra.CommandSpec {
	if cfg == nil {
		return nil
	}
	cmds := make([]ttsinfra.CommandSpec, 0, len(cfg.TTS.PlaybackCommands))
	for _, c := range cfg.TTS.PlaybackCommands {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		cmds = append(cmds, ttsinfra.CommandSpec{Name: c.Name, Args: append([]string{}, c.Args...)})
	}
	return cmds
}

func chooseTTSVoiceID(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.TTS.Irodori.Enabled && strings.TrimSpace(cfg.TTS.Irodori.VoiceID) != "" {
		return cfg.TTS.Irodori.VoiceID
	}
	return cfg.TTS.VoiceID
}

func logTTSProviderSelection(sel ttsProviderSelection) {
	switch sel.Name {
	case "irodori":
		log.Printf("TTS Irodori bridge enabled (base=%s endpoint=%s)", sel.BaseURL, sel.Endpoint)
	}
}
