package line

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	orchestrator     Orchestrator
	channelSecret    string
	sender           *MessageSender
	mediaDownloader  *MediaDownloader
	botUserID        string // Bot's LINE user ID for mention detection
}

// NewHandler は新しいHandlerを作成
func NewHandler(orch Orchestrator, channelSecret, accessToken string) *Handler {
	return &Handler{
		orchestrator:    orch,
		channelSecret:   channelSecret,
		sender:          NewMessageSender(accessToken),
		mediaDownloader: NewMediaDownloader(accessToken),
		botUserID:       "", // Set via SetBotUserID if needed
	}
}

// SetBotUserID sets the bot's user ID for mention detection in group chats
func (h *Handler) SetBotUserID(botUserID string) {
	h.botUserID = botUserID
}

// ServeHTTP はHTTPリクエストを処理
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[HTTP] %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

	if r.URL.Path == "/webhook" && r.Method == http.MethodPost {
		h.handleWebhook(w, r)
		return
	}

	http.NotFound(w, r)
}

// handleWebhook はLINE webhookを処理
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	// リクエストボディを読み取り（署名検証のため）
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 署名検証
	signature := r.Header.Get("X-Line-Signature")
	log.Printf("[Webhook] Body length: %d, Signature present: %v, Secret length: %d",
		len(body), signature != "", len(h.channelSecret))
	if !verifySignature(body, signature, h.channelSecret) {
		log.Printf("[Webhook] Signature verification FAILED")
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}
	log.Printf("[Webhook] Signature verified OK, events parsing...")

	// リクエストボディをパース
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	log.Printf("[Webhook] Events count: %d", len(payload.Events))
	for i, ev := range payload.Events {
		log.Printf("[Webhook] Event[%d]: type=%s, msg_type=%s, source=%s, text=%q",
			i, ev.Type, ev.Message.Type, ev.Source.Type, ev.Message.Text)
	}

	// 即座に200を返し、イベント処理はバックグラウンドで実行
	// （LINE公式推奨: 2秒以内にレスポンスを返す）
	w.WriteHeader(http.StatusOK)

	// イベントをバックグラウンドで処理
	for _, event := range payload.Events {
		// メッセージイベントのみ処理
		if event.Type != "message" {
			continue
		}

		// テキストメッセージのみ処理
		if event.Message.Type != "text" {
			continue
		}

		// Group/Room chatの場合、Bot mentionチェック
		if h.botUserID != "" && event.Source.Type != "user" {
			var mentionees []Mentionee
			if event.Message.Mention != nil {
				mentionees = event.Message.Mention.Mentionees
			}
			if !isBotMention(event.Source.Type, mentionees, h.botUserID) {
				// Bot mentionがない場合はスキップ
				continue
			}
		}

		go h.processEvent(event)
	}
}

// processEvent はイベントをバックグラウンドで処理（HTTPコンテキストから独立）
func (h *Handler) processEvent(event WebhookEvent) {
	ctx := context.Background()

	// セッションID生成
	sessionID := h.generateSessionID(event.Source.UserID)

	// オーケストレータを呼び出し
	req := orchestrator.ProcessMessageRequest{
		SessionID:   sessionID,
		Channel:     "line",
		ChatID:      event.Source.UserID,
		UserMessage: event.Message.Text,
	}

	resp, err := h.orchestrator.ProcessMessage(ctx, req)
	if err != nil {
		log.Printf("[Webhook] Error processing message: %v", err)
		return
	}

	// Quote token取得
	quoteToken := extractQuoteToken(event)

	// LINE返信API呼び出し（quote token対応）
	var sendErr error
	if quoteToken != "" {
		sendErr = h.sender.SendReplyMessageWithQuote(ctx, event.ReplyToken, resp.Response, quoteToken)
	} else {
		sendErr = h.sender.SendReplyMessage(ctx, event.ReplyToken, resp.Response)
	}

	if sendErr != nil {
		log.Printf("[Webhook] Failed to send reply: %v", sendErr)
	} else {
		log.Printf("[Webhook] Reply sent successfully for session %s", sessionID)
	}
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
	Type       string      `json:"type"`
	Text       string      `json:"text"`
	ID         string      `json:"id"`
	QuoteToken string      `json:"quoteToken"`
	Mention    *Mention    `json:"mention,omitempty"`
}

// Mention はメンション情報
type Mention struct {
	Mentionees []Mentionee `json:"mentionees"`
}

// Mentionee はメンション対象ユーザー
type Mentionee struct {
	Index  int    `json:"index"`
	Length int    `json:"length"`
	UserID string `json:"userId"`
}

// EventSource はイベントソース
type EventSource struct {
	Type    string `json:"type"`
	UserID  string `json:"userId"`
	GroupID string `json:"groupId,omitempty"`
	RoomID  string `json:"roomId,omitempty"`
}
