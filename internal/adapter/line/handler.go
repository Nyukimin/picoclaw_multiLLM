package line

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

// Orchestrator はメッセージ処理のインターフェース
type Orchestrator interface {
	ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error)
}

// Handler はLINE webhookハンドラー
type Handler struct {
	orchestrator  Orchestrator
	channelSecret string
	accessToken   string
}

// NewHandler は新しいHandlerを作成
func NewHandler(orch Orchestrator, channelSecret, accessToken string) *Handler {
	return &Handler{
		orchestrator:  orch,
		channelSecret: channelSecret,
		accessToken:   accessToken,
	}
}

// ServeHTTP はHTTPリクエストを処理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// ルーティング
	if r.URL.Path == "/health" && r.Method == http.MethodGet {
		h.handleHealth(w, r)
		return
	}

	if r.URL.Path == "/webhook" && r.Method == http.MethodPost {
		h.handleWebhook(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleHealth はヘルスチェック
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleWebhook はLINE webhookを処理
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// リクエストボディをパース
	var payload WebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// イベントを処理
	for _, event := range payload.Events {
		// メッセージイベントのみ処理
		if event.Type != "message" {
			continue
		}

		// テキストメッセージのみ処理
		if event.Message.Type != "text" {
			continue
		}

		// セッションID生成
		sessionID := h.generateSessionID(event.Source.UserID)

		// オーケストレータを呼び出し
		req := orchestrator.ProcessMessageRequest{
			SessionID:   sessionID,
			Channel:     "line",
			ChatID:      event.Source.UserID,
			UserMessage: event.Message.Text,
		}

		resp, err := h.orchestrator.ProcessMessage(r.Context(), req)
		if err != nil {
			// エラーログ（本来はロガーを使用）
			fmt.Printf("Error processing message: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// LINE返信API呼び出し（簡易版：実際はLINE Messaging APIを使用）
		// ここでは200 OKを返すのみ
		// TODO: 実際のLINE返信API統合
		_ = resp
	}

	w.WriteHeader(http.StatusOK)
}

// generateSessionID はセッションIDを生成
func (h *Handler) generateSessionID(userID string) string {
	// フォーマット: YYYYMMDD-line-{userID}
	datePrefix := time.Now().Format("20060102")
	return fmt.Sprintf("%s-line-%s", datePrefix, userID)
}

// WebhookPayload はLINE webhookペイロード
type WebhookPayload struct {
	Events []WebhookEvent `json:"events"`
}

// WebhookEvent はLINE webhookイベント
type WebhookEvent struct {
	Type       string        `json:"type"`
	Message    EventMessage  `json:"message"`
	Source     EventSource   `json:"source"`
	ReplyToken string        `json:"replyToken"`
	Timestamp  int64         `json:"timestamp"`
}

// EventMessage はイベントメッセージ
type EventMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
	ID   string `json:"id"`
}

// EventSource はイベントソース
type EventSource struct {
	Type   string `json:"type"`
	UserID string `json:"userId"`
}
