package openai

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

const defaultBaseURL = "https://api.openai.com"

// OpenAIProvider はOpenAI APIプロバイダーの実装
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenAIProvider は新しいOpenAIProviderを作成
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// SetBaseURL はベースURLを設定（テスト用）
func (p *OpenAIProvider) SetBaseURL(url string) {
	p.baseURL = url
}

// Generate はLLM生成を実行
func (p *OpenAIProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	// OpenAI APIリクエスト構築
	openaiReq := map[string]interface{}{
		"model":    p.model,
		"messages": p.convertMessages(req),
	}

	// MaxTokens（OpenAIではmax_tokens）
	if req.MaxTokens > 0 {
		openaiReq["max_tokens"] = req.MaxTokens
	}

	// Temperature
	if req.Temperature > 0 {
		openaiReq["temperature"] = req.Temperature
	}

	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// HTTPリクエスト作成
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	// リクエスト実行
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.GenerateResponse{}, fmt.Errorf("openai API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// レスポンスパース
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	// コンテンツ抽出
	var content string
	var finishReason string
	if len(openaiResp.Choices) > 0 {
		content = openaiResp.Choices[0].Message.Content
		finishReason = openaiResp.Choices[0].FinishReason
	}

	return llm.GenerateResponse{
		Content:      content,
		TokensUsed:   openaiResp.Usage.TotalTokens,
		FinishReason: finishReason,
	}, nil
}

// Name はプロバイダー名を返す
func (p *OpenAIProvider) Name() string {
	return fmt.Sprintf("openai-%s", p.model)
}

// convertMessages はドメインメッセージをOpenAI APIフォーマットに変換
func (p *OpenAIProvider) convertMessages(req llm.GenerateRequest) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0)

	// システムプロンプトを最初に追加
	if req.SystemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    "system",
			"content": req.SystemPrompt,
		})
	}

	// ユーザーメッセージを追加
	for _, msg := range req.Messages {
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	return messages
}
