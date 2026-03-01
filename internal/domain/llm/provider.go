package llm

import "context"

// Message はLLMメッセージを表す
type Message struct {
	Role    string // "user", "assistant", "system"
	Content string
}

// GenerateRequest はLLM生成リクエスト
type GenerateRequest struct {
	Messages     []Message
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
}

// GenerateResponse はLLM生成レスポンス
type GenerateResponse struct {
	Content      string
	TokensUsed   int
	FinishReason string
}

// LLMProvider はLLMプロバイダーの抽象化
type LLMProvider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
	Name() string
}
