package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleRuntimeConfig_ReturnsSTTStreamURL(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		STTBaseURL:   "https://192.168.1.31:8443/",
		STTStreamURL: "wss://192.168.1.31:8443/stt/stream",
	})
	req := httptest.NewRequest(http.MethodGet, "/viewer/runtime-config", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode runtime config: %v", err)
	}
	if body.STTStreamURL != "wss://192.168.1.31:8443/stt/stream" {
		t.Fatalf("unexpected stt stream url: %+v", body)
	}
	if body.STTBaseURL != "https://192.168.1.31:8443" {
		t.Fatalf("unexpected stt base url: %+v", body)
	}
}
