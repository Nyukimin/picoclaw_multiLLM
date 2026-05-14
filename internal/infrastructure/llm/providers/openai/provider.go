package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

const defaultBaseURL = "https://api.openai.com"

// OpenAIProvider はOpenAI APIプロバイダーの実装
type OpenAIProvider struct {
	apiKey         string
	model          string
	baseURL        string
	thinkingBridge bool
	client         *http.Client
}

// NewOpenAIProvider は新しいOpenAIProviderを作成
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	return NewOpenAIProviderWithOptions(apiKey, model, defaultBaseURL, 120*time.Second)
}

// NewOpenAIProviderWithOptions creates an OpenAI-compatible provider with custom endpoint and timeout.
func NewOpenAIProviderWithOptions(apiKey, model, baseURL string, timeout time.Duration) *OpenAIProvider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAIProvider{
		apiKey:         apiKey,
		model:          model,
		baseURL:        baseURL,
		thinkingBridge: strings.TrimRight(baseURL, "/") != defaultBaseURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetBaseURL はベースURLを設定（テスト用）
func (p *OpenAIProvider) SetBaseURL(url string) {
	p.baseURL = url
}

// Generate はLLM生成を実行
func (p *OpenAIProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	streaming := req.OnToken != nil

	// OpenAI APIリクエスト構築
	openaiReq := map[string]interface{}{
		"model":    p.model,
		"messages": p.convertMessages(req),
	}
	p.addThinkingBridgeFields(openaiReq, streaming)

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
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

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

	if streaming {
		return p.readChatCompletionsStream(resp.Body, req.OnToken)
	}

	// レスポンスパース
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Role        string `json:"role"`
				Content     string `json:"content"`
				ParseStatus string `json:"parse_status"`
				ParserName  string `json:"parser_name"`
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
		msg := openaiResp.Choices[0].Message
		content = p.sanitizeThinkingBridgeContent(msg.Content, msg.ParseStatus, msg.ParserName)
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

// Chat はtool calling対応のチャットを実行（OpenAI /v1/chat/completions + tools）
func (p *OpenAIProvider) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	messages := p.convertChatMessages(req.Messages)

	openaiReq := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	p.addThinkingBridgeFields(openaiReq, false)
	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(req.Tools))
		for _, td := range req.Tools {
			tools = append(tools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        td.Function.Name,
					"description": td.Function.Description,
					"parameters":  td.Function.Parameters,
				},
			})
		}
		openaiReq["tools"] = tools
	}
	if req.Temperature > 0 {
		openaiReq["temperature"] = req.Temperature
	}

	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to marshal chat request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to execute chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return llm.ChatResponse{}, fmt.Errorf("openai chat API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	return p.parseChatResponse(resp.Body)
}

func (p *OpenAIProvider) addThinkingBridgeFields(req map[string]interface{}, streaming bool) {
	if !p.thinkingBridge {
		return
	}
	req["parse_reasoning"] = true
	req["include_reasoning"] = false
	req["separate_reasoning"] = true
	if streaming {
		req["stream"] = true
	}
}

func (p *OpenAIProvider) readChatCompletionsStream(body io.Reader, onToken llm.StreamCallback) (llm.GenerateResponse, error) {
	var full strings.Builder
	chunks := make([]string, 0, 16)
	var finishReason string
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return llm.GenerateResponse{}, fmt.Errorf("failed to decode stream chunk: %w", err)
		}
		for _, choice := range chunk.Choices {
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}
			if choice.Delta.Content == "" {
				continue
			}
			full.WriteString(choice.Delta.Content)
			chunks = append(chunks, choice.Delta.Content)
		}
	}
	if err := scanner.Err(); err != nil {
		return llm.GenerateResponse{}, fmt.Errorf("failed to read stream: %w", err)
	}
	if finishReason == "" {
		finishReason = "stop"
	}
	content := full.String()
	if p.thinkingBridge && looksLikeUntaggedReasoning(content) {
		content = extractFinalAnswerFromUntaggedReasoning(content)
		if content != "" {
			onToken(content)
		}
		return llm.GenerateResponse{
			Content:      content,
			FinishReason: finishReason,
		}, nil
	}
	for _, chunk := range chunks {
		onToken(chunk)
	}
	return llm.GenerateResponse{
		Content:      content,
		FinishReason: finishReason,
	}, nil
}

// convertChatMessages はChatMessageをOpenAI APIフォーマットに変換
func (p *OpenAIProvider) convertChatMessages(msgs []llm.ChatMessage) []map[string]interface{} {
	systemParts := make([]string, 0, 2)
	messages := make([]map[string]interface{}, 0, len(msgs))
	for _, m := range msgs {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") && strings.TrimSpace(m.Content) != "" && len(m.ToolCalls) == 0 && strings.TrimSpace(m.ToolCallID) == "" {
			systemParts = append(systemParts, strings.TrimSpace(m.Content))
			continue
		}
		msg := map[string]interface{}{
			"role": m.Role,
		}
		if m.Content != "" {
			msg["content"] = m.Content
		}
		if len(m.ToolCalls) > 0 {
			tcs := make([]map[string]interface{}, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Function.Arguments)
				tcs = append(tcs, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Function.Name,
						"arguments": string(argsJSON),
					},
				})
			}
			msg["tool_calls"] = tcs
		}
		if m.ToolCallID != "" {
			msg["tool_call_id"] = m.ToolCallID
		}
		messages = append(messages, msg)
	}
	if len(systemParts) > 0 {
		messages = append([]map[string]interface{}{
			{
				"role":    "system",
				"content": strings.Join(systemParts, "\n\n"),
			},
		}, messages...)
	}
	return messages
}

