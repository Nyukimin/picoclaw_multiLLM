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

func TestHandleRuntimeConfig_ReturnsLLMOpsEnabled(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		LLMOpsConfigured: true,
		LLMOpsEnabled:    true,
		LLMOpsBaseURL:    "http://192.168.1.31:8079/",
		LocalLLM: LocalLLMRuntimeConfig{
			Enabled:           true,
			Provider:          "local_openai",
			ChatBaseURL:       "http://192.168.1.31:8081/",
			WorkerBaseURL:     "http://192.168.1.31:8082/",
			HeavyBaseURL:      "http://192.168.1.31:8083/",
			WildBaseURL:       "http://192.168.1.31:8084/",
			ChatModel:         "Chat",
			WorkerModel:       "Worker",
			HeavyModel:        "Heavy",
			WildModel:         "Wild",
			TimeoutSec:        120,
			GlobalConcurrency: 1,
			ModelConcurrency:  1,
		},
	})
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, "/viewer/runtime-config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.LLMOpsEnabled {
		t.Fatalf("expected llm_ops_enabled: %+v", body)
	}
	if !body.LLMOpsConfigured {
		t.Fatalf("expected llm_ops_configured: %+v", body)
	}
	if body.LLMOpsBaseURL != "http://192.168.1.31:8079" {
		t.Fatalf("unexpected llm ops base url: %+v", body)
	}
	if !body.LocalLLM.Enabled || body.LocalLLM.ChatBaseURL != "http://192.168.1.31:8081" || body.LocalLLM.WorkerModel != "Worker" || body.LocalLLM.HeavyBaseURL != "http://192.168.1.31:8083" || body.LocalLLM.HeavyModel != "Heavy" {
		t.Fatalf("unexpected local llm runtime config: %+v", body.LocalLLM)
	}
}
