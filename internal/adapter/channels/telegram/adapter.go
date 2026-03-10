package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	channelapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/channel"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

// Orchestrator is the minimal processor required by Telegram adapter.
type Orchestrator interface {
	ProcessMessage(ctx context.Context, req orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error)
}

// Adapter handles Telegram webhook and outbound sends.
type Adapter struct {
	botToken      string
	webhookSecret string
	orchestrator  Orchestrator
	httpClient    *http.Client
	apiBaseURL    string
}

func NewAdapter(botToken string, orch ...Orchestrator) *Adapter {
	var o Orchestrator
	if len(orch) > 0 {
		o = orch[0]
	}
	return &Adapter{
		botToken:     botToken,
		orchestrator: o,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		apiBaseURL:   "https://api.telegram.org",
	}
}

func (a *Adapter) Name() string { return "telegram" }

func (a *Adapter) SetHTTPClient(c *http.Client) {
	if c != nil {
		a.httpClient = c
	}
}

func (a *Adapter) SetAPIBaseURL(url string) {
	if url != "" {
		a.apiBaseURL = url
	}
}

func (a *Adapter) SetWebhookSecret(secret string) {
	a.webhookSecret = secret
}

func (a *Adapter) Send(ctx context.Context, chatID, text string) error {
	if a.botToken == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	if chatID == "" {
		return fmt.Errorf("chatID is required")
	}
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	b, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/bot%s/sendMessage", a.apiBaseURL, a.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram sendMessage failed: status=%d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) Probe(ctx context.Context) error {
	if a.botToken == "" {
		return fmt.Errorf("telegram bot token is not configured")
	}
	url := fmt.Sprintf("%s/bot%s/getMe", a.apiBaseURL, a.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("telegram getMe failed: status=%d", resp.StatusCode)
	}
	return nil
}

func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.orchestrator == nil {
		http.Error(w, "orchestrator is not configured", http.StatusServiceUnavailable)
		return
	}
	if a.webhookSecret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != a.webhookSecret {
			http.Error(w, "invalid secret token", http.StatusUnauthorized)
			return
		}
	}
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if update.Message == nil || update.Message.Text == "" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
		return
	}

	req := orchestrator.ProcessMessageRequest{
		SessionID:   channelapp.BuildSessionID(time.Now().UTC(), "telegram", strconv.FormatInt(update.Message.Chat.ID, 10)),
		Channel:     "telegram",
		ChatID:      strconv.FormatInt(update.Message.Chat.ID, 10),
		UserMessage: update.Message.Text,
	}
	resp, err := a.orchestrator.ProcessMessage(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.Send(r.Context(), strconv.FormatInt(update.Message.Chat.ID, 10), resp.Response); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// Telegram webhook payload
type Update struct {
	UpdateID int64          `json:"update_id"`
	Message  *UpdateMessage `json:"message,omitempty"`
}

type UpdateMessage struct {
	MessageID int64      `json:"message_id"`
	Text      string     `json:"text"`
	Chat      UpdateChat `json:"chat"`
	From      UpdateUser `json:"from"`
}

type UpdateChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type UpdateUser struct {
	ID int64 `json:"id"`
}
