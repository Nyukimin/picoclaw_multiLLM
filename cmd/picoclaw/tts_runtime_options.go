package main

import (
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

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
