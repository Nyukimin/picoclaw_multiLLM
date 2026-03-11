package orchestrator

import (
	"context"
	"log"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

const (
	defaultTTSVoiceID      = "female_01"
	defaultTTSVoiceProfile = "lumina_female"
	maleTTSVoiceID         = "male_01"
	maleTTSVoiceProfile    = "lumina_male"
	ttsChunkMinRunes       = 6
	ttsChunkMaxRunes       = 72
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

type ttsStreamForwarder struct {
	bridge       TTSBridge
	sessionID    string
	route        routing.Route
	eventType    string
	ttsCtx       ttsapp.EmotionContext
	voiceProfile string
	logPrefix    string
	pending      strings.Builder
	emitted      bool
}

func newTTSStreamForwarder(bridge TTSBridge, sessionID string, route routing.Route, eventType, logPrefix string) *ttsStreamForwarder {
	if bridge == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	return &ttsStreamForwarder{
		bridge:       bridge,
		sessionID:    sessionID,
		route:        route,
		eventType:    eventType,
		ttsCtx:       buildTTSContext(route, "normal", false),
		voiceProfile: voiceProfile,
		logPrefix:    logPrefix,
	}
}

func (f *ttsStreamForwarder) OnToken(ctx context.Context, token string) {
	if f == nil || token == "" {
		return
	}
	f.pending.WriteString(token)
	for {
		chunk, rest, ok := nextTTSChunk(f.pending.String(), false)
		if !ok {
			return
		}
		f.pending.Reset()
		f.pending.WriteString(rest)
		f.emit(ctx, chunk)
	}
}

func (f *ttsStreamForwarder) Finalize(ctx context.Context, finalText string) {
	if f == nil {
		return
	}
	if f.emitted {
		chunk, _, ok := nextTTSChunk(f.pending.String(), true)
		if ok {
			f.pending.Reset()
			f.emit(ctx, chunk)
		}
		return
	}
	f.pending.Reset()
	f.emit(ctx, finalText)
}

func (f *ttsStreamForwarder) emit(ctx context.Context, text string) {
	filtered, emotion := buildTTSPayload(f.eventType, f.route, text, f.ttsCtx, f.voiceProfile)
	if filtered == "" {
		return
	}
	pushTTS(ctx, f.bridge, f.sessionID, filtered, emotion, f.logPrefix)
	f.emitted = true
}

func nextTTSChunk(text string, final bool) (chunk, rest string, ok bool) {
	trimmed := strings.TrimLeftFunc(text, unicode.IsSpace)
	if trimmed == "" {
		return "", "", false
	}

	lastHard := -1
	lastSoft := -1
	lastSpace := -1
	runeCount := 0
	for i, r := range trimmed {
		runeCount++
		end := i + utf8.RuneLen(r)
		switch {
		case isTTSHardBoundary(r):
			lastHard = end
		case isTTSSoftBoundary(r):
			lastSoft = end
		case unicode.IsSpace(r):
			lastSpace = end
		}
		if runeCount >= ttsChunkMaxRunes {
			cut := chooseTTSChunkCut(lastHard, lastSoft, lastSpace)
			if cut > 0 {
				return splitTTSChunk(trimmed, cut)
			}
			if final {
				return splitTTSChunk(trimmed, len(trimmed))
			}
			return "", trimmed, false
		}
	}

	if lastHard > 0 && runeCount >= ttsChunkMinRunes {
		return splitTTSChunk(trimmed, lastHard)
	}
	if final {
		return splitTTSChunk(trimmed, len(trimmed))
	}
	return "", trimmed, false
}

func chooseTTSChunkCut(lastHard, lastSoft, lastSpace int) int {
	switch {
	case lastHard > 0:
		return lastHard
	case lastSoft > 0:
		return lastSoft
	case lastSpace > 0:
		return lastSpace
	default:
		return 0
	}
}

func splitTTSChunk(text string, cut int) (chunk, rest string, ok bool) {
	if cut <= 0 || cut > len(text) {
		return "", text, false
	}
	chunk = strings.TrimSpace(text[:cut])
	rest = strings.TrimLeftFunc(text[cut:], unicode.IsSpace)
	if chunk == "" {
		return "", rest, false
	}
	return chunk, rest, true
}

func isTTSHardBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '.', '!', '?', '\n':
		return true
	default:
		return false
	}
}

func isTTSSoftBoundary(r rune) bool {
	switch r {
	case '、', '，', ',', ';', '；', ':', '：':
		return true
	default:
		return false
	}
}
