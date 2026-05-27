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
	worker := fakeConversationProvider{name: "worker-provider"}
	provider, label := selectForecastProvider(&config.Config{
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
	}, nil, worker, nil)

	if provider == nil || provider == worker {
		t.Fatalf("expected Coder2 provider, got %#v", provider)
	}
	if !strings.Contains(label, "Coder2") {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestSelectForecastProviderSkipsBrokenCoder1AndUsesCoder2OpenAI(t *testing.T) {
	worker := fakeConversationProvider{name: "worker-provider"}
	provider, label := selectForecastProvider(&config.Config{
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
	}, nil, worker, nil)

	if provider == nil || provider == worker {
		t.Fatalf("expected Coder2 OpenAI provider, got %#v", provider)
	}
	if !strings.Contains(label, "Coder2 openai") || !strings.Contains(label, "gpt-4o-mini") {
		t.Fatalf("unexpected label: %q", label)
	}
}

func TestSelectForecastProviderFallsBackToChatWhenNoCoderAvailable(t *testing.T) {
	chat := fakeConversationProvider{name: "chat-provider"}
	provider, label := selectForecastProvider(&config.Config{
		LocalLLM: config.LocalLLMConfig{
			Enabled:   true,
			ChatModel: "Chat",
		},
	}, chat, nil, nil)

	if provider != chat {
		t.Fatalf("expected Chat provider, got %#v", provider)
	}
	if !strings.Contains(label, "Chat") {
		t.Fatalf("unexpected label: %q", label)
	}
}
