package idlechat

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

type idlechatAliasTestProvider struct {
	name string
}

func (p idlechatAliasTestProvider) Generate(context.Context, llm.GenerateRequest) (llm.GenerateResponse, error) {
	return llm.GenerateResponse{Content: "ok"}, nil
}

func (p idlechatAliasTestProvider) Name() string {
	return p.name
}

func TestProviderForSpeakerPrefersChatWorkerAlias(t *testing.T) {
	orch := NewIdleChatOrchestrator(idlechatAliasTestProvider{name: "mio"}, &session.CentralMemory{}, nil, 1, 1, 0.5, nil, "")
	orch.SetSpeakerProviders(map[string]llm.LLMProvider{
		"shiro":      idlechatAliasTestProvider{name: "Worker"},
		"chatworker": idlechatAliasTestProvider{name: "ChatWorker"},
	})

	provider := orch.providerForSpeaker("shiro")
	if provider == nil {
		t.Fatal("expected provider")
	}
	if got := provider.Name(); got != "ChatWorker" {
		t.Fatalf("providerForSpeaker(shiro) = %q, want ChatWorker", got)
	}
}

func TestChatWorkerDefaultsToNonThinking(t *testing.T) {
	orch := NewIdleChatOrchestrator(idlechatAliasTestProvider{name: "mio"}, &session.CentralMemory{}, nil, 1, 1, 0.5, nil, "")
	if got := orch.speakerThinkEnabled("chatworker"); got {
		t.Fatalf("speakerThinkEnabled(chatworker) = %t, want false", got)
	}
}

func TestShiroDialogueUsesChatWorkerMaxTokens(t *testing.T) {
	if got := idleMaxTokensForSpeaker("shiro", idleChatResponseMaxTokens); got != idleChatShiroResponseMaxTokens {
		t.Fatalf("idleMaxTokensForSpeaker(shiro, response) = %d, want %d", got, idleChatShiroResponseMaxTokens)
	}
	if got := idleMaxTokensForSpeaker("shiro", idleChatRetryMaxTokens); got != idleChatShiroRetryMaxTokens {
		t.Fatalf("idleMaxTokensForSpeaker(shiro, retry) = %d, want %d", got, idleChatShiroRetryMaxTokens)
	}
}
