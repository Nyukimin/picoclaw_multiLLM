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

// --- Tool Calling 対応型 ---

// ChatMessage はツール呼び出し対応メッセージ
type ChatMessage struct {
	Role       string     // "system", "user", "assistant", "tool"
	Content    string
	ToolCalls  []ToolCall // role="assistant" 時のツール呼び出し
	ToolCallID string     // role="tool" 時の対応ID
}

// ToolCall はLLMが返すツール呼び出し
type ToolCall struct {
	ID       string
	Function ToolCallFunction
}

// ToolCallFunction はツール呼び出しの関数情報
type ToolCallFunction struct {
	Name      string
	Arguments map[string]any
}

// ToolDefinition はLLMに渡すツール定義
type ToolDefinition struct {
	Type     string          // "function"
	Function ToolFunctionDef
}

// ToolFunctionDef はツール関数の定義
type ToolFunctionDef struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema
}

// ChatRequest はtool calling対応チャットリクエスト
type ChatRequest struct {
	Model       string
	Messages    []ChatMessage
	Tools       []ToolDefinition
	Temperature float64
}

// ChatResponse はtool calling対応チャットレスポンス
type ChatResponse struct {
	Message      ChatMessage
	Done         bool
	FinishReason string // "stop" or "tool_calls"
}

// ToolCallingProvider はtool calling対応のLLMプロバイダー
type ToolCallingProvider interface {
	LLMProvider
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}
