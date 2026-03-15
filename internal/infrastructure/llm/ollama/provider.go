package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// OllamaProvider はOllama APIプロバイダーの実装
type OllamaProvider struct {
	baseURL string
	model   string
	numCtx  int
	client  *http.Client
}

// NewOllamaProvider は新しいOllamaProviderを作成
func NewOllamaProvider(baseURL, model string) *OllamaProvider {
	return NewOllamaProviderWithNumCtx(baseURL, model, 0)
}

// NewOllamaProviderWithNumCtx は num_ctx を明示した OllamaProvider を作成
func NewOllamaProviderWithNumCtx(baseURL, model string, numCtx int) *OllamaProvider {
	return &OllamaProvider{
		baseURL: baseURL,
		model:   model,
		numCtx:  numCtx,
		client: &http.Client{
			Timeout: 120 * time.Second, // Ollamaは遅い場合があるため長めに設定
		},
	}
}

// Generate はLLM生成を実行（OnToken が設定されていればストリーミング）
func (p *OllamaProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	// プロンプト構築
	prompt := p.buildPrompt(req)

	streaming := req.OnToken != nil

	if err := p.ensureModelReady(ctx, p.model); err != nil {
		return llm.GenerateResponse{}, err
	}

	// Ollama APIリクエスト
	ollamaReq := map[string]interface{}{
		"model":      p.model,
		"prompt":     prompt,
		"stream":     streaming,
		"keep_alive": -1,
		"options": map[string]interface{}{
			"temperature": req.Temperature,
			"num_predict": req.MaxTokens,
			"stop":        []string{},
		},
	}
	if p.numCtx > 0 {
		ollamaReq["options"].(map[string]interface{})["num_ctx"] = p.numCtx
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

	// ストリーミング時はタイムアウトなしの別クライアントを使用
	client := p.client
	if streaming {
		client = &http.Client{} // no timeout for streaming
	}

	// リクエスト実行
	resp, err := client.Do(httpReq)
	if err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.GenerateResponse{}, fmt.Errorf("ollama API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	// ストリーミング
	if streaming {
		return p.readStream(resp.Body, req.OnToken)
	}

	// 非ストリーミング（従来通り）
	var ollamaResp struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to decode response: %w", err)
	}

	return llm.GenerateResponse{
		Content:      ollamaResp.Response,
		TokensUsed:   0,
		FinishReason: "stop",
	}, nil
}

// readStream は Ollama の NDJSON ストリームを読み込む
func (p *OllamaProvider) readStream(body io.Reader, onToken llm.StreamCallback) (llm.GenerateResponse, error) {
	var full strings.Builder
	decoder := json.NewDecoder(body)

	for {
		var chunk struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return llm.GenerateResponse{}, fmt.Errorf("failed to decode stream chunk: %w", err)
		}

		if chunk.Response != "" {
			full.WriteString(chunk.Response)
			onToken(chunk.Response)
		}

		if chunk.Done {
			break
		}
	}

	return llm.GenerateResponse{
		Content:      full.String(),
		TokensUsed:   0,
		FinishReason: "stop",
	}, nil
}

// Name はプロバイダー名を返す
func (p *OllamaProvider) Name() string {
	return fmt.Sprintf("ollama-%s", p.model)
}

