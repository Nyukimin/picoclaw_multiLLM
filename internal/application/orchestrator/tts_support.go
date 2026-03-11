package orchestrator

import (
	"context"
	"log"
	"strings"
	"time"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

const (
	defaultTTSVoiceID      = "female_01"
	defaultTTSVoiceProfile = "lumina_female"
	maleTTSVoiceID         = "male_01"
	maleTTSVoiceProfile    = "lumina_male"
)

func buildTTSContext(route routing.Route, urgency string, attention bool) ttsapp.EmotionContext {
	timeOfDay := "day"
	hour := time.Now().Hour()
	if hour < 6 || hour >= 21 {
		timeOfDay = "night"
	}
	return ttsapp.EmotionContext{
		ConversationMode:      conversationModeForRoute(route),
		TimeOfDay:             timeOfDay,
		Urgency:               chooseNonEmpty(urgency, "normal"),
		UserAttentionRequired: attention,
	}
}

func eventForRoute(route routing.Route) string {
	switch route {
	case routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH, routing.RouteOPS:
		return "analysis_report"
	default:
		return "conversation"
	}
}

func conversationModeForRoute(route routing.Route) string {
	switch route {
	case routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH, routing.RouteOPS:
		return "report"
	default:
		return "chat"
	}
}

func buildTTSPayload(eventType string, route routing.Route, text string, ctx ttsapp.EmotionContext, voiceProfile string) (string, *ttsapp.EmotionState) {
	filtered := ttsapp.FilterSpeakableText(eventType, string(route), text)
	if filtered == "" {
		return "", nil
	}
	emotion := ttsapp.PlanEmotion(ttsapp.EmotionInput{
		Event:        eventForRoute(route),
		Text:         filtered,
		Context:      ctx,
		VoiceProfile: chooseNonEmpty(voiceProfile, defaultTTSVoiceProfile),
	})
	return filtered, &emotion
}

func voiceForSpeaker(speaker string) (voiceID, voiceProfile string) {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "shiro":
		return maleTTSVoiceID, maleTTSVoiceProfile
	default:
		return defaultTTSVoiceID, defaultTTSVoiceProfile
	}
}

func speakerForRoute(route routing.Route) string {
	switch route {
	case routing.RouteOPS, routing.RouteCODE, routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3:
		return "shiro"
	default:
		return "mio"
	}
}

func chooseNonEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func pushTTS(ctx context.Context, bridge TTSBridge, sessionID, text string, emotion *ttsapp.EmotionState, prefix string) {
	if bridge == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(text) == "" {
		return
	}
	if err := bridge.PushText(ctx, sessionID, text, emotion); err != nil {
		log.Printf("%s %v", prefix, err)
	}
}
