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
		publicSessionID, publicChunkIndex := resolveTTSPublicChunk(sessionID, chunkIndex)
		displayText = strings.TrimSpace(displayText)
		if displayText == "" {
			displayText = text
		}
		if onChunkReady != nil {
			onChunkReady(publicSessionID, characterID, displayText)
		}
		if onChunk == nil {
			return
		}
		payload, err := json.Marshal(map[string]any{
			"session_id":   publicSessionID,
			"response_id":  responseID,
			"utterance_id": fmt.Sprintf("%s:%04d", publicSessionID, publicChunkIndex),
			"chunk_index":  publicChunkIndex,
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
		if isIdleChatPublicSession(publicSessionID) {
			channel = "idlechat"
			chatID = strings.TrimSpace(publicSessionID)
		}
		onChunk(orchestrator.NewEvent("tts.audio_chunk", "tts", "user", string(payload), "TTS", "", publicSessionID, channel, chatID))
	}
	onSessionDoneFn := func(sessionID, characterID string) {
		publicSessionID := resolveTTSPublicSession(sessionID)
		responseID := resolveTTSPublicResponse(sessionID)
		if onChunk != nil {
			payload, err := json.Marshal(map[string]any{
				"session_id":   strings.TrimSpace(publicSessionID),
				"response_id":  strings.TrimSpace(responseID),
				"character_id": strings.TrimSpace(characterID),
			})
			if err != nil {
				log.Printf("WARN: tts session completed payload marshal failed: %v", err)
			} else {
				channel := "viewer"
				chatID := "viewer-user"
				if isIdleChatPublicSession(publicSessionID) {
					channel = "idlechat"
					chatID = strings.TrimSpace(publicSessionID)
				}
				onChunk(orchestrator.NewEvent("tts.session_completed", "tts", "user", string(payload), "TTS", "", publicSessionID, channel, chatID))
			}
		}
		clearTTSPublicSession(sessionID)
		if onSessionCompleted != nil {
			onSessionCompleted(publicSessionID, characterID)
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
