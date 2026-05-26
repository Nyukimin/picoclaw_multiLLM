package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestViewerActiveControl_LastClaimWinsPerKind(t *testing.T) {
	resetActiveViewerControlForTest()

	first := activeViewerControl.claim("audio", "pc-viewer")
	if first.ActiveAudioViewerID != "pc-viewer" {
		t.Fatalf("expected first audio viewer, got %q", first.ActiveAudioViewerID)
	}
	second := activeViewerControl.claim("audio", "phone-viewer")
	if second.ActiveAudioViewerID != "phone-viewer" {
		t.Fatalf("expected later audio viewer to win, got %q", second.ActiveAudioViewerID)
	}
	input := activeViewerControl.claim("input", "pc-viewer")
	if input.ActiveAudioViewerID != "phone-viewer" || input.ActiveInputViewerID != "pc-viewer" {
		t.Fatalf("audio and input active IDs should be independent, got %#v", input)
	}
}

func TestTTSPlaybackAckOnlyReleasesActiveAudioViewer(t *testing.T) {
	resetActiveViewerControlForTest()
	ch := registerIdleChatTTSPending("idle-active-tts", "response-active-1")
	activeViewerControl.claim("audio", "pc-viewer")

	reqBody, _ := json.Marshal(ttsPlaybackAckRequest{
		ResponseID:     "response-active-1",
		SessionID:      "idle-active-tts",
		ViewerClientID: "phone-viewer",
		Status:         "ended",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/tts/playback-ack", bytes.NewReader(reqBody))
	handleTTSPlaybackAck()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("inactive ack should be accepted as an observation, got HTTP %d", rec.Code)
	}
	select {
	case <-ch:
		t.Fatal("inactive viewer ack must not release idlechat TTS pending")
	default:
	}

	reqBody, _ = json.Marshal(ttsPlaybackAckRequest{
		ResponseID:     "response-active-1",
		SessionID:      "idle-active-tts",
		ViewerClientID: "pc-viewer",
		Status:         "ended",
	})
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/viewer/tts/playback-ack", bytes.NewReader(reqBody))
	handleTTSPlaybackAck()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("active ack got HTTP %d", rec.Code)
	}
	select {
	case <-ch:
	default:
		t.Fatal("active viewer ack should release idlechat TTS pending")
	}
}

func TestTTSPlaybackFallbackAckReleasesWhenNoActiveAudioViewer(t *testing.T) {
	resetActiveViewerControlForTest()
	ch := registerIdleChatTTSPending("idle-fallback-tts", "response-fallback-1")

	reqBody, _ := json.Marshal(ttsPlaybackAckRequest{
		ResponseID:     "response-fallback-1",
		SessionID:      "idle-fallback-tts",
		ViewerClientID: "pc-viewer",
		Status:         "fallback",
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/tts/playback-ack", bytes.NewReader(reqBody))
	handleTTSPlaybackAck()(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fallback ack got HTTP %d", rec.Code)
	}
	select {
	case <-ch:
	default:
		t.Fatal("fallback ack with viewer_client_id should release idlechat TTS pending when no active audio viewer exists")
	}
}

func TestViewerActiveClaimHandlerBroadcastsControlEvent(t *testing.T) {
	resetActiveViewerControlForTest()
	var emitted []orchestrator.OrchestratorEvent
	body := bytes.NewBufferString(`{"viewer_client_id":"phone-viewer","kind":"input","reason":"stt_start"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/active-control", body)

	handleViewerActiveClaim(func(ev orchestrator.OrchestratorEvent) {
		emitted = append(emitted, ev)
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("claim got HTTP %d: %s", rec.Code, rec.Body.String())
	}
	if got := activeViewerControl.snapshot().ActiveInputViewerID; got != "phone-viewer" {
		t.Fatalf("expected active input viewer, got %q", got)
	}
	if len(emitted) != 1 || emitted[0].Type != "viewer.active_control" {
		t.Fatalf("expected viewer.active_control event, got %#v", emitted)
	}
}