// parseChatResponse はOpenAI chat completionsレスポンスをパースする
func (p *OpenAIProvider) parseChatResponse(body io.Reader) (llm.ChatResponse, error) {
	var openaiResp struct {
		Choices []struct {
			Message struct {
				Role        string `json:"role"`
				Content     string `json:"content"`
				ParseStatus string `json:"parse_status"`
				ParserName  string `json:"parser_name"`
				ToolCalls   []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}

	if err := json.NewDecoder(body).Decode(&openaiResp); err != nil {
		return llm.ChatResponse{}, fmt.Errorf("failed to decode chat response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return llm.ChatResponse{}, fmt.Errorf("empty choices in response")
	}

	choice := openaiResp.Choices[0]
	result := llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    choice.Message.Role,
			Content: p.sanitizeThinkingBridgeContent(choice.Message.Content, choice.Message.ParseStatus, choice.Message.ParserName),
		},
		Done:         true,
		FinishReason: choice.FinishReason,
	}

	for _, tc := range choice.Message.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		result.Message.ToolCalls = append(result.Message.ToolCalls, llm.ToolCall{
			ID: tc.ID,
			Function: llm.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: args,
			},
		})
	}

	if len(result.Message.ToolCalls) > 0 && result.FinishReason == "" {
		result.FinishReason = "tool_calls"
	}

	return result, nil
}

func (p *OpenAIProvider) sanitizeThinkingBridgeContent(content, parseStatus, _ string) string {
	if !p.thinkingBridge {
		return content
	}
	if strings.TrimSpace(parseStatus) != "no_reasoning" {
		return content
	}
	if !looksLikeUntaggedReasoning(content) {
		return content
	}
	if final := extractFinalAnswerFromUntaggedReasoning(content); final != "" {
		return final
	}
	return ""
}

func looksLikeUntaggedReasoning(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	startsLikeReasoning := strings.HasPrefix(lower, "okay,") ||
		strings.HasPrefix(lower, "ok,") ||
		strings.HasPrefix(lower, "let me ") ||
		strings.HasPrefix(lower, "we need ") ||
		strings.HasPrefix(lower, "i need ") ||
		strings.HasPrefix(lower, "i should ") ||
		strings.HasPrefix(lower, "the user ")
	if !startsLikeReasoning {
		return false
	}
	markers := []string{
		"the user",
		"they wrote",
		"the query",
		"let me",
		"i need to",
		"i should",
		"translates to",
		"asking for",
		"want me to",
		"need to respond",
		"final answer",
	}
	hits := 0
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			hits++
		}
	}
	return hits >= 2
}

func extractFinalAnswerFromUntaggedReasoning(s string) string {
	candidates := []string{
		"Final answer:",
		"Final Answer:",
		"final answer:",
		"最終回答:",
		"最終回答：",
		"回答:",
		"回答：",
	}
	for _, marker := range candidates {
		if idx := strings.LastIndex(s, marker); idx >= 0 {
			return strings.TrimSpace(s[idx+len(marker):])
		}
	}
	return ""
}

// convertMessages はドメインメッセージをOpenAI APIフォーマットに変換
func (p *OpenAIProvider) convertMessages(req llm.GenerateRequest) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0)
	systemParts := make([]string, 0, 2)

	// システムプロンプトを最初に追加
	if req.SystemPrompt != "" {
		systemParts = append(systemParts, req.SystemPrompt)
	}

	// ユーザーメッセージを追加
	for _, msg := range req.Messages {
		if msg.Role == "system" && len(msg.Parts) == 0 {
			if strings.TrimSpace(msg.Content) != "" {
				systemParts = append(systemParts, msg.Content)
			}
			continue
		}
		content := any(msg.Content)
		if len(msg.Parts) > 0 {
			parts := make([]map[string]interface{}, 0, len(msg.Parts))
			for _, part := range msg.Parts {
				switch part.Type {
				case llm.MessagePartImage:
					if len(part.Data) == 0 || part.MimeType == "" {
						continue
					}
					parts = append(parts, map[string]interface{}{
						"type": "image_url",
						"image_url": map[string]interface{}{
							"url": "data:" + part.MimeType + ";base64," + base64.StdEncoding.EncodeToString(part.Data),
						},
					})
				default:
					text := part.Text
					if text == "" {
						text = msg.Content
					}
					if text != "" {
						parts = append(parts, map[string]interface{}{"type": "text", "text": text})
					}
				}
			}
			if len(parts) > 0 {
				content = parts
			}
		}
		messages = append(messages, map[string]interface{}{
			"role":    msg.Role,
			"content": content,
		})
	}

	if len(systemParts) > 0 {
		systemMessage := map[string]interface{}{
			"role":    "system",
			"content": strings.Join(systemParts, "\n\n"),
		}
		messages = append([]map[string]interface{}{systemMessage}, messages...)
	}

	return messages
}
