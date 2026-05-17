package slack

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	appattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/application/attachment"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	domainattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

type mockOrchestrator struct{}

func (m *mockOrchestrator) ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error) {
	return orchestrator.ProcessMessageResponse{Response: "ok", Route: routing.RouteCHAT, JobID: "job1"}, nil
}

type captureOrchestrator struct {
	req orchestrator.ProcessMessageRequest
}

func (m *captureOrchestrator) ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error) {
	m.req = req
	return orchestrator.ProcessMessageResponse{Response: "ok", Route: routing.RouteCHAT, JobID: "job1"}, nil
}

type fakeAttachmentSaver struct{}

func (s fakeAttachmentSaver) SaveAll(ctx context.Context, files []appattachment.IncomingFile) ([]domainattachment.Attachment, error) {
	out := make([]domainattachment.Attachment, 0, len(files))
	for _, file := range files {
		data, err := io.ReadAll(file.Reader)
		if err != nil {
			return nil, err
		}
		out = append(out, domainattachment.Attachment{
			Filename:      file.Filename,
			ContentType:   file.ContentType,
			SizeBytes:     int64(len(data)),
			ExtractedText: string(data),
		})
	}
	return out, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newHTTPClient(fn roundTripFunc) *http.Client {
	return &http.Client{Transport: fn}
}

func TestAdapter_SendAndProbe(t *testing.T) {
	adapter := NewAdapter("xoxb-token", "")
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	if err := adapter.Probe(context.Background()); err != nil {
		t.Fatalf("Probe failed: %v", err)
	}
	if err := adapter.Send(context.Background(), "C1", "hello"); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

func TestAdapter_ServeHTTP_URLVerification(t *testing.T) {
	adapter := NewAdapter("xoxb-token", "", &mockOrchestrator{})
	body := []byte(`{"type":"url_verification","challenge":"abc"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/slack", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_ServeHTTP_MessageEvent(t *testing.T) {
	adapter := NewAdapter("xoxb-token", "", &mockOrchestrator{})
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://example.invalid")

	body := []byte(`{"type":"event_callback","event":{"type":"message","text":"hi","user":"U1","channel":"C1"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/slack", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAdapter_ServeHTTP_FileEventUsesAttachmentPipeline(t *testing.T) {
	orch := &captureOrchestrator{}
	adapter := NewAdapter("xoxb-token", "", orch)
	adapter.SetAttachmentSaver(fakeAttachmentSaver{})
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "files.invalid" {
			if got := req.Header.Get("Authorization"); got != "Bearer xoxb-token" {
				t.Fatalf("missing slack file authorization: %q", got)
			}
			h := make(http.Header)
			h.Set("Content-Type", "text/plain")
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("slack attachment text")), Header: h}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
	}))
	adapter.SetAPIBaseURL("https://slack.invalid")

	body := []byte(`{"type":"event_callback","event":{"type":"message","subtype":"file_share","text":"","user":"U1","channel":"C1","files":[{"id":"F1","name":"memo.txt","mimetype":"text/plain","url_private_download":"https://files.invalid/memo.txt"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/slack", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(orch.req.Attachments) != 1 || orch.req.Attachments[0].Filename != "memo.txt" || orch.req.Attachments[0].ExtractedText != "slack attachment text" {
		t.Fatalf("attachment was not passed to orchestrator: %+v", orch.req.Attachments)
	}
}

func TestAdapter_NormalizeEvent(t *testing.T) {
	adapter := NewAdapter("xoxb-token", "", &mockOrchestrator{})

	ev := EventEnvelope{
		Type: "event_callback",
		Event: EventInner{
			Type:        "app_mention",
			Text:        "<@Ubot> hello there",
			User:        "U1",
			Channel:     "C1",
			ClientMsgID: "m1",
		},
	}
	got, ok := adapter.NormalizeEvent(ev, []byte(`{}`))
	if !ok {
		t.Fatal("expected app_mention to be normalized")
	}
	if got.Text != "hello there" {
		t.Fatalf("unexpected normalized text: %q", got.Text)
	}
	if got.UserID != "U1" || got.ChatID != "C1" || got.MessageID != "m1" {
		t.Fatalf("unexpected normalized event: %+v", got)
	}
}

func TestAdapter_NormalizeEvent_IgnoreBotOrSubtype(t *testing.T) {
	adapter := NewAdapter("xoxb-token", "", &mockOrchestrator{})
	cases := []EventEnvelope{
		{Event: EventInner{Type: "message", Text: "x", Channel: "C1", BotID: "B1"}},
		{Event: EventInner{Type: "message", Text: "x", Channel: "C1", Subtype: "message_changed"}},
		{Event: EventInner{Type: "reaction_added", Text: "x", Channel: "C1"}},
	}
	for i, ev := range cases {
		if _, ok := adapter.NormalizeEvent(ev, nil); ok {
			t.Fatalf("case %d should be ignored", i)
		}
	}
}
