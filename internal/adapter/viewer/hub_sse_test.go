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
