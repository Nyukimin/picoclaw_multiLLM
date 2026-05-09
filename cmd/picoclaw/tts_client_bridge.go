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
	cmds := buildTTSCommandSpecs(cfg)

	sink := ttsinfra.AudioSink(ttsinfra.NewNoopAudioSink())
	if len(cmds) == 0 {
		log.Printf("TTS browser-only mode enabled (local playback disabled)")
	} else {
		player := ttsinfra.NewCommandPlayer(cmds)
		sink = ttsinfra.NewPlaybackAudioSink(player, cfg.TTS.AudioPathRoot)
	}
	onChunkFn := func(sessionID, responseID string, chunkIndex int, characterID, text, displayText, audioPath, audioURL string) {
		displayText = strings.TrimSpace(displayText)
		if displayText == "" {
			displayText = text
		}
		if onChunkReady != nil {
			onChunkReady(sessionID, characterID, displayText)
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
			"display_text": displayText,
			"audio_path":   audioPath,
			"audio_url":    audioURL,
		})
		if err != nil {
			log.Printf("WARN: tts chunk payload marshal failed: %v", err)
			return
		}
		channel := "viewer"
		chatID := "viewer-user"
		if strings.HasPrefix(strings.TrimSpace(sessionID), "idle-") {
			channel = "idlechat"
			chatID = strings.TrimSpace(sessionID)
		}
		onChunk(orchestrator.NewEvent("tts.audio_chunk", "tts", "user", string(payload), "TTS", "", sessionID, channel, chatID))
	}
	onSessionDoneFn := func(sessionID, characterID string) {
		if onChunk != nil {
			payload, err := json.Marshal(map[string]any{
				"session_id":   strings.TrimSpace(sessionID),
				"character_id": strings.TrimSpace(characterID),
			})
			if err != nil {
				log.Printf("WARN: tts session completed payload marshal failed: %v", err)
			} else {
				onChunk(orchestrator.NewEvent("tts.session_completed", "tts", "user", string(payload), "TTS", "", sessionID, "viewer", "viewer-user"))
			}
		}
		notifyIdleChatTTSCompleted(sessionID)
		if onSessionCompleted != nil {
			onSessionCompleted(sessionID, characterID)
		}
	}
	if sel, ok := buildPrimaryTTSProvider(cfg); ok {
		logTTSProviderSelection(sel)
		return ttsinfra.NewSBV2TTSBridge(ttsinfra.SBV2TTSBridgeConfig{
			Provider:           sel.Provider,
			Sink:               sink,
			OutputDir:          cfg.TTS.OutputDir,
			OnChunkReady:       onChunkFn,
			OnSessionCompleted: onSessionDoneFn,
		})
	}
	bridge := ttsinfra.NewRenCrowTTSBridge(ttsinfra.RenCrowTTSBridgeConfig{
		HTTPBaseURL:        cfg.TTS.HTTPBaseURL,
		VoiceID:            cfg.TTS.VoiceID,
		TLSSkipVerify:      cfg.TTS.TLSSkipVerify,
		RequestTimeout:     time.Duration(cfg.TTS.TimeoutMS) * time.Millisecond,
		ProviderParams:     cfg.TTS.ProviderParams,
		Sink:               sink,
		OnChunkReady:       onChunkFn,
		OnSessionCompleted: onSessionDoneFn,
	})
	log.Printf("TTS RenCrow bridge enabled (/synthesis base=%s)", cfg.TTS.HTTPBaseURL)
	return bridge
}
