package main

import (
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
)

func TestLocalLLMBaseURLForAlias_UsesRoleOverride(t *testing.T) {
	cfg := &config.Config{
		LocalLLM: config.LocalLLMConfig{
			BaseURL:       "http://192.168.1.31:8081",
			ChatBaseURL:   "http://192.168.1.31:8081",
			WorkerBaseURL: "http://192.168.1.31:8082",
			WildBaseURL:   "http://192.168.1.31:8083",
		},
	}

	if got := localLLMBaseURLForAlias(cfg, "Chat"); got != "http://192.168.1.31:8081" {
		t.Fatalf("unexpected chat base url: %s", got)
	}
	if got := localLLMBaseURLForAlias(cfg, "Worker"); got != "http://192.168.1.31:8082" {
		t.Fatalf("unexpected worker base url: %s", got)
	}
	if got := localLLMBaseURLForAlias(cfg, "Wild"); got != "http://192.168.1.31:8083" {
		t.Fatalf("unexpected wild base url: %s", got)
	}
}

func TestLocalLLMBaseURLForAlias_FallsBackToBaseURL(t *testing.T) {
	cfg := &config.Config{
		LocalLLM: config.LocalLLMConfig{
			BaseURL: "http://192.168.1.31:8081",
		},
	}

	if got := localLLMBaseURLForAlias(cfg, "Worker"); got != "http://192.168.1.31:8081" {
		t.Fatalf("unexpected fallback base url: %s", got)
	}
}

func TestLocalLLMTimeoutForAlias_UsesRoleSpecificTimeouts(t *testing.T) {
	cfg := &config.Config{
		LocalLLM: config.LocalLLMConfig{
			TimeoutSec: 120,
		},
	}

	cases := map[string]time.Duration{
		"Chat":   10 * time.Second,
		"Wild":   15 * time.Second,
		"Heavy":  30 * time.Second,
		"Worker": 120 * time.Second,
	}
	for alias, want := range cases {
		if got := localLLMTimeoutForAlias(cfg, alias); got != want {
			t.Fatalf("%s timeout = %s, want %s", alias, got, want)
		}
	}
}

func TestLocalLLMTimeoutForAlias_DefaultsWorkerTo120Seconds(t *testing.T) {
	if got := localLLMTimeoutForAlias(&config.Config{}, "Worker"); got != 120*time.Second {
		t.Fatalf("Worker timeout = %s, want 120s", got)
	}
}
