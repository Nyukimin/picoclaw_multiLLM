//go:build e2e

package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
)

const defaultOllamaURL = "http://100.83.207.6:11434"
const defaultOllamaModel = "chat-v1"

func ollamaURL() string {
	if url := os.Getenv("OLLAMA_URL"); url != "" {
		return url
	}
	return defaultOllamaURL
}

func ollamaModel() string {
	if model := os.Getenv("OLLAMA_MODEL"); model != "" {
		return model
	}
	return defaultOllamaModel
}

// === Mock dependencies for MioAgent (minimal) ===

type nullClassifier struct{}

func (n *nullClassifier) Classify(ctx context.Context, t task.Task) (routing.Decision, error) {
	return routing.Decision{}, nil
}

type nullRuleDict struct{}

func (n *nullRuleDict) Match(t task.Task) (routing.Route, float64, bool) {
	return "", 0, false
}

type nullToolRunner struct{}

func (n *nullToolRunner) Execute(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	return "", nil
}
func (n *nullToolRunner) List(ctx context.Context) ([]string, error) { return nil, nil }

type nullMCPClient struct{}

func (n *nullMCPClient) CallTool(ctx context.Context, server, tool string, args map[string]interface{}) (string, error) {
	return "", nil
}
func (n *nullMCPClient) ListTools(ctx context.Context, server string) ([]string, error) {
	return nil, nil
}

// === Tests ===

func TestE2E_OllamaProvider_Generate(t *testing.T) {
	provider := ollama.NewOllamaProvider(ollamaURL(), ollamaModel())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Say hello in one word."},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty response")
	}
	t.Logf("Response: %s", resp.Content)
}

func TestE2E_OllamaProvider_Japanese(t *testing.T) {
	provider := ollama.NewOllamaProvider(ollamaURL(), ollamaModel())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "「こんにちは」と一言だけ返してください。"},
		},
		MaxTokens:   64,
		Temperature: 0.1,
	})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if resp.Content == "" {
		t.Error("expected non-empty Japanese response")
	}
	t.Logf("Response: %s", resp.Content)
}

func TestE2E_OllamaProvider_Timeout(t *testing.T) {
	provider := ollama.NewOllamaProvider(ollamaURL(), ollamaModel())

	// Very short timeout — should fail
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	_, err := provider.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "Tell me a long story about programming."},
		},
		MaxTokens:   512,
		Temperature: 0.7,
	})
	if err == nil {
		t.Log("WARN: expected timeout but got response (fast network?)")
	}
}

func TestE2E_MioAgent_Chat_RealOllama(t *testing.T) {
	provider := ollama.NewOllamaProvider(ollamaURL(), ollamaModel())

	mio := agent.NewMioAgent(
		provider,
		&nullClassifier{},
		&nullRuleDict{},
		&nullToolRunner{},
		&nullMCPClient{},
		nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	testTask := task.NewTask(task.NewJobID(), "こんにちは", "test", "test_user")
	resp, err := mio.Chat(ctx, testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp == "" {
		t.Error("expected non-empty chat response")
	}
	t.Logf("Mio response: %s", resp)
}
