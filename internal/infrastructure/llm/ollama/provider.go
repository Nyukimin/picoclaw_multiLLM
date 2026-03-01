package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// OllamaProvider はOllama APIプロバイダーの実装
type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaProvider は新しいOllamaProviderを作成
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		client: &http.Client{
			Timeout: 120 * time.Second, // Ollamaは遅い場合があるため長めに設定
		},
	}
}

// Generate はLLM生成を実行
func (p *OllamaProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	// プロンプト構築
	prompt := p.buildPrompt(req)

	// Ollama APIリクエスト
	ollamaReq := map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]interface{}{
			"temperature":   req.Temperature,
			"num_predict":   req.MaxTokens,
			"stop":          []string{},
		},
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// HTTPリクエスト作成
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(reqBody))
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// リクエスト実行
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.GenerateResponse{}, fmt.Errorf("ollama API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// レスポンスパース
	var ollamaResp struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return llm.GenerateResponse{
		Content:      ollamaResp.Response,
		TokensUsed:   0, // Ollamaは簡易APIでトークン数を返さない
		FinishReason: "stop",
	}, nil
}

// Name はプロバイダー名を返す
func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama-%s", p.model)
}

// buildPrompt はメッセージリストからプロンプトを構築
func (p *OllamaProvider) buildPrompt(req llm.GenerateRequest) string {
	var parts []string

	// システムプロンプト
	if req.SystemPrompt != "" {
		parts = append(parts, fmt.Sprintf("System: %s\n", req.SystemPrompt))
	}

	// メッセージ履歴
	for _, msg := range req.Messages {
		switch msg.Role {
		case "user":
			parts = append(parts, fmt.Sprintf("User: %s", msg.Content))
		case "assistant":
			parts = append(parts, fmt.Sprintf("Assistant: %s", msg.Content))
		case "system":
			parts = append(parts, fmt.Sprintf("System: %s", msg.Content))
		}
	}

	return strings.Join(parts, "\n")
}
