package orchestrator

import (
	"context"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func (o *MessageOrchestrator) startTTSSessionForRoute(ctx context.Context, req ProcessMessageRequest, jobID task.JobID, decision routing.Decision, ttsSessionID string) {
	if o.ttsBridge == nil || ttsSessionID == "" {
		return
	}
	ttsCtx := buildTTSContext(decision.Route, "normal", false)
	voiceID, voiceProfile := voiceForSpeaker(speakerForRoute(decision.Route))
	startReq := TTSSessionStart{
		SessionID:             ttsSessionID,
		ResponseID:            jobID.String(),
		CharacterID:           speakerForRoute(decision.Route),
		VoiceID:               voiceID,
		SpeechMode:            speechModeForRoute(decision.Route),
		Event:                 eventForRoute(decision.Route),
		Urgency:               ttsCtx.Urgency,
		ConversationMode:      ttsCtx.ConversationMode,
		UserAttentionRequired: ttsCtx.UserAttentionRequired,
		Context:               ttsCtx,
		VoiceProfile:          voiceProfile,
	}
	if err := o.ttsBridge.StartSession(ctx, startReq); err != nil {
		log.Printf("[MessageOrch] TTS route update degraded: %v", err)
	}
}

func (o *MessageOrchestrator) endTTSSession(ctx context.Context, ttsSessionID string) {
	if ttsSessionID == "" {
		return
	}
	if err := o.ttsBridge.EndSession(ctx, ttsSessionID); err != nil {
		log.Printf("[MessageOrch] TTS end degraded: %v", err)
	}
}

func (o *MessageOrchestrator) withStreamHooks(
	ctx context.Context,
	route routing.Route,
	jid, sessionID, channel, chatID, ttsSessionID string,
) (context.Context, *streamBundle) {
	prev := llm.StreamCallbackFromContext(ctx)
	ttsStream := newTTSStreamForwarder(o.ttsBridge, ttsSessionID, route, "agent.response", "[MessageOrch] TTS push degraded:")
	vtuberStream := newVTuberStreamForwarder(o.vtuberBridge, ttsSessionID, route, "agent.response", "[MessageOrch] VTuber push degraded:")
	return llm.ContextWithStreamCallback(ctx, func(token string) {
		if prev != nil {
			prev(token)
		}
		o.emit("agent.thinking", "mio", "user", token, string(route), jid, sessionID, channel, chatID)
		ttsStream.OnToken(ctx, token)
		vtuberStream.OnToken(ctx, token)
	}), &streamBundle{tts: ttsStream, vtuber: vtuberStream}
}

func (o *MessageOrchestrator) pushTTS(ctx context.Context, sessionID string, route routing.Route, eventType, text string) {
	ttsCtx := buildTTSContext(route, "normal", false)
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	pushTTSTextChunks(ctx, o.ttsBridge, sessionID, route, eventType, text, ttsCtx, voiceProfile, "[MessageOrch] TTS push degraded:")
	req, ok := buildVTuberRequest(eventType, route, sessionID, text, ttsCtx, voiceProfile)
	if ok {
		pushVTuber(ctx, o.vtuberBridge, req, "[MessageOrch] VTuber push degraded:")
	}
}

func speechModeForRoute(route routing.Route) string {
	switch route {
	case routing.RouteOPS:
		return "report"
	case routing.RoutePLAN, routing.RouteANALYZE, routing.RouteRESEARCH:
		return "report"
	default:
		return "conversational"
	}
}
