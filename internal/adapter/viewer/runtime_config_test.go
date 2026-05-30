package viewer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRuntimeConfig_ReturnsSTTStreamURL(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		STTBaseURL:    "https://192.168.1.31:8443/",
		STTStreamURL:  "wss://192.168.1.31:8443/stt/stream",
		TTSBaseURL:    "http://127.0.0.1:7870/",
		TTSHealthPath: "/gradio_api/info",
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
	if body.TTSBaseURL != "http://127.0.0.1:7870" || body.TTSHealthPath != "/gradio_api/info" {
		t.Fatalf("unexpected tts runtime config: %+v", body)
	}
}

func TestHandleRuntimeConfig_KeepsConfiguredSTTStreamURLForLANHTTP(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		STTBaseURL:   "http://192.168.1.207:8766",
		STTStreamURL: "ws://192.168.1.207:8766/stt",
	})
	req := httptest.NewRequest(http.MethodGet, "http://192.168.1.204:18790/viewer/runtime-config", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.STTStreamURL != "ws://192.168.1.207:8766/stt" {
		t.Fatalf("unexpected LAN stt stream url: %+v", body)
	}
}

func TestHandleRuntimeConfig_ReturnsSameOriginWSSForTailscaleHTTPS(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		STTBaseURL:   "http://192.168.1.207:8766",
		STTStreamURL: "ws://192.168.1.207:8766/stt",
	})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18790/viewer/runtime-config", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "fujitsu-ubunts.tailb07d8d.ts.net")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	var body RuntimeConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.STTStreamURL != "wss://fujitsu-ubunts.tailb07d8d.ts.net/stt" {
		t.Fatalf("unexpected Tailscale stt stream url: %+v", body)
	}
	if body.STTBaseURL != "http://192.168.1.207:8766" {
		t.Fatalf("server-side stt base url should remain LAN-local: %+v", body)
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
		WebwrightFetch: WebwrightFetchRuntimeConfig{
			Enabled:           true,
			RunnerPath:        "tools/webwright_fetch/run_webwright_fetch.py",
			ConfigPath:        "tools/webwright_fetch/config_local_worker.yaml",
			OutputDir:         "tmp/webwright_runs",
			StagingOutputDir:  "tmp/webwright_staging",
			UvxFrom:           "git+https://github.com/microsoft/Webwright.git",
			ResponsesEndpoint: "http://192.168.1.31:8082/v1/responses/",
			Model:             "Coder1",
			APIKeyConfigured:  true,
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
	if !body.WebwrightFetch.Enabled || body.WebwrightFetch.ResponsesEndpoint != "http://192.168.1.31:8082/v1/responses" || body.WebwrightFetch.Model != "Coder1" {
		t.Fatalf("unexpected webwright fetch runtime config: %+v", body.WebwrightFetch)
	}
	if !body.WebwrightFetch.APIKeyConfigured {
		t.Fatalf("expected webwright api key configured without exposing value: %+v", body.WebwrightFetch)
	}
}

func TestHandleRuntimeConfig_ReturnsRuntimeReadinessWithoutSecretValues(t *testing.T) {
	handler := HandleRuntimeConfig(DebugSystemOptions{
		STTBaseURL:   "http://127.0.0.1:8766",
		TTSBaseURL:   "http://127.0.0.1:7870",
		STTStreamURL: "wss://127.0.0.1/stt",
		RuntimeReadiness: RuntimeDependencyReadiness{
			SlackCredentialsPresent:      true,
			SlackWebhookRegistered:       true,
			SlackFilePayloadPipeline:     true,
			DiscordCredentialsPresent:    false,
			DiscordWebhookRegistered:     false,
			DiscordFilePayloadPipeline:   false,
			TelegramCredentialsPresent:   true,
			TelegramWebhookRegistered:    true,
			TelegramFilePayloadPipeline:  true,
			STTGatewayEnvPresent:         true,
			TTSProviderEnvPresent:        false,
			DistributedEnabled:           true,
			DistributedTransportsPresent: true,
			DistributedSSHConfigured:     true,
			DistributedSSHConnected:      false,
			DistributedLocalTransport:    true,
			ConversationEnabled:          true,
			L1SQLiteConfigPresent:        true,
			MemoryLayersAvailable:        true,
			MemoryLayersStatus:           true,
			SourceRegistryAvailable:      true,
			SourceRegistryStatus:         true,
			KnowledgeMemoryEnabled:       true,
			KnowledgeMemoryStatus:        true,
			BrowserTraceAPIEnabled:       true,
			BrowserTraceAPIStatus:        true,
			BrowserTraceAPIFetcher:       true,
			SandboxEnabled:               false,
			SandboxStatusAvailable:       true,
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
	if !body.RuntimeReadiness.SlackCredentialsPresent || !body.RuntimeReadiness.SlackWebhookRegistered || !body.RuntimeReadiness.SlackFilePayloadPipeline || body.RuntimeReadiness.DiscordCredentialsPresent || body.RuntimeReadiness.DiscordWebhookRegistered || body.RuntimeReadiness.DiscordFilePayloadPipeline || !body.RuntimeReadiness.TelegramCredentialsPresent || !body.RuntimeReadiness.TelegramWebhookRegistered || !body.RuntimeReadiness.TelegramFilePayloadPipeline || !body.RuntimeReadiness.STTGatewayEnvPresent || !body.RuntimeReadiness.STTGatewayConfigPresent || body.RuntimeReadiness.TTSProviderEnvPresent || !body.RuntimeReadiness.TTSProviderConfigPresent || !body.RuntimeReadiness.DistributedEnabled || !body.RuntimeReadiness.DistributedTransportsPresent || !body.RuntimeReadiness.DistributedSSHConfigured || body.RuntimeReadiness.DistributedSSHConnected || !body.RuntimeReadiness.DistributedLocalTransport || !body.RuntimeReadiness.ConversationEnabled || !body.RuntimeReadiness.L1SQLiteConfigPresent || !body.RuntimeReadiness.MemoryLayersAvailable || !body.RuntimeReadiness.MemoryLayersStatus || !body.RuntimeReadiness.SourceRegistryAvailable || !body.RuntimeReadiness.SourceRegistryStatus || !body.RuntimeReadiness.KnowledgeMemoryEnabled || !body.RuntimeReadiness.KnowledgeMemoryStatus || !body.RuntimeReadiness.BrowserTraceAPIEnabled || !body.RuntimeReadiness.BrowserTraceAPIStatus || !body.RuntimeReadiness.BrowserTraceAPIFetcher || body.RuntimeReadiness.SandboxEnabled || !body.RuntimeReadiness.SandboxStatusAvailable {
		t.Fatalf("unexpected runtime readiness: %+v", body.RuntimeReadiness)
	}
	if strings.Contains(rec.Body.String(), "SLACK_BOT_TOKEN") || strings.Contains(rec.Body.String(), "TELEGRAM_BOT_TOKEN") {
		t.Fatalf("runtime config leaked env names or secrets: %s", rec.Body.String())
	}
}