// Chat はtool calling対応のチャットを実行（Ollama /api/chat）
func (p *OllamaProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	if err := p.ensureModelReady(ctx, model); err != nil {
		return llm.ChatResponse{}, err
	}

	// メッセージ変換
	messages := make([]ollamaChatMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msg := ollamaChatMessage{
			Role:    m.Role,
			Content: m.Content,
		}
		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, ollamaToolCall{
					Function: ollamaToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
		messages = append(messages, msg)
	}

	// ツール定義変換
	var tools []ollamaToolDef
	for _, td := range req.Tools {
		tools = append(tools, ollamaToolDef{
			Type: td.Type,
			Function: ollamaFunctionDef{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				Parameters:  td.Function.Parameters,
			},
		})
	}

	chatReq := ollamaChatRequest{
		Model:     model,
		Messages:  messages,
		Tools:     tools,
		Stream:    false,
		KeepAlive: -1,
	}
	if req.Temperature > 0 {
		chatReq.Options = &ollamaChatOptions{Temperature: req.Temperature}
	}
	if p.numCtx > 0 {
		if chatReq.Options == nil {
			chatReq.Options = &ollamaChatOptions{}
		}
		chatReq.Options.NumCtx = p.numCtx
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to execute chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.ChatResponse{}, fmt.Errorf("ollama chat API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var chatResp ollamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to decode chat response: %w", err)
	}

	// レスポンス変換
	result := llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    chatResp.Message.Role,
			Content: chatResp.Message.Content,
		},
		Done: chatResp.Done,
	}

	if len(chatResp.Message.ToolCalls) > 0 {
		result.FinishReason = "tool_calls"
		for i, tc := range chatResp.Message.ToolCalls {
			id := tc.Function.Name
			if id == "" {
				id = fmt.Sprintf("call_%d", i)
			} else {
				id = fmt.Sprintf("call_%s_%d", tc.Function.Name, i)
			}
			result.Message.ToolCalls = append(result.Message.ToolCalls, llm.ToolCall{
				ID: id,
				Function: llm.ToolCallFunction{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
	} else {
		result.FinishReason = "stop"
	}

	return result, nil
}

// --- Ollama /api/chat 用の内部型 ---

type ollamaChatRequest struct {
	Model     string             `json:"model"`
	Messages  []ollamaChatMessage `json:"messages"`
	Tools     []ollamaToolDef    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
	KeepAlive int                `json:"keep_alive"`
	Options   *ollamaChatOptions `json:"options,omitempty"`
}

type ollamaChatOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumCtx      int     `json:"num_ctx,omitempty"`
}

type ollamaChatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

type ollamaToolCall struct {
	Function ollamaToolCallFunction `json:"function"`
}

type ollamaToolCallFunction struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type ollamaToolDef struct {
	Type     string          `json:"type"`
	Function ollamaFunctionDef `json:"function"`
}

type ollamaFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ollamaChatResponse struct {
	Model   string            `json:"model"`
	Message ollamaChatMessage `json:"message"`
	Done    bool              `json:"done"`
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

func (p *OllamaProvider) ensureModelReady(ctx context.Context, model string) error {
	log.Printf("[OllamaProvider] preflight start model=%s num_ctx=%d", model, p.numCtx)
	loaded, err := p.isModelLoaded(ctx, model)
	if err != nil {
		return fmt.Errorf("ollama model health check failed for %s: %w", model, err)
	}
	if loaded {
		log.Printf("[OllamaProvider] preflight ready model=%s source=resident", model)
		return nil
	}
	log.Printf("[OllamaProvider] preflight warmup model=%s", model)
	if err := p.warmModel(ctx, model); err != nil {
		return fmt.Errorf("ollama model warmup failed for %s: %w", model, err)
	}
	log.Printf("[OllamaProvider] preflight ready model=%s source=warmup", model)
	return nil
}

func (p *OllamaProvider) isModelLoaded(ctx context.Context, model string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/ps", nil)
	if err != nil {
		return false, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("status=%d body=%s", resp.StatusCode, string(body))
	}
	var psResp ollamaPsResponse
	if err := json.NewDecoder(resp.Body).Decode(&psResp); err != nil {
		return false, err
	}
	for _, m := range psResp.Models {
		if strings.TrimSpace(m.Name) == strings.TrimSpace(model) {
			return true, nil
		}
	}
	return false, nil
}

func (p *OllamaProvider) warmModel(ctx context.Context, model string) error {
	options := map[string]interface{}{
		"temperature": 0,
		"num_predict": 0,
		"stop":        []string{},
	}
	if p.numCtx > 0 {
		options["num_ctx"] = p.numCtx
	}
	body, err := json.Marshal(map[string]interface{}{
		"model":      model,
		"prompt":     "",
		"stream":     false,
		"keep_alive": -1,
		"options":    options,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return nil
}

type ollamaPsResponse struct {
	Models []struct {
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
	} `json:"models"`
}
