package tts

import (
	"context"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

// RoutingTTSBridge routes TTS sessions to different servers based on voice_id.
// Each entry in voiceBridges maps an exact voice_id to a dedicated TTSBridge.
// Sessions that don't match any voice_id use defaultBridge.
type RoutingTTSBridge struct {
	defaultBridge orchestrator.TTSBridge
	voiceBridges  map[string]orchestrator.TTSBridge

	mu       sync.RWMutex
	sessions map[string]orchestrator.TTSBridge // sessionID → selected bridge
}

func NewRoutingTTSBridge(defaultBridge orchestrator.TTSBridge, voiceBridges map[string]orchestrator.TTSBridge) *RoutingTTSBridge {
	return &RoutingTTSBridge{
		defaultBridge: defaultBridge,
		voiceBridges:  voiceBridges,
		sessions:      make(map[string]orchestrator.TTSBridge),
	}
}

func (r *RoutingTTSBridge) selectBridge(voiceID string) orchestrator.TTSBridge {
	if b, ok := r.voiceBridges[voiceID]; ok {
		return b
	}
	return r.defaultBridge
}

func (r *RoutingTTSBridge) StartSession(ctx context.Context, req orchestrator.TTSSessionStart) error {
	bridge := r.selectBridge(req.VoiceID)
	if err := bridge.StartSession(ctx, req); err != nil {
		return err
	}
	r.mu.Lock()
	r.sessions[req.SessionID] = bridge
	r.mu.Unlock()
	return nil
}

func (r *RoutingTTSBridge) PushText(ctx context.Context, sessionID string, text string, emotion *ttsapp.EmotionState) error {
	r.mu.RLock()
	bridge, ok := r.sessions[sessionID]
	r.mu.RUnlock()
	if !ok {
		bridge = r.defaultBridge
	}
	return bridge.PushText(ctx, sessionID, text, emotion)
}

func (r *RoutingTTSBridge) EndSession(ctx context.Context, sessionID string) error {
	r.mu.Lock()
	bridge, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
	}
	r.mu.Unlock()
	if !ok {
		return r.defaultBridge.EndSession(ctx, sessionID)
	}
	return bridge.EndSession(ctx, sessionID)
}
