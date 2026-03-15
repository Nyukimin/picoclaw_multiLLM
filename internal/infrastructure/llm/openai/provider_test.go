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

// --- Chat (tool calling) テスト ---

func TestChat_WithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}

		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		// tools が送信されていることを確認
		tools, ok := reqBody["tools"].([]interface{})
		if !ok || len(tools) == 0 {
			t.Error("expected tools in request")
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role": "assistant",
						"tool_calls": []map[string]interface{}{
							{
								"id":   "call_abc123",
								"type": "function",
								"function": map[string]interface{}{
									"name":      "web_search",
									"arguments": `{"query":"RenCrow"}`,
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 50},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
	provider.SetBaseURL(server.URL)

	resp, err := provider.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "user", Content: "RenCrowを検索して"},
		},
		Tools: []llm.ToolDefinition{
			{
				Type: "function",
				Function: llm.ToolFunctionDef{
					Name:        "web_search",
					Description: "Web検索を実行",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
	})

	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason=tool_calls, got %s", resp.FinishReason)
	}
	if len(resp.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Message.ToolCalls))
	}
	tc := resp.Message.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("expected ID=call_abc123, got %s", tc.ID)
	}
	if tc.Function.Name != "web_search" {
		t.Errorf("expected tool name=web_search, got %s", tc.Function.Name)
	}
	if tc.Function.Arguments["query"] != "RenCrow" {
		t.Errorf("expected query=RenCrow, got %v", tc.Function.Arguments["query"])
	}
}

func TestChat_WithoutToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "こんにちは！",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 10},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
	provider.SetBaseURL(server.URL)

	resp, err := provider.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "user", Content: "こんにちは"},
		},
	})

	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason=stop, got %s", resp.FinishReason)
	}
	if resp.Message.Content != "こんにちは！" {
		t.Errorf("expected content=こんにちは！, got %s", resp.Message.Content)
	}
	if len(resp.Message.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(resp.Message.ToolCalls))
	}
}

func TestChat_ToolResultRoundtrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		json.NewDecoder(r.Body).Decode(&reqBody)

		msgs := reqBody["messages"].([]interface{})
		// system, user, assistant(tool_calls), tool, の4メッセージを期待
		if len(msgs) != 4 {
			t.Errorf("expected 4 messages, got %d", len(msgs))
		}

		// tool メッセージの検証
		toolMsg := msgs[3].(map[string]interface{})
		if toolMsg["role"] != "tool" {
			t.Errorf("expected role=tool, got %v", toolMsg["role"])
		}
		if toolMsg["tool_call_id"] != "call_1" {
			t.Errorf("expected tool_call_id=call_1, got %v", toolMsg["tool_call_id"])
		}

		response := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "検索結果はこちらです。",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{"total_tokens": 30},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
	provider.SetBaseURL(server.URL)

	resp, err := provider.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "検索して"},
			{Role: "assistant", ToolCalls: []llm.ToolCall{
				{ID: "call_1", Function: llm.ToolCallFunction{Name: "web_search", Arguments: map[string]any{"query": "test"}}},
			}},
			{Role: "tool", Content: "result data", ToolCallID: "call_1"},
		},
	})

	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp.Message.Content != "検索結果はこちらです。" {
		t.Errorf("expected final answer, got %s", resp.Message.Content)
	}
}

func TestChat_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded"}}`))
	}))
	defer server.Close()

	provider := NewOpenAIProvider("test-api-key", "gpt-4")
	provider.SetBaseURL(server.URL)

	_, err := provider.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.ChatMessage{{Role: "user", Content: "test"}},
	})

	if err == nil {
		t.Error("expected error for 429 response")
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
