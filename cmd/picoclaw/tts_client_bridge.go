package main

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tts"
)

func buildTTSClientBridge(cfg *config.Config, onChunk func(ev orchestrator.OrchestratorEvent)) orchestrator.TTSBridge {
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

	if cfg.TTS.SBV2.Enabled && strings.Contains(strings.TrimRight(strings.ToLower(cfg.TTS.SBV2.BaseURL), "/"), "/api/synthesis") {
		voiceID := strings.TrimSpace(cfg.TTS.SBV2.VoiceID)
		if voiceID == "" {
			voiceID = strings.TrimSpace(cfg.TTS.VoiceID)
		}
		bridge := ttsinfra.NewSBV2DirectBridge(ttsinfra.SBV2DirectBridgeConfig{
			Provider: ttsinfra.NewSBV2Provider(ttsinfra.SBV2Config{
				BaseURL:       cfg.TTS.SBV2.BaseURL,
				VoiceID:       cfg.TTS.SBV2.VoiceID,
				Timeout:       time.Duration(cfg.TTS.SBV2.TimeoutSec) * time.Second,
				AudioPathRoot: cfg.TTS.AudioPathRoot,
			}),
			Sink:      sink,
			OutputDir: cfg.TTS.OutputDir,
			VoiceID:   voiceID,
			OnChunkReady: func(sessionID string, chunkIndex int, characterID, text, audioPath, audioURL string) {
				if onChunk == nil {
					return
				}
				browserAudioURL := strings.TrimSpace(audioURL)
				if browserAudioURL == "" {
					browserAudioURL = buildLocalTTSAudioURL(cfg.TTS.OutputDir, audioPath)
				}
				payload, err := json.Marshal(map[string]any{
					"session_id":   sessionID,
					"chunk_index":  chunkIndex,
					"character_id": characterID,
					"text":         text,
					"audio_path":   audioPath,
					"audio_url":    browserAudioURL,
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
			OnSessionCompleted: func(sessionID string) {
				notifyIdleChatTTSCompleted(sessionID)
			},
		})
		log.Printf("TTS direct SBV2 bridge enabled (sbv2=%s)", cfg.TTS.SBV2.BaseURL)
		return bridge
	}

	// Build the default ClientBridge (Server A).
	defaultBridge := buildClientBridgeForServer(
		cfg.TTS.HTTPBaseURL, cfg.TTS.WSURL, cfg.TTS.VoiceID,
		cfg, sink, onChunk,
	)

	// Build per-voice bridges (e.g. Server B for male_01).
	if len(cfg.TTS.VoiceServers) == 0 {
		log.Printf("TTS client bridge enabled (http=%s ws=%s)", cfg.TTS.HTTPBaseURL, cfg.TTS.WSURL)
		return defaultBridge
	}

	voiceBridges := make(map[string]orchestrator.TTSBridge, len(cfg.TTS.VoiceServers))
	for voiceID, serverCfg := range cfg.TTS.VoiceServers {
		voiceBridges[voiceID] = buildClientBridgeForServer(
			serverCfg.HTTPBaseURL, serverCfg.WSURL, voiceID,
			cfg, sink, onChunk,
		)
	}
	log.Printf("TTS routing bridge enabled (default=%s voices=%v)", cfg.TTS.HTTPBaseURL, cfg.TTS.VoiceServers)
	return ttsinfra.NewRoutingTTSBridge(defaultBridge, voiceBridges)
}

func buildClientBridgeForServer(httpBase, wsURL, voiceID string, cfg *config.Config, sink ttsinfra.AudioSink, onChunk func(ev orchestrator.OrchestratorEvent)) *ttsinfra.ClientBridge {
	return ttsinfra.NewClientBridge(ttsinfra.ClientConfig{
		HTTPBaseURL:     httpBase,
		WSURL:           wsURL,
		VoiceID:         voiceID,
		SpeechMode:      cfg.TTS.SpeechMode,
		ConnectTimeout:  time.Duration(cfg.TTS.ConnectTimeoutMS) * time.Millisecond,
		ReceiveTimeout:  time.Duration(cfg.TTS.ReceiveTimeoutMS) * time.Millisecond,
		ChunkGapTimeout: time.Duration(cfg.TTS.ChunkGapTimeoutMS) * time.Millisecond,
		OnChunkReady: func(sessionID string, chunkIndex int, characterID, text, audioPath, audioURL string) {
			if onChunk == nil {
				return
			}
			payload, err := json.Marshal(map[string]any{
				"session_id":   sessionID,
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
		OnSessionCompleted: func(sessionID string) {
			notifyIdleChatTTSCompleted(sessionID)
		},
	}, sink)
}
