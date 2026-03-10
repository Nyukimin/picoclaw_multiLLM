package main

import (
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func buildTTSClientBridge(cfg *config.Config) orchestrator.TTSBridge {
	if cfg == nil || !cfg.TTS.Enabled {
		return nil
	}
	cmds := make([]ttsinfra.CommandSpec, 0, len(cfg.TTS.PlaybackCommands))
	for _, c := range cfg.TTS.PlaybackCommands {
		if strings.TrimSpace(c.Name) == "" {
			continue
		}
		cmds = append(cmds, ttsinfra.CommandSpec{Name: c.Name, Args: append([]string{}, c.Args...)})
	}
	if len(cmds) == 0 {
		log.Printf("WARN: TTS client bridge disabled (no playback commands configured)")
		return nil
	}
	player := ttsinfra.NewCommandPlayer(cmds)
	sink := ttsinfra.NewPlaybackAudioSink(player)
	bridge := ttsinfra.NewClientBridge(ttsinfra.ClientConfig{
		HTTPBaseURL:     cfg.TTS.HTTPBaseURL,
		WSURL:           cfg.TTS.WSURL,
		VoiceID:         cfg.TTS.VoiceID,
		SpeechMode:      cfg.TTS.SpeechMode,
		ConnectTimeout:  time.Duration(cfg.TTS.ConnectTimeoutMS) * time.Millisecond,
		ReceiveTimeout:  time.Duration(cfg.TTS.ReceiveTimeoutMS) * time.Millisecond,
		ChunkGapTimeout: time.Duration(cfg.TTS.ChunkGapTimeoutMS) * time.Millisecond,
	}, sink)
	log.Printf("TTS client bridge enabled (http=%s ws=%s)", cfg.TTS.HTTPBaseURL, cfg.TTS.WSURL)
	return bridge
}
