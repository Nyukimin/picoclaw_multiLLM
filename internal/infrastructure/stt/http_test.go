package stt

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler_FileTranscribesMultipartWAV(t *testing.T) {
	h := NewHandler(MockProvider{Text: "ルミナ、今日の予定を確認して。"})
	h.Now = func() time.Time { return time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC) }
	req := multipartWAVRequest(t, "/stt/file", tinyWAV())
	rec := httptest.NewRecorder()

	h.File(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out Result
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Text == "" || out.Provider != ProviderMock || out.EventID == "" {
		t.Fatalf("unexpected result: %+v", out)
	}
}

func TestHandler_ChatInputReturnsVoiceUserInput(t *testing.T) {
	h := NewHandler(MockProvider{Text: "ルミナ、RenCrowの状態を確認して。"})
	req := multipartWAVRequest(t, "/stt/chat-input", tinyWAV())
	rec := httptest.NewRecorder()

	h.ChatInput(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["type"] != "user_input" || out["source"] != "local_stt" || out["input_type"] != "voice" {
		t.Fatalf("unexpected chat-input envelope: %+v", out)
	}
	if out["text"] == "" || out["event_id"] == "" {
		t.Fatalf("missing text/event_id: %+v", out)
	}
}

func TestHandler_InvalidAudioReturnsJSONError(t *testing.T) {
	h := NewHandler(MockProvider{Text: "ignored"})
	req := multipartWAVRequest(t, "/stt/file", []byte("not wav"))
	rec := httptest.NewRecorder()

	h.File(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out Result
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.ErrorCode != ErrorInvalidAudio {
		t.Fatalf("unexpected error result: %+v", out)
	}
}

func multipartWAVRequest(t *testing.T, path string, wav []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(wav); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func tinyWAV() []byte {
	dataSize := 2
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	out[4] = byte(36 + dataSize)
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	out[16] = 16
	out[20] = 1
	out[22] = 1
	out[24] = 0x80
	out[25] = 0x3e
	out[28] = 0x00
	out[29] = 0x7d
	out[32] = 2
	out[34] = 16
	copy(out[36:40], "data")
	out[40] = byte(dataSize)
	return out
}
