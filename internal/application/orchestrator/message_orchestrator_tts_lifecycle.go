package orchestrator

import (
	"context"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type messageTTSLifecycle struct {
	ttsBridge    TTSBridge
	vtuberBridge VTuberBridge
	emit         messageEventEmitter
}

func newMessageTTSLifecycle(ttsBridge TTSBridge, vtuberBridge VTuberBridge, emit messageEventEmitter) *messageTTSLifecycle {
	return &messageTTSLifecycle{
		ttsBridge:    ttsBridge,
		vtuberBridge: vtuberBridge,
		emit:         emit,
	}
}

func (l *messageTTSLifecycle) SetTTSBridge(ttsBridge TTSBridge) {
	l.ttsBridge = ttsBridge
}

func (l *messageTTSLifecycle) SetVTuberBridge(vtuberBridge VTuberBridge) {
	l.vtuberBridge = vtuberBridge
}

func (l *messageTTSLifecycle) StartSessionForRoute(ctx context.Context, req ProcessMessageRequest, jobID task.JobID, decision routing.Decision, ttsSessionID string) {
	if l.ttsBridge == nil || ttsSessionID == "" {
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
	if err := l.ttsBridge.StartSession(ctx, startReq); err != nil {
		log.Printf("[MessageOrch] TTS route update degraded: %v", err)
	}
}

func (l *messageTTSLifecycle) EndSession(ctx context.Context, ttsSessionID string) {
	if ttsSessionID == "" {
		return
	}
	if err := l.ttsBridge.EndSession(ctx, ttsSessionID); err != nil {
		log.Printf("[MessageOrch] TTS end degraded: %v", err)
	}
}

func (l *messageTTSLifecycle) WithStreamHooks(
	ctx context.Context,
	route routing.Route,
	jid, sessionID, channel, chatID, ttsSessionID string,
) (context.Context, *streamBundle) {
	prev := llm.StreamCallbackFromContext(ctx)
	ttsStream := newTTSStreamForwarder(l.ttsBridge, ttsSessionID, route, "agent.response", "[MessageOrch] TTS push degraded:")
	vtuberStream := newVTuberStreamForwarder(l.vtuberBridge, ttsSessionID, route, "agent.response", "[MessageOrch] VTuber push degraded:")
	return llm.ContextWithStreamCallback(ctx, func(token string) {
		if prev != nil {
			prev(token)
		}
		l.emit("agent.thinking", "mio", "user", token, string(route), jid, sessionID, channel, chatID)
		ttsStream.OnToken(ctx, token)
		vtuberStream.OnToken(ctx, token)
	}), &streamBundle{tts: ttsStream, vtuber: vtuberStream}
}

func (l *messageTTSLifecycle) Push(ctx context.Context, sessionID string, route routing.Route, eventType, text string) {
	ttsCtx := buildTTSContext(route, "normal", false)
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	pushTTSTextChunks(ctx, l.ttsBridge, sessionID, route, eventType, text, ttsCtx, voiceProfile, "[MessageOrch] TTS push degraded:")
	req, ok := buildVTuberRequest(eventType, route, sessionID, text, ttsCtx, voiceProfile)
	if ok {
		pushVTuber(ctx, l.vtuberBridge, req, "[MessageOrch] VTuber push degraded:")
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
