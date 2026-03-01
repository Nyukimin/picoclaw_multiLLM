package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

func TestNewOpenAIProvider(t *testing.T) {
	provider := NewOpenAIProvider("test-api-key", "gpt-4")

	if provider == nil {
		t.Fatal("NewOpenAIProvider should not return nil")
	}

	if provider.Name() != "openai-gpt-4" {
		t.Errorf("Expected name 'openai-gpt-4', got '%s'", provider.Name())
	}
}

func TestOpenAIProviderGenerate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path '/v1/chat/completions', got '%s'", r.URL.Path)
		}

		// Authorizationヘッダー確認
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Expected 'Bearer test-api-key', got '%s'", auth)
		}

		// リクエストボディ検証
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody["model"] != "gpt-4" {
			t.Errorf("Expected model 'gpt-4', got '%v'", reqBody["model"])
		}

		// レスポンス
		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "gpt-4",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "こんにちは！お手伝いします。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
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

	if resp.Content != "こんにちは！お手伝いします。" {
		t.Errorf("Expected response content, got '%s'", resp.Content)
	}

	if resp.TokensUsed != 30 {
		t.Errorf("Expected 30 tokens used, got %d", resp.TokensUsed)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("Expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
}

func TestOpenAIProviderGenerate_WithSystemPrompt(t *testing.T) {
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

		if firstMsg["content"] != "You are a helpful assistant" {
			t.Errorf("Expected system content 'You are a helpful assistant', got '%v'", firstMsg["content"])
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "System prompt applied",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 15},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
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

func TestOpenAIProviderGenerate_MultipleMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		messages, ok := reqBody["messages"].([]interface{})
		if !ok || len(messages) != 3 { // user, assistant, user
			t.Errorf("Expected 3 messages, got %d", len(messages))
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Multi-turn response",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 50},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
	provider.SetBaseURL(server.URL)

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

func TestOpenAIProviderGenerate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Rate limit exceeded",
				"type":    "rate_limit_error",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
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

func TestOpenAIProviderGenerate_InvalidAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Incorrect API key provided",
				"type":    "invalid_request_error",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("invalid-key", "gpt-4")
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
