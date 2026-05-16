//go:build e2e

package e2e_test

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestE2E_Phase25LiveRuntimeHealthAndViewerConfig(t *testing.T) {
	if os.Getenv("PICOCLAW_LIVE_E2E") != "1" {
		t.Skip("set PICOCLAW_LIVE_E2E=1 to verify live service health and Viewer runtime config")
	}

	baseURL := strings.TrimRight(os.Getenv("PICOCLAW_LIVE_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:18790"
	}
	client := &http.Client{Timeout: 5 * time.Second}

	healthResp, err := client.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("live /health failed at %s: %v", baseURL, err)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("live /health status=%d, want 200", healthResp.StatusCode)
	}

	cfgResp, err := client.Get(baseURL + "/viewer/runtime-config")
	if err != nil {
		t.Fatalf("live /viewer/runtime-config failed at %s: %v", baseURL, err)
	}
	defer cfgResp.Body.Close()
	if cfgResp.StatusCode != http.StatusOK {
		t.Fatalf("live /viewer/runtime-config status=%d, want 200", cfgResp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(cfgResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode live runtime config: %v", err)
	}
	if _, ok := body["local_llm"]; !ok {
		t.Fatalf("runtime config must expose local_llm separately from repo example: keys=%v", keysOf(body))
	}
	if _, ok := body["stt_stream_url"]; !ok {
		t.Fatalf("runtime config must expose stt_stream_url for Viewer STT contract: keys=%v", keysOf(body))
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
