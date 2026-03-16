package orchestrator

import (
	"context"
	"strings"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

type vtuberStreamForwarder struct {
	bridge       VTuberBridge
	sessionID    string
	route        routing.Route
	eventType    string
	ttsCtx       ttsapp.EmotionContext
	voiceProfile string
	logPrefix    string
	pending      strings.Builder
	emitted      bool
}

func newVTuberStreamForwarder(bridge VTuberBridge, sessionID string, route routing.Route, eventType, logPrefix string) *vtuberStreamForwarder {
	if bridge == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	_, voiceProfile := voiceForSpeaker(speakerForRoute(route))
	return &vtuberStreamForwarder{
		bridge:       bridge,
		sessionID:    sessionID,
		route:        route,
		eventType:    eventType,
		ttsCtx:       buildTTSContext(route, "normal", false),
		voiceProfile: voiceProfile,
		logPrefix:    logPrefix,
	}
}

func (f *vtuberStreamForwarder) OnToken(ctx context.Context, token string) {
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

func (f *vtuberStreamForwarder) Finalize(ctx context.Context, finalText string) {
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

func (f *vtuberStreamForwarder) emit(ctx context.Context, text string) {
	req, ok := buildVTuberRequest(f.eventType, f.route, f.sessionID, text, f.ttsCtx, f.voiceProfile)
	if !ok {
		return
	}
	pushVTuber(ctx, f.bridge, req, f.logPrefix)
	f.emitted = true
}
