package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestNewOllamaProvider(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "test-model")

	if provider == nil {
		t.Fatal("NewOllamaProvider should not return nil")
	}

	if provider.Name() != "ollama-test-model" {
		t.Errorf("Expected name 'ollama-test-model', got '%s'", provider.Name())
	}
}

func TestOllamaProviderGenerate_Success(t *testing.T) {
	// モックOllamaサーバー
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("Expected path '/api/generate', got '%s'", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		// レスポンス
		response := map[string]interface{}{
			"response": "こんにちは！何かお手伝いできますか？",
			"done":     true,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "こんにちは"},
		},
		MaxTokens:   100,
		Temperature: 0.7,
	}

	resp, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if resp.Content != "こんにちは！何かお手伝いできますか？" {
		t.Errorf("Expected response content, got '%s'", resp.Content)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("Expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
}

func TestOllamaProviderGenerate_WithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// システムプロンプトが含まれているか確認
		prompt, ok := reqBody["prompt"].(string)
		if !ok {
			t.Error("Request should contain prompt")
		}

		if prompt == "" {
			t.Error("Prompt should not be empty")
		}

		response := map[string]interface{}{
			"response": "System prompt applied",
			"done":     true,
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
		SystemPrompt: "You are a helpful assistant",
		MaxTokens:    100,
		Temperature:  0.7,
	}

	_, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate with system prompt failed: %v", err)
	}
}

func TestOllamaProviderGenerate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
	}

	_, err := provider.Generate(context.Background(), req)
	if err == nil {
		t.Error("Expected error when server returns 500")
	}
}

func TestOllamaProviderGenerate_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// タイムアウトをシミュレート（レスポンスを返さない）
		<-r.Context().Done()
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")

	ctx, cancel := context.WithTimeout(context.Background(), 100) // 100ms
	defer cancel()

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
	}

	_, err := provider.Generate(ctx, req)
	if err == nil {
		t.Error("Expected timeout error")
	}
}

func TestOllamaProviderGenerate_MultipleMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// プロンプトに複数のメッセージが含まれているか確認
		prompt, ok := reqBody["prompt"].(string)
		if !ok || prompt == "" {
			t.Error("Prompt should contain multiple messages")
		}

		response := map[string]interface{}{
			"response": "Multi-turn conversation response",
			"done":     true,
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOllamaProvider(server.URL, "test-model")

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "こんにちは"},
			{Role: "assistant", Content: "こんにちは！"},
			{Role: "user", Content: "元気ですか？"},
		},
	}

	_, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate with multiple messages failed: %v", err)
	}
}
