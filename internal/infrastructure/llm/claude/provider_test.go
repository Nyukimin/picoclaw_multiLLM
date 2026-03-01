package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestNewClaudeProvider(t *testing.T) {
	provider := NewClaudeProvider("test-api-key", "claude-3-5-sonnet-20241022")

	if provider == nil {
		t.Fatal("NewClaudeProvider should not return nil")
	}

	if provider.Name() != "claude-claude-3-5-sonnet-20241022" {
		t.Errorf("Expected name 'claude-claude-3-5-sonnet-20241022', got '%s'", provider.Name())
	}
}

func TestClaudeProviderGenerate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("Expected path '/v1/messages', got '%s'", r.URL.Path)
		}

		// APIキーヘッダー確認
		apiKey := r.Header.Get("x-api-key")
		if apiKey != "test-api-key" {
			t.Errorf("Expected API key 'test-api-key', got '%s'", apiKey)
		}

		// Anthropic-Versionヘッダー確認
		version := r.Header.Get("anthropic-version")
		if version == "" {
			t.Error("anthropic-version header should be set")
		}

		// レスポンス
		response := map[string]interface{}{
			"id":   "msg_123",
			"type": "message",
			"role": "assistant",
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": "こんにちは！お手伝いします。",
				},
			},
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  10,
				"output_tokens": 20,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewClaudeProvider("test-api-key", "claude-3-5-sonnet-20241022")
	provider.SetBaseURL(server.URL) // テスト用にベースURLを上書き

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "こんにちは"},
		},
		MaxTokens:   1000,
		Temperature: 0.7,
	}

	resp, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if resp.Content != "こんにちは！お手伝いします。" {
		t.Errorf("Expected response content, got '%s'", resp.Content)
	}

	if resp.TokensUsed != 30 { // input 10 + output 20
		t.Errorf("Expected 30 tokens used, got %d", resp.TokensUsed)
	}

	if resp.FinishReason != "end_turn" {
		t.Errorf("Expected finish reason 'end_turn', got '%s'", resp.FinishReason)
	}
}

func TestClaudeProviderGenerate_WithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// システムプロンプトが含まれているか確認
		system, ok := reqBody["system"].(string)
		if !ok || system == "" {
			t.Error("Request should contain system prompt")
		}

		if system != "You are a helpful assistant" {
			t.Errorf("Expected system prompt 'You are a helpful assistant', got '%s'", system)
		}

		response := map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "System prompt applied"},
			},
			"stop_reason": "end_turn",
			"usage":       map[string]interface{}{"input_tokens": 10, "output_tokens": 5},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewClaudeProvider("test-api-key", "claude-3-5-sonnet-20241022")
	provider.SetBaseURL(server.URL)

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
		SystemPrompt: "You are a helpful assistant",
		MaxTokens:    1000,
	}

	_, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate with system prompt failed: %v", err)
	}
}

func TestClaudeProviderGenerate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		response := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "rate_limit_error",
				"message": "Rate limit exceeded",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewClaudeProvider("test-api-key", "claude-3-5-sonnet-20241022")
	provider.SetBaseURL(server.URL)

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
	}

	_, err := provider.Generate(context.Background(), req)
	if err == nil {
		t.Error("Expected error when API returns rate limit error")
	}
}

func TestClaudeProviderGenerate_InvalidAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		response := map[string]interface{}{
			"type": "error",
			"error": map[string]interface{}{
				"type":    "authentication_error",
				"message": "Invalid API key",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewClaudeProvider("invalid-key", "claude-3-5-sonnet-20241022")
	provider.SetBaseURL(server.URL)

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
	}

	_, err := provider.Generate(context.Background(), req)
	if err == nil {
		t.Error("Expected error for invalid API key")
	}
}
