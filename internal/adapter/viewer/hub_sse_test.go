package viewer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestHandleSSE_UsesLastEventIDForHistoryReplay(t *testing.T) {
	hub := NewEventHub(10)
	hub.OnEvent(orchestrator.NewEvent("entry.stage", "chrome", "system", "received", "CHAT", "j1", "s1", "local", "u1"))
	hub.OnEvent(orchestrator.NewEvent("entry.stage", "chrome", "system", "planning", "CHAT", "j1", "s1", "local", "u1"))

	req := httptest.NewRequest(http.MethodGet, "/viewer/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()

	ctx := req.Context()
	ctx, cancel := context.WithCancel(ctx)
	cancel() // history送信後に終了
	req = req.WithContext(ctx)

	hub.HandleSSE(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, `"seq":1`) {
		t.Fatalf("expected seq=1 to be skipped, got: %s", body)
	}
	if !strings.Contains(body, `"seq":2`) {
		t.Fatalf("expected seq=2 in replay, got: %s", body)
	}
}

func TestHandleAudioRouterSSE_FiltersAndReplaysAudioChunks(t *testing.T) {
	hub := NewEventHub(10)
	hub.OnEvent(orchestrator.NewEvent("entry.stage", "chrome", "system", "received", "CHAT", "j1", "s1", "local", "u1"))
	hub.OnEvent(orchestrator.NewEvent("tts.audio_chunk", "tts", "user", `{"session_id":"s1","chunk_index":0,"character_id":"mio","audio_url":"http://example/audio.wav"}`, "TTS", "", "s1", "viewer", "viewer-user"))

	req := httptest.NewRequest(http.MethodGet, "/audio-router/events", nil)
	req.Header.Set("Last-Event-ID", "0")
	rec := httptest.NewRecorder()

	ctx := req.Context()
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	req = req.WithContext(ctx)

	HandleAudioRouterSSE(hub)(rec, req)
	body := rec.Body.String()
	if strings.Contains(body, `"eventType":"entry.stage"`) || strings.Contains(body, "received") {
		t.Fatalf("unexpected non-audio event in stream: %s", body)
	}
	if !strings.Contains(body, `"character_id":"mio"`) {
		t.Fatalf("expected character_id payload in stream: %s", body)
	}
	if !strings.Contains(body, "event: tts.audio_chunk") {
		t.Fatalf("expected named SSE event, got: %s", body)
	}
}
