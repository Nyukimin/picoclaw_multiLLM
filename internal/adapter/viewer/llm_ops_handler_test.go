package viewer

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleLLMOpsStatus_ProxiesBearer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/status" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer testtok" {
			t.Fatalf("unexpected auth: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"ok": "1"})
	}))
	t.Cleanup(upstream.Close)

	h := HandleLLMOpsStatus(LLMOpsProxyOptions{BaseURL: upstream.URL, Token: "testtok"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/llm-ops/status", nil)
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != "1" {
		t.Fatalf("body: %+v", m)
	}
}

func TestLLMOpsProxyTimeoutCoversModelStartupWait(t *testing.T) {
	if llmOpsProxyTimeout < 10*time.Minute {
		t.Fatalf("llm ops proxy timeout should cover 600s startup wait, got %s", llmOpsProxyTimeout)
	}
}

func TestHandleLLMOpsHealth_ProxiesWithoutBearerRequirement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","daemon":"llm-mgmt"}`))
	}))
	t.Cleanup(upstream.Close)

	h := HandleLLMOpsHealth(LLMOpsProxyOptions{BaseURL: upstream.URL})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/llm-ops/health", nil)
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", rec.Code, rec.Body.String())
	}
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["daemon"] != "llm-mgmt" {
		t.Fatalf("body: %+v", m)
	}
}

func TestHandleLLMOpsStop_DefaultBody(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/control/stop" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	h := HandleLLMOpsStop(LLMOpsProxyOptions{BaseURL: upstream.URL, Token: "x"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/llm-ops/stop", strings.NewReader(""))
	h(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: %d", rec.Code)
	}
	if gotBody != `{"roles":["Chat","Worker"]}` {
		t.Fatalf("upstream body: %q", gotBody)
	}
}

func TestHandleLLMOpsStart_DefaultBody(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/control/start" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)

	h := HandleLLMOpsStart(LLMOpsProxyOptions{BaseURL: upstream.URL, Token: "x"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/llm-ops/start", strings.NewReader(""))
	h(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: %d", rec.Code)
	}
	if gotBody != `{"selection":"Worker"}` {
		t.Fatalf("upstream body: %q", gotBody)
	}
}

func TestHandleLLMOpsStart_ProxiesSelection(t *testing.T) {
	var gotBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/control/start" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("unexpected auth: %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok_all":true}`))
	}))
	t.Cleanup(upstream.Close)

	h := HandleLLMOpsStart(LLMOpsProxyOptions{BaseURL: upstream.URL, Token: "tok"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/llm-ops/start", strings.NewReader(`{"selection":"Heavy"}`))
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if gotBody != `{"selection":"Heavy"}` {
		t.Fatalf("upstream body: %q", gotBody)
	}
}

func TestHandleLLMOpsNotConfigured(t *testing.T) {
	h := HandleLLMOpsStatus(LLMOpsProxyOptions{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/llm-ops/status", nil)
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
