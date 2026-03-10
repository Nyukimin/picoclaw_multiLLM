package discord

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
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
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}, nil
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
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	body := []byte(`{"channel_id":"c1","author_id":"u1","content":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_ServeHTTP_InteractionPing(t *testing.T) {
	adapter := NewAdapter("token", &mockOrchestrator{})
	body := []byte(`{"type":1}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_ServeHTTP_InteractionCommand(t *testing.T) {
	adapter := NewAdapter("token", &mockOrchestrator{})
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")
	body := []byte(`{"type":2,"channel_id":"c1","member":{"user":{"id":"u1"}},"data":{"name":"ask","options":[{"value":"hello"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_NormalizeRelayPayload(t *testing.T) {
	adapter := NewAdapter("token")
	ev, ok := adapter.NormalizeRelayPayload(RelayPayload{
		ChannelID: "c1",
		AuthorID:  "u1",
		Content:   "hello",
	}, []byte(`{}`))
	if !ok {
		t.Fatal("expected relay payload to normalize")
	}
	if ev.Channel != "discord" || ev.ChatID != "c1" || ev.UserID != "u1" || ev.Text != "hello" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestAdapter_NormalizeInteraction(t *testing.T) {
	adapter := NewAdapter("token")
	ev, ok := adapter.NormalizeInteraction(Interaction{
		Type:      2,
		ChannelID: "c1",
		Member:    &InteractionMember{User: &InteractionUser{ID: "u1"}},
		Data:      &InteractionCommandData{Name: "ask", Options: []InteractionOption{{Value: "hi"}}},
	}, []byte(`{}`))
	if !ok {
		t.Fatal("expected interaction to normalize")
	}
	if ev.ChatID != "c1" || ev.UserID != "u1" || ev.Text != "/ask hi" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestAdapter_ServeHTTP_InvalidSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	adapter := NewAdapter("token", &mockOrchestrator{})
	adapter.SetPublicKeyHex(hex.EncodeToString(pub))
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	body := []byte(`{"channel_id":"c1","author_id":"u1","content":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/discord", bytes.NewReader(body))
	req.Header.Set("X-Signature-Ed25519", "invalid")
	req.Header.Set("X-Signature-Timestamp", "123")
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
