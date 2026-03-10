package telegram

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

type mockOrchestrator struct{}

func (m *mockOrchestrator) ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error) {
	return orchestrator.ProcessMessageResponse{Response: "ok", Route: routing.RouteCHAT, JobID: "job1"}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestAdapter_SendAndProbe(t *testing.T) {
	adapter := NewAdapter("token")
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	if err := adapter.Probe(context.Background()); err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if err := adapter.Send(context.Background(), "123", "hello"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestAdapter_ServeHTTP(t *testing.T) {
	adapter := NewAdapter("token", &mockOrchestrator{})
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	body := []byte(`{"update_id":1,"message":{"message_id":10,"text":"hi","chat":{"id":123,"type":"private"},"from":{"id":456}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_ServeHTTP_InvalidSecret(t *testing.T) {
	adapter := NewAdapter("token", &mockOrchestrator{})
	adapter.SetWebhookSecret("secret")
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	body := []byte(`{"update_id":1,"message":{"message_id":10,"text":"hi","chat":{"id":123,"type":"private"},"from":{"id":456}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram", bytes.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong")
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
