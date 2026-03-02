package line

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

// mockOrchestrator はテスト用のOrchestrator
type mockOrchestrator struct {
	response orchestrator.ProcessMessageResponse
	err      error
}

func (m *mockOrchestrator) ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error) {
	if m.err != nil {
		return orchestrator.ProcessMessageResponse{}, m.err
	}
	return m.response, nil
}

func TestNewHandler(t *testing.T) {
	orch := &mockOrchestrator{
		response: orchestrator.ProcessMessageResponse{
			Response: "test",
			Route:    routing.RouteCHAT,
		},
	}

	handler := NewHandler(orch, "test-channel-secret", "test-access-token")

	if handler == nil {
		t.Fatal("NewHandler should not return nil")
	}
}

func TestHandler_WebhookEndpoint_ValidMessage(t *testing.T) {
	orch := &mockOrchestrator{
		response: orchestrator.ProcessMessageResponse{
			Response:   "こんにちは！",
			Route:      routing.RouteCHAT,
			Confidence: 1.0,
			JobID:      "20260302-120000-abcd1234",
		},
	}

	handler := NewHandler(orch, "test-secret", "test-token")

	// LINE webhook payload
	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "text",
					"text": "こんにちは",
				},
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
				"replyToken": "test-reply-token",
			},
		},
	}

	body, _ := json.Marshal(payload)

	// Generate valid signature
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_InvalidJSON(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	body := []byte("invalid json")
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_NonMessageEvent(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	// フォローイベント（メッセージではない）
	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "follow",
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
			},
		},
	}

	body, _ := json.Marshal(payload)
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 非メッセージイベントは無視してOK返す
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_NonTextMessage(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	// 画像メッセージ
	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "image",
					"id":   "image123",
				},
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
				"replyToken": "test-reply-token",
			},
		},
	}

	body, _ := json.Marshal(payload)
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// 非テキストメッセージは無視してOK返す
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_GenerateSessionID(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		expected string // prefix check
	}{
		{
			name:     "Standard user ID",
			userID:   "U123456",
			expected: "20260302-line-U123456", // 日付部分は実行日により変わる
		},
		{
			name:     "Long user ID",
			userID:   "Uabcdefghijklmnop",
			expected: "20260302-line-Uabcdefghijklmnop",
		},
	}

	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID := handler.generateSessionID(tt.userID)

			// 形式チェック: YYYYMMDD-line-{userID}
			if len(sessionID) < len("YYYYMMDD-line-") {
				t.Errorf("Session ID too short: %s", sessionID)
			}

			// "line-"が含まれているか
			if !contains(sessionID, "line-") {
				t.Errorf("Session ID should contain 'line-': %s", sessionID)
			}

			// userIDが含まれているか
			if !contains(sessionID, tt.userID) {
				t.Errorf("Session ID should contain userID '%s': %s", tt.userID, sessionID)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHandler_HealthCheck(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}

	var response map[string]string
	json.Unmarshal(rec.Body.Bytes(), &response)

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%s'", response["status"])
	}
}

func TestHandler_WebhookEndpoint_InvalidSignature(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "text",
					"text": "Test",
				},
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
				"replyToken": "test-reply-token",
			},
		},
	}

	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", "invalid-signature")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_MissingSignature(t *testing.T) {
	orch := &mockOrchestrator{}
	handler := NewHandler(orch, "test-secret", "test-token")

	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "text",
					"text": "Test",
				},
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
				"replyToken": "test-reply-token",
			},
		},
	}

	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// No X-Line-Signature header

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401, got %d", rec.Code)
	}
}

// generateSignature generates HMAC-SHA256 signature for testing
func generateSignature(body []byte, channelSecret string) string {
	mac := hmac.New(sha256.New, []byte(channelSecret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestHandler_WebhookEndpoint_GroupChatWithBotMention(t *testing.T) {
	orch := &mockOrchestrator{
		response: orchestrator.ProcessMessageResponse{
			Response:   "グループチャット返信",
			Route:      routing.RouteCHAT,
			Confidence: 1.0,
			JobID:      "test-job-id",
		},
	}

	handler := NewHandler(orch, "test-secret", "test-token")
	handler.SetBotUserID("U-BOT123") // Set bot user ID

	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "text",
					"text": "@bot こんにちは",
					"mention": map[string]interface{}{
						"mentionees": []map[string]interface{}{
							{
								"index":  0,
								"length": 4,
								"userId": "U-BOT123",
							},
						},
					},
				},
				"source": map[string]interface{}{
					"type":    "group",
					"userId":  "U123456",
					"groupId": "G123456",
				},
				"replyToken": "reply-token-123",
			},
		},
	}

	body, _ := json.Marshal(payload)
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_GroupChatWithoutBotMention(t *testing.T) {
	orch := &mockOrchestrator{}

	handler := NewHandler(orch, "test-secret", "test-token")
	handler.SetBotUserID("U-BOT123")

	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type": "text",
					"text": "こんにちは",
					// No mention
				},
				"source": map[string]interface{}{
					"type":    "group",
					"userId":  "U123456",
					"groupId": "G123456",
				},
				"replyToken": "reply-token-123",
			},
		},
	}

	body, _ := json.Marshal(payload)
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Bot mention無しの場合はスキップされるが、webhookは成功
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}

func TestHandler_WebhookEndpoint_WithQuoteToken(t *testing.T) {
	orch := &mockOrchestrator{
		response: orchestrator.ProcessMessageResponse{
			Response:   "引用返信",
			Route:      routing.RouteCHAT,
			Confidence: 1.0,
			JobID:      "test-job-id",
		},
	}

	handler := NewHandler(orch, "test-secret", "test-token")

	payload := map[string]interface{}{
		"events": []map[string]interface{}{
			{
				"type": "message",
				"message": map[string]interface{}{
					"type":       "text",
					"text":       "返信します",
					"quoteToken": "quote-token-abc123",
				},
				"source": map[string]interface{}{
					"type":   "user",
					"userId": "U123456",
				},
				"replyToken": "reply-token-123",
			},
		},
	}

	body, _ := json.Marshal(payload)
	signature := generateSignature(body, "test-secret")

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Line-Signature", signature)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}
