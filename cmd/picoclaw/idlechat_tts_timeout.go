package main

import (
	"log"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
)

func markIdleChatTTSTimeout(ev idlechat.TTSTimeoutEvent) {
	sessionID := strings.TrimSpace(ev.SessionID)
	if sessionID == "" {
		return
	}
	kind := strings.TrimSpace(ev.Kind)
	allForSession := kind == "session_audio_timeout"
	if !allForSession {
		log.Printf("[IdleChat] keeping pending TTS utterance for late ACK: session=%s message_id=%s turn_index=%d", sessionID, strings.TrimSpace(ev.MessageID), ev.TurnIndex)
		return
	}
	matched := markTTSPublicSessionTimedOut(sessionID, ev.MessageID, ev.TurnIndex, allForSession)
	for _, internalSessionID := range matched {
		clearIdleChatTTSPending(internalSessionID)
	}
	log.Printf("[IdleChat] marked pending TTS session audio timeout: session=%s matched=%d remaining_index=%d/%d", sessionID, len(matched), ev.RemainingIndex, ev.RemainingCount)
}
