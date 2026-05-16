package main

import (
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

// This file is the integration boundary for RenCrow_TTS.
// Keep provider selection and config translation here so callers do not know
// about provider implementation details.

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
