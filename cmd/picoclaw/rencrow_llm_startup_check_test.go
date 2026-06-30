package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
)

func TestLogRenCrowLLMStartupCheckProbesConfiguredEndpoints(t *testing.T) {
	var sawAuthorizedStatus bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health/live", "/health/ready", "/health", "/v1/models":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v1/status":
			if r.Header.Get("Authorization") == "Bearer tok" {
				sawAuthorizedStatus = true
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"roles": map[string]any{
					"Chat": map[string]any{"health_ok": true},
					"Wild": map[string]any{"health_ok": true},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(upstream.Close)

	cfg := &config.Config{}
	cfg.RenCrow.LLM.Enabled = true
	cfg.RenCrow.LLM.BaseURL = upstream.URL
	cfg.RenCrow.LLM.TokenEnv = "RENCROW_LLM_TOKEN"
	cfg.RenCrow.LLM.Health.LivePath = "/health/live"
	cfg.RenCrow.LLM.Health.ReadyPath = "/health/ready"
	cfg.RenCrow.LLM.Endpoints.StatusPath = "/v1/status"
	cfg.RenCrow.LLM.Recipients = map[string]config.RenCrowLLMRecipientConfig{
		"midori": {Role: "wild", Model: "Wild", Selection: "Wild"},
	}
	cfg.LLMOps.Enabled = true
	cfg.LLMOps.BaseURL = upstream.URL
	cfg.LocalLLM.Enabled = true
	cfg.LocalLLM.Provider = "local_openai"
	cfg.LocalLLM.ChatBaseURL = upstream.URL
	cfg.LocalLLM.WorkerBaseURL = upstream.URL
	cfg.LocalLLM.HeavyBaseURL = upstream.URL
	cfg.LocalLLM.WildBaseURL = upstream.URL
	cfg.LocalLLM.ChatModel = "Chat"
	cfg.LocalLLM.WorkerModel = "Worker"
	cfg.LocalLLM.ChatWorkerModel = "ChatWorker"
	cfg.LocalLLM.HeavyModel = "Heavy"
	cfg.LocalLLM.WildModel = "Wild"

	var logs []string
	client := &http.Client{Timeout: time.Second}
	results := logRenCrowLLMStartupCheck(context.Background(), cfg, "tok", client, func(format string, args ...any) {
		logs = append(logs, fmt.Sprintf(format, args...))
	})

	if !sawAuthorizedStatus {
		t.Fatal("expected status probe to use bearer token")
	}
	if len(results) < 8 {
		t.Fatalf("expected startup probes, got %d: %+v", len(results), results)
	}
	joined := strings.Join(logs, "\n")
	for _, want := range []string{
		"rencrow_base=" + upstream.URL,
		"llm_ops_base=" + upstream.URL,
		"recipient id=midori role=wild model=Wild selection=Wild",
		"local_endpoint role=Wild base=" + upstream.URL + " model=Wild",
		"probe name=llm_ops.status ok=true",
		"probe name=local.wild.models ok=true",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected log to contain %q\nlogs:\n%s", want, joined)
		}
	}
}

func TestExpandRenCrowLLMLaunchCommand(t *testing.T) {
	got := expandRenCrowLLMLaunchCommand([]string{
		"uv",
		"run",
		"{root}/scripts/start.py",
		"",
	}, "/tmp/RenCrow_LLM")
	want := []string{"uv", "run", "/tmp/RenCrow_LLM/scripts/start.py"}
	if len(got) != len(want) {
		t.Fatalf("command length = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("command[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
