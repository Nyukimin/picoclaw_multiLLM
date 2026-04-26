package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func buildTTSClientBridge(
	cfg *config.Config,
	onChunk func(ev orchestrator.OrchestratorEvent),
	onChunkReady func(sessionID, characterID, text string),
	onSessionCompleted func(sessionID, characterID string),
) orchestrator.TTSBridge {
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

	sink := ttsinfra.AudioSink(ttsinfra.NewNoopAudioSink())
	if len(cmds) == 0 {
		log.Printf("TTS browser-only mode enabled (local playback disabled)")
	} else {
		player := ttsinfra.NewCommandPlayer(cmds)
		sink = ttsinfra.NewPlaybackAudioSink(player, cfg.TTS.AudioPathRoot)
	}
	bridge := ttsinfra.NewRenCrowTTSBridge(ttsinfra.RenCrowTTSBridgeConfig{
		HTTPBaseURL:    cfg.TTS.HTTPBaseURL,
		VoiceID:        cfg.TTS.VoiceID,
		TLSSkipVerify:  cfg.TTS.TLSSkipVerify,
		RequestTimeout: time.Duration(cfg.TTS.TimeoutMS) * time.Millisecond,
		ProviderParams: cfg.TTS.ProviderParams,
		Sink:           sink,
		OnChunkReady: func(sessionID, responseID string, chunkIndex int, characterID, text, audioPath, audioURL string) {
			if onChunkReady != nil {
				onChunkReady(sessionID, characterID, text)
			}
			if onChunk == nil {
				return
			}
			payload, err := json.Marshal(map[string]any{
				"session_id":   sessionID,
				"response_id":  responseID,
				"utterance_id": fmt.Sprintf("%s:%04d", sessionID, chunkIndex),
				"chunk_index":  chunkIndex,
				"character_id": characterID,
				"text":         text,
				"audio_path":   audioPath,
				"audio_url":    audioURL,
			})
			if err != nil {
				log.Printf("WARN: tts chunk payload marshal failed: %v", err)
				return
			}
			onChunk(orchestrator.NewEvent(
				"tts.audio_chunk",
				"tts",
				"user",
				string(payload),
				"TTS",
				"",
				sessionID,
				"viewer",
				"viewer-user",
			))
		},
		OnSessionCompleted: func(sessionID, characterID string) {
			notifyIdleChatTTSCompleted(sessionID)
			if onSessionCompleted != nil {
				onSessionCompleted(sessionID, characterID)
			}
		},
	})
	log.Printf("TTS RenCrow bridge enabled (/synthesis base=%s)", cfg.TTS.HTTPBaseURL)
	return bridge
}
