package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
)

func TestOpenAICompatibleChatCheck_OK(t *testing.T) {
	paths := make([]string, 0, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody["parse_reasoning"] != true || reqBody["include_reasoning"] != false || reqBody["separate_reasoning"] != true {
			t.Fatalf("health check should use ThinkingBridge-safe flags: %#v", reqBody)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer srv.Close()

	check := NewOpenAICompatibleChatCheck("Chat", srv.URL, "Chat", "", 0)
	result := check.Run(context.Background())
	if result.Status != domainhealth.StatusOK {
		t.Fatalf("status = %s, want ok; message=%s", result.Status, result.Message)
	}
	if result.Name != "local_llm_chat" {
		t.Fatalf("name = %q", result.Name)
	}
	if len(paths) != 1 || paths[0] != "/v1/chat/completions" {
		t.Fatalf("health check must not probe /ready; paths=%v", paths)
	}
}

func TestOpenAICompatibleChatCheck_DownOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	check := NewOpenAICompatibleChatCheck("Worker", srv.URL, "Worker", "", 0)
	result := check.Run(context.Background())
	if result.Status != domainhealth.StatusDown {
		t.Fatalf("status = %s, want down", result.Status)
	}
}

func TestOpenAICompatibleChatCheck_WorkerTimeoutIsBackendTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer srv.Close()

	check := NewOpenAICompatibleChatCheck("Worker", srv.URL, "Worker", "", 10*time.Millisecond)
	result := check.Run(context.Background())
	if result.Status != domainhealth.StatusDown {
		t.Fatalf("status = %s, want down", result.Status)
	}
	if !strings.Contains(result.Message, "worker_backend_timeout") {
		t.Fatalf("message=%q", result.Message)
	}
}

func TestOpenAICompatibleChatCheck_Worker429BackendBusyIsDegraded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"backend_busy"}}`))
	}))
	defer srv.Close()

	check := NewOpenAICompatibleChatCheck("ChatWorker", srv.URL, "ChatWorker", "", time.Second)
	result := check.Run(context.Background())
	if result.Status != domainhealth.StatusDegraded {
		t.Fatalf("status = %s, want degraded; message=%s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "worker_backend_busy") {
		t.Fatalf("message=%q", result.Message)
	}
}

func TestOpenAICompatibleChatCheck_Worker504BackendTimeoutIsDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"error":{"code":"backend_timeout"}}`))
	}))
	defer srv.Close()

	check := NewOpenAICompatibleChatCheck("Worker", srv.URL, "Worker", "", time.Second)
	result := check.Run(context.Background())
	if result.Status != domainhealth.StatusDown {
		t.Fatalf("status = %s, want down; message=%s", result.Status, result.Message)
	}
	if !strings.Contains(result.Message, "worker_backend_timeout") {
		t.Fatalf("message=%q", result.Message)
	}
}

func TestOpenAICompatibleChatCheck_ExclusiveRoleConnectionRefusedIsStandby(t *testing.T) {
	for _, role := range []string{"Worker", "Heavy", "Wild"} {
		t.Run(role, func(t *testing.T) {
			check := NewOpenAICompatibleChatCheck(role, "http://127.0.0.1:1", role, "", 10*time.Millisecond)
			result := check.Run(context.Background())
			if result.Status != domainhealth.StatusDegraded {
				t.Fatalf("status = %s, want degraded; message=%s", result.Status, result.Message)
			}
			if !strings.Contains(result.Message, "standby") {
				t.Fatalf("message=%q", result.Message)
			}
		})
	}
}
