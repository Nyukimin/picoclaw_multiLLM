package health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
