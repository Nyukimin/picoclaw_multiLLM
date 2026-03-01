package deepseek

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestNewDeepSeekProvider(t *testing.T) {
	provider := NewDeepSeekProvider("test-api-key", "deepseek-chat")

	if provider == nil {
		t.Fatal("NewDeepSeekProvider should not return nil")
	}

	if provider.Name() != "deepseek-deepseek-chat" {
		t.Errorf("Expected name 'deepseek-deepseek-chat', got '%s'", provider.Name())
	}
}

func TestDeepSeekProviderGenerate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path '/v1/chat/completions', got '%s'", r.URL.Path)
		}

		// Authorizationヘッダー確認
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Expected 'Bearer test-api-key', got '%s'", auth)
		}

		// レスポンス（OpenAI互換）
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "DeepSeekの応答です。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"total_tokens": 25,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewDeepSeekProvider("test-api-key", "deepseek-chat")
	provider.SetBaseURL(server.URL)

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

	if resp.Content != "DeepSeekの応答です。" {
		t.Errorf("Expected response content, got '%s'", resp.Content)
	}

	if resp.TokensUsed != 25 {
		t.Errorf("Expected 25 tokens used, got %d", resp.TokensUsed)
	}
}

func TestDeepSeekProviderGenerate_WithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// メッセージリストにsystemメッセージが含まれているか確認
		messages, ok := reqBody["messages"].([]interface{})
		if !ok || len(messages) == 0 {
			t.Error("Request should contain messages")
		}

		firstMsg := messages[0].(map[string]interface{})
		if firstMsg["role"] != "system" {
			t.Errorf("First message should be system, got '%v'", firstMsg["role"])
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "System prompt applied",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 10},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewDeepSeekProvider("test-api-key", "deepseek-chat")
	provider.SetBaseURL(server.URL)

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: "テスト"},
		},
		SystemPrompt: "You are a code architect",
	}

	_, err := provider.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("Generate with system prompt failed: %v", err)
	}
}

func TestDeepSeekProviderGenerate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit exceeded",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewDeepSeekProvider("test-api-key", "deepseek-chat")
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
