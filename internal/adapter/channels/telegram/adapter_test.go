package telegram

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

func TestAdapter_ServeHTTP_DocumentUsesAttachmentPipeline(t *testing.T) {
	orch := &captureOrchestrator{}
	adapter := NewAdapter("token", orch)
	adapter.SetAttachmentSaver(fakeAttachmentSaver{})
	adapter.SetHTTPClient(newHTTPClient(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/bottoken/getFile":
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true,"result":{"file_path":"docs/memo.txt","file_size":24}}`)), Header: make(http.Header)}, nil
		case "/file/bottoken/docs/memo.txt":
			h := make(http.Header)
			h.Set("Content-Type", "text/plain")
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString("telegram attachment text")), Header: h}, nil
		default:
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"ok":true}`)), Header: make(http.Header)}, nil
		}
	}))
	adapter.SetAPIBaseURL("https://telegram.invalid")

	body := []byte(`{"update_id":1,"message":{"message_id":10,"caption":"see memo","chat":{"id":123,"type":"private"},"from":{"id":456},"document":{"file_id":"file-1","file_name":"memo.txt","mime_type":"text/plain","file_size":24}}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if orch.req.UserMessage != "see memo" {
		t.Fatalf("caption should be passed as message text: %q", orch.req.UserMessage)
	}
	if len(orch.req.Attachments) != 1 || orch.req.Attachments[0].Filename != "memo.txt" || orch.req.Attachments[0].ExtractedText != "telegram attachment text" {
		t.Fatalf("attachment was not passed to orchestrator: %+v", orch.req.Attachments)
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
