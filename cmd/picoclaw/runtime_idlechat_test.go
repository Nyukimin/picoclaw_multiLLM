package main

import (
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
)

func TestSelectForecastProviderPrefersCoderPriorityOverWorker(t *testing.T) {
	worker := fakeConversationProvider{name: "worker-provider"}
	chat := fakeConversationProvider{name: "chat-provider"}
	provider, label := selectForecastProvider(&config.Config{
		LocalLLM: config.LocalLLMConfig{
			Enabled:     true,
			WorkerModel: "Worker",
			ChatModel:   "Chat",
		},
		Coder2: config.CoderConfig{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-4o-mini",
			APIKey:   "test-key",
		},
		Coder1: config.CoderConfig{
			Enabled:  true,
			Provider: "local_openai",
			Model:    "Worker",
			BaseURL:  "http://127.0.0.1:8082",
		},
	}, chat, worker, nil)

	if provider == nil || provider == worker || provider == chat {
		t.Fatalf("expected Coder1 provider, got %#v", provider)
	}
	if !strings.Contains(label, "Coder1") || !strings.Contains(label, "local_openai") {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestSelectForecastProviderSkipsBrokenCoderAndUsesNextCoder(t *testing.T) {
	primary, primaryLabel, external, externalLabel := selectForecastProviders(&config.Config{
		Coder1: config.CoderConfig{
			Enabled:  true,
			Provider: "local_openai",
			Model:    "Worker",
		},
		Coder2: config.CoderConfig{
			Enabled:  true,
			Provider: "local_openai",
			Model:    "Worker",
			BaseURL:  "http://127.0.0.1:8082",
		},
	})

	if primary != nil || primaryLabel != "" {
		t.Fatalf("broken Coder1 should not become primary: provider=%#v label=%q", primary, primaryLabel)
	}
	if external == nil {
		t.Fatal("expected Coder2 external provider")
	}
	if !strings.Contains(externalLabel, "Coder2") {
		t.Fatalf("unexpected external label: %q", externalLabel)
	}
}

func TestSelectForecastProviderSkipsBrokenCoder1AndUsesCoder2OpenAI(t *testing.T) {
	primary, primaryLabel, external, externalLabel := selectForecastProviders(&config.Config{
		Coder1: config.CoderConfig{
			Enabled:  true,
			Provider: "local_openai",
			Model:    "Worker",
		},
		Coder2: config.CoderConfig{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-4o-mini",
			APIKey:   "test-key",
		},
	})

	if primary != nil || primaryLabel != "" {
		t.Fatalf("broken Coder1 should not become primary: provider=%#v label=%q", primary, primaryLabel)
	}
	if external == nil {
		t.Fatal("expected Coder2 OpenAI external provider")
	}
	if !strings.Contains(externalLabel, "Coder2 openai") || !strings.Contains(externalLabel, "gpt-4o-mini") {
		t.Fatalf("unexpected external label: %q", externalLabel)
	}
}

func TestSelectForecastProviderDoesNotFallBackToChatWhenNoCoderAvailable(t *testing.T) {
	chat := fakeConversationProvider{name: "chat-provider"}
	provider, label := selectForecastProvider(&config.Config{
		LocalLLM: config.LocalLLMConfig{
			Enabled:   true,
			ChatModel: "Chat",
		},
	}, chat, nil, nil)

	if provider != nil {
		t.Fatalf("Forecast must not fall back to Chat provider, got %#v", provider)
	}
	if label != "" {
		t.Fatalf("unexpected label: %q", label)
	}
}
