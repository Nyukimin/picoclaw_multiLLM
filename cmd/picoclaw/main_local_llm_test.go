package main

import (
	"testing"

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
