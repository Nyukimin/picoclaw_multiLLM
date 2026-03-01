package deepseek

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

const defaultBaseURL = "https://api.deepseek.com"

// DeepSeekProvider はDeepSeek APIプロバイダーの実装
// DeepSeek APIはOpenAI互換のため、実装はOpenAIProviderと類似
type DeepSeekProvider struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewDeepSeekProvider は新しいDeepSeekProviderを作成
func NewDeepSeekProvider(apiKey, model string) *DeepSeekProvider {
	return &DeepSeekProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: defaultBaseURL,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// SetBaseURL はベースURLを設定（テスト用）
func (p *DeepSeekProvider) SetBaseURL(url string) {
	p.baseURL = url
}

// Generate はLLM生成を実行
func (p *DeepSeekProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	// DeepSeek APIリクエスト構築（OpenAI互換）
	deepseekReq := map[string]interface{}{
		"model":    p.model,
		"messages": p.convertMessages(req),
	}

	// MaxTokens
	if req.MaxTokens > 0 {
		deepseekReq["max_tokens"] = req.MaxTokens
	}

	// Temperature
	if req.Temperature > 0 {
		deepseekReq["temperature"] = req.Temperature
	}

	reqBody, err := json.Marshal(deepseekReq)
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
		return llm.GenerateResponse{}, fmt.Errorf("deepseek API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// レスポンスパース（OpenAI互換）
	var deepseekResp struct {
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

	if err := json.NewDecoder(resp.Body).Decode(&deepseekResp); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	// コンテンツ抽出
	var content string
	var finishReason string
	if len(deepseekResp.Choices) > 0 {
		content = deepseekResp.Choices[0].Message.Content
		finishReason = deepseekResp.Choices[0].FinishReason
	}

	return llm.GenerateResponse{
		Content:      content,
		TokensUsed:   deepseekResp.Usage.TotalTokens,
		FinishReason: finishReason,
	}, nil
}

// Name はプロバイダー名を返す
func (p *DeepSeekProvider) Name() string {
	return fmt.Sprintf("deepseek-%s", p.model)
}

// convertMessages はドメインメッセージをDeepSeek APIフォーマットに変換
func (p *DeepSeekProvider) convertMessages(req llm.GenerateRequest) []map[string]interface{} {
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
