package main

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type recordingVoiceDirectHandler struct {
	finalCalls []orchestrator.ProcessVoiceDirectRequest
	tokenCalls int
}

func (h *recordingVoiceDirectHandler) ProcessVoiceDirect(_ context.Context, req orchestrator.ProcessVoiceDirectRequest) (orchestrator.ProcessMessageResponse, error) {
	h.finalCalls = append(h.finalCalls, req)
	return orchestrator.ProcessMessageResponse{
		Response: req.FinalText,
		Route:    routing.RouteCHAT,
		JobID:    task.NewJobID().String(),
	}, nil
}

func (h *recordingVoiceDirectHandler) NotifyVoiceDirectFirstToken(context.Context, orchestrator.ProcessVoiceDirectRequest, task.JobID, time.Time) {
	h.tokenCalls++
}

func TestVoiceChatBridgeTracker_FinalizesVoiceDirectOnLLMFinal(t *testing.T) {
	handler := &recordingVoiceDirectHandler{}
	tracker := newVoiceChatBridgeTracker(handler)

	tracker.observeClientText([]byte(`{"type":"session.start","utterance_id":"utt-1","channel":"viewer","chat_id":"viewer-user","sample_rate":16000,"channels":1,"format":"pcm16le","model":"Chat"}`))
	tracker.observeClientText([]byte(`{"type":"session.commit","utterance_id":"utt-1"}`))
	tracker.observeGatewayText([]byte(`{"type":"llm.delta","utterance_id":"utt-1","seq":1,"text":"お"}`))
	tracker.observeGatewayText([]byte(`{"type":"llm.final","utterance_id":"utt-1","text":"おはよう"}`))

	if len(handler.finalCalls) != 1 {
		t.Fatalf("expected one ProcessVoiceDirect call, got %d", len(handler.finalCalls))
	}
	if handler.finalCalls[0].UtteranceID != "utt-1" || handler.finalCalls[0].FinalText != "おはよう" {
		t.Fatalf("unexpected final call: %+v", handler.finalCalls[0])
	}
	if handler.finalCalls[0].Channel != "viewer" {
		t.Fatalf("expected viewer channel, got %q", handler.finalCalls[0].Channel)
	}
	if handler.tokenCalls != 1 {
		t.Fatalf("expected one first-token notification, got %d", handler.tokenCalls)
	}
}

func TestVoiceChatBridgeTracker_CancelClearsState(t *testing.T) {
	handler := &recordingVoiceDirectHandler{}
	tracker := newVoiceChatBridgeTracker(handler)

	tracker.observeClientText([]byte(`{"type":"session.start","utterance_id":"utt-1","channel":"viewer"}`))
	tracker.observeClientText([]byte(`{"type":"session.cancel","utterance_id":"utt-1"}`))
	tracker.observeGatewayText([]byte(`{"type":"llm.final","utterance_id":"utt-1","text":"ignored"}`))

	if len(handler.finalCalls) != 0 {
		t.Fatalf("cancelled utterance must not finalize: %+v", handler.finalCalls)
	}
}

func TestVoiceChatBridgeTracker_SessionStartUsesViewerDefaults(t *testing.T) {
	tracker := newVoiceChatBridgeTracker(nil)
	tracker.observeClientText([]byte(`{"type":"session.start","utterance_id":"utt-9","viewer_session_id":"viewer-session","channel":"viewer"}`))

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.active.UtteranceID != "utt-9" {
		t.Fatalf("unexpected utterance id: %+v", tracker.active)
	}
	if tracker.active.SessionID != "viewer-session" {
		t.Fatalf("unexpected session id: %+v", tracker.active)
	}
	if tracker.active.Channel != "viewer" {
		t.Fatalf("unexpected channel: %+v", tracker.active)
	}
}
