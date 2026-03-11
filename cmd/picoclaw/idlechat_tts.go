package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

const (
	idleChatRoute             = "IDLECHAT"
	idleChatDefaultVoiceID    = "female_01"
	idleChatDefaultVoiceProf  = "lumina_female"
	idleChatMaleVoiceID       = "male_01"
	idleChatMaleVoiceProf     = "lumina_male"
)

func emitIdleChatTTS(ctx context.Context, bridge orchestrator.TTSBridge, ev idlechat.TimelineEvent) {
	if bridge == nil || strings.TrimSpace(ev.Content) == "" || ev.Type != "idlechat.message" {
		return
	}

	filtered := ttsapp.FilterSpeakableText("agent.response", idleChatRoute, ev.Content)
	if filtered == "" {
		return
	}

	voiceID, voiceProfile := idleChatVoiceForSpeaker(ev.From)
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

	sessionID := fmt.Sprintf("%s-tts-%d", strings.TrimSpace(ev.SessionID), time.Now().UnixNano())
	if err := bridge.StartSession(ctx, orchestrator.TTSSessionStart{
		SessionID:        sessionID,
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
		log.Printf("[IdleChat] TTS start failed: %v", err)
		return
	}
	if err := bridge.PushText(ctx, sessionID, filtered, &emotion); err != nil {
		log.Printf("[IdleChat] TTS push failed: %v", err)
	}
	if err := bridge.EndSession(ctx, sessionID); err != nil {
		log.Printf("[IdleChat] TTS end failed: %v", err)
	}
}

func emitIdleChatTTSAsync(bridge orchestrator.TTSBridge, ev idlechat.TimelineEvent) {
	if bridge == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		emitIdleChatTTS(ctx, bridge, ev)
	}()
}

func idleChatVoiceForSpeaker(speaker string) (voiceID, voiceProfile string) {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "shiro":
		return idleChatMaleVoiceID, idleChatMaleVoiceProf
	default:
		return idleChatDefaultVoiceID, idleChatDefaultVoiceProf
	}
}

func idleChatTimeOfDay() string {
	hour := time.Now().Hour()
	if hour < 6 || hour >= 21 {
		return "night"
	}
	return "day"
}
