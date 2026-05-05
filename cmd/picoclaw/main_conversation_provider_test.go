package main

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

type fakeConversationProvider struct {
	name string
}

func (f fakeConversationProvider) Generate(context.Context, llm.GenerateRequest) (llm.GenerateResponse, error) {
	return llm.GenerateResponse{Content: "ok"}, nil
}

func (f fakeConversationProvider) Name() string {
	return f.name
}

func TestBuildConversationTextProviderUsesLocalWorkerWhenLocalLLMEnabled(t *testing.T) {
	worker := fakeConversationProvider{name: "worker-provider"}
	provider, label := buildConversationTextProvider(&config.Config{
		LocalLLM: config.LocalLLMConfig{Enabled: true},
	}, primaryLLMProviders{Worker: worker})

	if provider != worker {
		t.Fatalf("expected local Worker provider, got %#v", provider)
	}
	if label != "local_llm Worker" {
		t.Fatalf("unexpected label: %s", label)
	}
}
