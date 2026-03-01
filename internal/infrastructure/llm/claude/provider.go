package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

const defaultBaseURL = "https://api.anthropic.com"
const anthropicVersion = "2023-06-01"

// ClaudeProvider はClaude APIプロバイダーの実装
type ClaudeProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewClaudeProvider は新しいClaudeProviderを作成
func NewClaudeProvider(apiKey, model string) *ClaudeProvider {
	return &ClaudeProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// SetBaseURL はベースURLを設定（テスト用）
func (p *ClaudeProvider) SetBaseURL(url string) {
	p.baseURL = url
}

// Generate はLLM生成を実行
func (p *ClaudeProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	// Claude APIリクエスト構築
	claudeReq := map[string]interface{}{
		"model":      p.model,
		"messages":   p.convertMessages(req.Messages),
		"max_tokens": req.MaxTokens,
	}

	// システムプロンプト
	if req.SystemPrompt != "" {
		claudeReq["system"] = req.SystemPrompt
	}

	// Temperature（0.0-1.0の範囲）
	if req.Temperature > 0 {
		claudeReq["temperature"] = req.Temperature
	}

	reqBody, err := json.Marshal(claudeReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// HTTPリクエスト作成
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	// リクエスト実行
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.GenerateResponse{}, fmt.Errorf("claude API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// レスポンスパース
	var claudeResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&claudeResp); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	// コンテンツ抽出
	var content string
	if len(claudeResp.Content) > 0 && claudeResp.Content[0].Type == "text" {
		content = claudeResp.Content[0].Text
	}

	return llm.GenerateResponse{
		Content:      content,
		TokensUsed:   claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		FinishReason: claudeResp.StopReason,
	}, nil
}

// Name はプロバイダー名を返す
func (p *ClaudeProvider) Name() string {
	return fmt.Sprintf("claude-%s", p.model)
}

// convertMessages はドメインメッセージをClaude APIフォーマットに変換
func (p *ClaudeProvider) convertMessages(messages []llm.Message) []map[string]interface{} {
	claudeMessages := make([]map[string]interface{}, 0, len(messages))

	for _, msg := range messages {
		// Claude APIはsystemロールをサポートしないため、systemはトップレベルで渡す
		if msg.Role == "system" {
			continue
		}

		claudeMessages = append(claudeMessages, map[string]interface{}{
			"role": msg.Role,
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": msg.Content,
				},
			},
		})
	}

	return claudeMessages
}
