package main

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	modulevoicechat "github.com/Nyukimin/picoclaw_multiLLM/modules/voicechat"
)

type voiceDirectFinalHandler interface {
	ProcessVoiceDirect(ctx context.Context, req orchestrator.ProcessVoiceDirectRequest) (orchestrator.ProcessMessageResponse, error)
	NotifyVoiceDirectFirstToken(ctx context.Context, req orchestrator.ProcessVoiceDirectRequest, jobID task.JobID, firstTokenAt time.Time)
}

// voiceDirectBridgeAdapter は orchestrator 連携を bridge から切り離すための薄い adapter。
type voiceDirectBridgeAdapter struct {
	handler voiceDirectFinalHandler
}

func (a voiceDirectBridgeAdapter) ProcessVoiceDirect(ctx context.Context, req orchestrator.ProcessVoiceDirectRequest) (orchestrator.ProcessMessageResponse, error) {
	if a.handler == nil {
		return orchestrator.ProcessMessageResponse{}, nil
	}
	return a.handler.ProcessVoiceDirect(ctx, req)
}

func (a voiceDirectBridgeAdapter) NotifyVoiceDirectFirstToken(ctx context.Context, req orchestrator.ProcessVoiceDirectRequest, jobID task.JobID, firstTokenAt time.Time) {
	if a.handler == nil {
		return
	}
	a.handler.NotifyVoiceDirectFirstToken(ctx, req, jobID, firstTokenAt)
}

type voiceChatBridgeTracker struct {
	mu             sync.Mutex
	active         orchestrator.ProcessVoiceDirectRequest
	jobID          task.JobID
	startedAt      time.Time
	firstTokenSent bool
	handler        voiceDirectBridgeAdapter
}

func newVoiceChatBridgeTracker(handler voiceDirectFinalHandler) *voiceChatBridgeTracker {
	return &voiceChatBridgeTracker{handler: voiceDirectBridgeAdapter{handler: handler}}
}

func (t *voiceChatBridgeTracker) observeClientText(payload []byte) {
	if t == nil || len(payload) == 0 {
		return
	}
	var ev map[string]any
	if err := json.Unmarshal(payload, &ev); err != nil {
		return
	}
	eventType, _ := ev["type"].(string)
	switch eventType {
	case modulevoicechat.EventSessionStart:
		t.beginUtterance(ev)
	case modulevoicechat.EventSessionCommit:
		t.markCommit(ev)
	case modulevoicechat.EventSessionCancel:
		t.reset()
	}
}

func (t *voiceChatBridgeTracker) observeGatewayText(payload []byte) {
	if t == nil || len(payload) == 0 {
		return
	}
	var ev map[string]any
	if err := json.Unmarshal(payload, &ev); err != nil {
		return
	}
	switch ev["type"] {
	case modulevoicechat.EventLLMDelta:
		t.onLLMDelta()
	case modulevoicechat.EventLLMFinal:
		t.onLLMFinal(ev)
	case modulevoicechat.EventError:
		t.reset()
	}
}

func (t *voiceChatBridgeTracker) beginUtterance(ev map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startedAt = time.Now()
	t.jobID = task.NewJobID()
	t.firstTokenSent = false
	t.active = orchestrator.ProcessVoiceDirectRequest{
		UtteranceID:   stringField(ev, "utterance_id"),
		SessionID:     voiceChatFirstNonEmpty(stringField(ev, "viewer_session_id"), stringField(ev, "session_id")),
		Channel:       voiceChatFirstNonEmpty(stringField(ev, "channel"), "viewer"),
		ChatID:        stringField(ev, "chat_id"),
		ViewerSession: stringField(ev, "viewer_session_id"),
		Prompt:        stringField(ev, "prompt"),
		SampleRate:    intField(ev, "sample_rate"),
		Channels:      intField(ev, "channels"),
		StartedAt:     t.startedAt,
	}
}

func (t *voiceChatBridgeTracker) markCommit(ev map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if utteranceID := stringField(ev, "utterance_id"); utteranceID != "" {
		t.active.UtteranceID = utteranceID
	}
	if t.active.StartedAt.IsZero() {
		t.active.StartedAt = time.Now()
	}
}

func (t *voiceChatBridgeTracker) onLLMDelta() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.firstTokenSent || t.handler.handler == nil {
		return
	}
	t.firstTokenSent = true
	req := t.active
	jobID := t.jobID
	if jobID.IsZero() {
		jobID = task.NewJobID()
		t.jobID = jobID
	}
	t.mu.Unlock()
	t.handler.NotifyVoiceDirectFirstToken(context.Background(), req, jobID, time.Now())
	t.mu.Lock()
}

func (t *voiceChatBridgeTracker) onLLMFinal(ev map[string]any) {
	if t == nil {
		return
	}
	text := strings.TrimSpace(stringField(ev, "text"))
	if text == "" {
		t.reset()
		return
	}
	t.mu.Lock()
	req := t.active
	if strings.TrimSpace(req.UtteranceID) == "" {
		t.mu.Unlock()
		return
	}
	req.FinalText = text
	if utteranceID := stringField(ev, "utterance_id"); utteranceID != "" {
		req.UtteranceID = utteranceID
	}
	if req.StartedAt.IsZero() {
		req.StartedAt = t.startedAt
	}
	t.mu.Unlock()

	if t.handler.handler != nil {
		if _, err := t.handler.ProcessVoiceDirect(context.Background(), req); err != nil {
			log.Printf("[voice-chat] ProcessVoiceDirect failed utterance_id=%s: %v", req.UtteranceID, err)
		}
	}
	t.reset()
}

func (t *voiceChatBridgeTracker) reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.active = orchestrator.ProcessVoiceDirectRequest{}
	t.jobID = task.JobID{}
	t.startedAt = time.Time{}
	t.firstTokenSent = false
}

func stringField(ev map[string]any, key string) string {
	if ev == nil {
		return ""
	}
	value, _ := ev[key].(string)
	return strings.TrimSpace(value)
}

func intField(ev map[string]any, key string) int {
	if ev == nil {
		return 0
	}
	switch v := ev[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

func voiceChatFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
