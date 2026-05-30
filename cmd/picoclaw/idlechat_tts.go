package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

const idleChatRoute = "IDLECHAT"

func emitIdleChatTTS(ctx context.Context, bridge orchestrator.TTSBridge, ev idlechat.TimelineEvent) (<-chan struct{}, bool) {
	if bridge == nil || strings.TrimSpace(ev.Content) == "" || !isIdleChatTTSEventType(ev.Type) {
		return nil, false
	}

	filtered := ttsapp.FilterSpeakableText("agent.response", idleChatRoute, formatIdleChatTTSText(ev))
	if filtered == "" {
		return nil, false
	}
	displayText := filtered
	if isIdleChatTopicAnnouncement(ev) {
		displayText = formatIdleChatDisplayText(ev)
	}

	voiceID, voiceProfile := idleChatVoiceForSpeaker(ev.From)
	characterID := normalizeIdleChatCharacterID(ev.From)
	emotion := ttsapp.PlanEmotion(ttsapp.EmotionInput{
		Event: "conversation",
		Text:  filtered,
		Context: ttsapp.EmotionContext{
			ConversationMode: "chat",
			TimeOfDay:        idleChatTimeOfDay(),
			Urgency:          "normal",
		},
		VoiceProfile: voiceProfile,
	})

	publicSessionID := strings.TrimSpace(ev.SessionID)
	responseID := nextTTSPublicResponseIDForMessage(publicSessionID, ev.MessageID)
	sessionID := fmt.Sprintf("%s-tts-%d", publicSessionID, time.Now().UnixNano())
	registerTTSPublicSessionWithMessage(sessionID, publicSessionID, responseID, ev.MessageID, ev.TurnIndex)
	waitCh := registerIdleChatTTSPending(sessionID, responseID)
	if err := bridge.StartSession(ctx, orchestrator.TTSSessionStart{
		SessionID:        sessionID,
		ResponseID:       responseID,
		CharacterID:      characterID,
		VoiceID:          voiceID,
		SpeechMode:       "conversational",
		Event:            "conversation",
		ConversationMode: "chat",
		Context: ttsapp.EmotionContext{
			ConversationMode: "chat",
			TimeOfDay:        idleChatTimeOfDay(),
			Urgency:          "normal",
		},
		VoiceProfile: voiceProfile,
	}); err != nil {
		clearIdleChatTTSPending(sessionID)
		log.Printf("[IdleChat] TTS start failed: %v", err)
		return nil, false
	}
	if displayBridge, ok := bridge.(orchestrator.TTSDisplayBridge); ok {
		err := displayBridge.PushTextWithDisplay(ctx, sessionID, filtered, displayText, &emotion)
		if err != nil {
			log.Printf("[IdleChat] TTS push failed: %v", err)
			if endErr := bridge.EndSession(ctx, sessionID); endErr != nil {
				log.Printf("[IdleChat] TTS end after push failure failed: %v", endErr)
			}
			clearIdleChatTTSPending(sessionID)
			return waitCh, true
		}
	} else if err := bridge.PushText(ctx, sessionID, filtered, &emotion); err != nil {
		log.Printf("[IdleChat] TTS push failed: %v", err)
		if endErr := bridge.EndSession(ctx, sessionID); endErr != nil {
			log.Printf("[IdleChat] TTS end after push failure failed: %v", endErr)
		}
		clearIdleChatTTSPending(sessionID)
		return waitCh, true
	}
	if err := bridge.EndSession(ctx, sessionID); err != nil {
		clearIdleChatTTSPending(sessionID)
		log.Printf("[IdleChat] TTS end failed: %v", err)
		return nil, false
	}
	return waitCh, true
}

func isIdleChatTTSEventType(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "idlechat.message", "idlechat.topic", "idlechat.tts":
		return true
	default:
		return false
	}
}
