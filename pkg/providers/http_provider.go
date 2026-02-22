// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"encoding/base64"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type HTTPProvider struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

type imagePayloadAudit struct {
	SourcePath            string
	URLType               string
	ImageURL              string
	ImageURLLength        int
	Included              bool
	LocalExistsBefore     bool
	LocalSizeBeforeBytes  int64
	DropReason            string
	LocalExistsAfterTimer *bool
	LocalSizeAfterBytes   *int64
}

const maxInlineImageBytes = 5 * 1024 * 1024

func NewHTTPProvider(apiKey, apiBase, proxy string) *HTTPProvider {
	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	return &HTTPProvider{
		apiKey:     apiKey,
		apiBase:    strings.TrimRight(apiBase, "/"),
		httpClient: client,
	}
}

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	// Strip provider prefix from model name (e.g., moonshot/kimi-k2.5 -> kimi-k2.5, groq/openai/gpt-oss-120b -> openai/gpt-oss-120b, ollama/qwen2.5:14b -> qwen2.5:14b)
	if idx := strings.Index(model, "/"); idx != -1 {
		prefix := model[:idx]
		if prefix == "moonshot" || prefix == "nvidia" || prefix == "groq" || prefix == "ollama" {
			model = model[idx+1:]
		}
	}

	requestBody := map[string]interface{}{
		"model":    model,
		"messages": nil,
	}
	httpMessages, imageAudits := buildHTTPMessagesWithAudit(messages)
	requestBody["messages"] = httpMessages
	if len(imageAudits) > 0 {
		logger.InfoCF("provider.http", "LLM image audit before request", map[string]interface{}{
			"model":  model,
			"images": imageAuditLogEntries(imageAudits),
		})
	}

	// Ollama's OpenAI-compatible endpoint can default to very large context windows
	// (e.g., 131072), which may crash/timeout under multimodal load.
	// Set a bounded context to keep vision requests stable.
	// keep_alive: -1 keeps Chat/Worker models loaded (永続).
	if isOllamaEndpoint(p.apiBase) {
		requestBody["keep_alive"] = -1
		requestBody["options"] = map[string]interface{}{
			"num_ctx": 8192,
		}
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := options["max_tokens"].(int); ok {
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "o1") {
			requestBody["max_completion_tokens"] = maxTokens
		} else {
			requestBody["max_tokens"] = maxTokens
		}
	}

	if temperature, ok := options["temperature"].(float64); ok {
		lowerModel := strings.ToLower(model)
		// Kimi k2 models only support temperature=1
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			requestBody["temperature"] = 1.0
		} else {
			requestBody["temperature"] = temperature
		}
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	// メッセージがLLMに届いたことを保証するログ（送信直前にINFOで出力）
	lastUserContent := lastUserContentPreview(messages, 200)
	logger.InfoCF("provider.http", "LLM request sent",
		map[string]interface{}{
			"endpoint":           p.apiBase + "/chat/completions",
			"model":              model,
			"messages_count":     len(messages),
			"last_user_preview":  lastUserContent,
		})

	resp, err := p.httpClient.Do(req)
	if err != nil {
		isTimeout := isTimeoutError(err)
		logger.InfoCF("provider.http", "LLM request failed (no response received)",
			map[string]interface{}{
				"model":      model,
				"error":      err.Error(),
				"is_timeout": isTimeout,
				"note":       "Ollama may have responded but PicoClaw timed out before reading",
			})
		if len(imageAudits) > 0 && isTimeout {
			timeoutAudits := enrichAuditAfterTimeout(imageAudits)
			logger.WarnCF("provider.http", "LLM image audit after timeout", map[string]interface{}{
				"model":  model,
				"error":  err.Error(),
				"images": imageAuditLogEntries(timeoutAudits),
			})
		}
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.InfoCF("provider.http", "LLM response body read failed",
			map[string]interface{}{"model": model, "error": err.Error()})
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.InfoCF("provider.http", "LLM response non-OK",
			map[string]interface{}{
				"model":       model,
				"status_code": resp.StatusCode,
				"body_preview": truncateForLog(string(body), 200),
			})
		return nil, fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s", resp.StatusCode, string(body))
	}

	logger.InfoCF("provider.http", "LLM response received (HTTP 200)",
		map[string]interface{}{
			"model":     model,
			"body_len":  len(body),
		})

	llmResp, err := p.parseResponse(body)
	if err != nil {
		logger.InfoCF("provider.http", "LLM response parse failed",
			map[string]interface{}{
				"model":       model,
				"error":       err.Error(),
				"body_preview": truncateForLog(string(body), 300),
			})
		return nil, err
	}

	if llmResp.Content == "" && len(llmResp.ToolCalls) == 0 {
		logger.InfoCF("provider.http", "LLM response empty (no content, no tool_calls)",
			map[string]interface{}{"model": model, "finish_reason": llmResp.FinishReason})
	}
	logger.InfoCF("provider.http", "LLM response parsed",
		map[string]interface{}{
			"model":         model,
			"content_len":   len(llmResp.Content),
			"tool_calls":    len(llmResp.ToolCalls),
			"finish_reason": llmResp.FinishReason,
		})

	return llmResp, nil
}

// truncateForLog returns s truncated to maxLen with "..." suffix for safe logging.
func truncateForLog(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// lastUserContentPreview returns the content of the last user message, truncated for log.
func lastUserContentPreview(messages []Message, maxLen int) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && messages[i].Content != "" {
			s := strings.TrimSpace(messages[i].Content)
			if len(s) > maxLen {
				return s[:maxLen] + "..."
			}
			return s
		}
	}
	return ""
}

func buildHTTPMessages(messages []Message) []map[string]interface{} {
	out, _ := buildHTTPMessagesWithAudit(messages)
	return out
}

func buildHTTPMessagesWithAudit(messages []Message) ([]map[string]interface{}, []imagePayloadAudit) {
	out := make([]map[string]interface{}, 0, len(messages))
	audits := []imagePayloadAudit{}
	for _, msg := range messages {
		entry := map[string]interface{}{
			"role": msg.Role,
		}

		if msg.Role == "user" && len(msg.Media) > 0 {
			parts := make([]map[string]interface{}, 0, len(msg.Media)+1)
			if strings.TrimSpace(msg.Content) != "" {
				parts = append(parts, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, media := range msg.Media {
				imageURL, audit, ok := toImageURLPayload(media)
				audits = append(audits, audit)
				if !ok {
					continue
				}
				parts = append(parts, map[string]interface{}{
					"type":      "image_url",
					"image_url": map[string]string{"url": imageURL},
				})
			}
			if len(parts) > 0 {
				entry["content"] = parts
			} else {
				entry["content"] = msg.Content
			}
		} else {
			entry["content"] = msg.Content
		}

		if len(msg.ToolCalls) > 0 {
			entry["tool_calls"] = msg.ToolCalls
		}
		if msg.ToolCallID != "" {
			entry["tool_call_id"] = msg.ToolCallID
		}
		out = append(out, entry)
	}
	return out, audits
}

func toImageURLPayload(media MediaRef) (string, imagePayloadAudit, bool) {
	p := strings.TrimSpace(media.Path)
	audit := imagePayloadAudit{
		SourcePath: p,
		ImageURL:   "",
	}
	if p == "" {
		audit.DropReason = "empty_path"
		return "", audit, false
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		audit.URLType = "remote_url"
		audit.ImageURL = p
		audit.ImageURLLength = len(p)
		audit.Included = true
		return p, audit, true
	}
	audit.URLType = "data_uri"
	if st, err := os.Stat(p); err == nil {
		audit.LocalExistsBefore = true
		audit.LocalSizeBeforeBytes = st.Size()
	}

	data, err := os.ReadFile(p)
	if err != nil {
		audit.DropReason = "read_failed"
		return "", audit, false
	}
	// Keep payload bounded for API compatibility.
	if len(data) > maxInlineImageBytes {
		audit.DropReason = "file_too_large"
		return "", audit, false
	}

	mimeType := strings.TrimSpace(media.MIMEType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	fullURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	audit.ImageURL = fmt.Sprintf("data:%s;base64,[omitted]", mimeType)
	audit.ImageURLLength = len(fullURL)
	audit.Included = true
	if !audit.LocalExistsBefore {
		audit.LocalExistsBefore = true
		audit.LocalSizeBeforeBytes = int64(len(data))
	}
	return fullURL, audit, true
}

func isOllamaEndpoint(apiBase string) bool {
	base := strings.ToLower(strings.TrimSpace(apiBase))
	return strings.Contains(base, ":11434") || strings.Contains(base, "localhost:11434") || strings.Contains(base, "127.0.0.1:11434")
}

func imageAuditLogEntries(entries []imagePayloadAudit) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		item := map[string]interface{}{
			"source_path":              e.SourcePath,
			"url_type":                 e.URLType,
			"image_url":                e.ImageURL,
			"image_url_length":         e.ImageURLLength,
			"included":                 e.Included,
			"local_exists_before":      e.LocalExistsBefore,
			"local_size_before_bytes":  e.LocalSizeBeforeBytes,
			"drop_reason":              e.DropReason,
		}
		if e.LocalExistsAfterTimer != nil {
			item["local_exists_after_timeout"] = *e.LocalExistsAfterTimer
		}
		if e.LocalSizeAfterBytes != nil {
			item["local_size_after_timeout_bytes"] = *e.LocalSizeAfterBytes
		}
		out = append(out, item)
	}
	return out
}

func enrichAuditAfterTimeout(entries []imagePayloadAudit) []imagePayloadAudit {
	out := make([]imagePayloadAudit, len(entries))
	copy(out, entries)
	for i := range out {
		path := strings.TrimSpace(out[i].SourcePath)
		if path == "" || strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
			continue
		}
		st, err := os.Stat(path)
		if err == nil {
			exists := true
			size := st.Size()
			out[i].LocalExistsAfterTimer = &exists
			out[i].LocalSizeAfterBytes = &size
			continue
		}
		if os.IsNotExist(err) {
			exists := false
			out[i].LocalExistsAfterTimer = &exists
		}
	}
	return out
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded")
}

func (p *HTTPProvider) parseResponse(body []byte) (*LLMResponse, error) {
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *UsageInfo `json:"usage"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return &LLMResponse{
			Content:      "",
			FinishReason: "stop",
		}, nil
	}

	choice := apiResponse.Choices[0]

	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		arguments := make(map[string]interface{})
		name := ""

		// Handle OpenAI format with nested function object
		if tc.Type == "function" && tc.Function != nil {
			name = tc.Function.Name
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
					arguments["raw"] = tc.Function.Arguments
				}
			}
		} else if tc.Function != nil {
			// Legacy format without type field
			name = tc.Function.Name
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
					arguments["raw"] = tc.Function.Arguments
				}
			}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      name,
			Arguments: arguments,
		})
	}

	return &LLMResponse{
		Content:      choice.Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: choice.FinishReason,
		Usage:        apiResponse.Usage,
	}, nil
}

func (p *HTTPProvider) GetDefaultModel() string {
	return ""
}

func createClaudeAuthProvider() (LLMProvider, error) {
	cred, err := auth.GetCredential("anthropic")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for anthropic. Run: picoclaw auth login --provider anthropic")
	}
	return NewClaudeProviderWithTokenSource(cred.AccessToken, createClaudeTokenSource()), nil
}

func createCodexAuthProvider() (LLMProvider, error) {
	cred, err := auth.GetCredential("openai")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for openai. Run: picoclaw auth login --provider openai")
	}
	return NewCodexProviderWithTokenSource(cred.AccessToken, cred.AccountID, createCodexTokenSource()), nil
}

func CreateProvider(cfg *config.Config) (LLMProvider, error) {
	model := cfg.Agents.Defaults.Model
	providerName := strings.ToLower(cfg.Agents.Defaults.Provider)

	var apiKey, apiBase, proxy string

	lowerModel := strings.ToLower(model)

	// First, try to use explicitly configured provider
	if providerName != "" {
		switch providerName {
		case "groq":
			if cfg.Providers.Groq.APIKey != "" {
				apiKey = cfg.Providers.Groq.APIKey
				apiBase = cfg.Providers.Groq.APIBase
				if apiBase == "" {
					apiBase = "https://api.groq.com/openai/v1"
				}
			}
		case "openai", "gpt":
			if cfg.Providers.OpenAI.APIKey != "" || cfg.Providers.OpenAI.AuthMethod != "" {
				if cfg.Providers.OpenAI.AuthMethod == "codex-cli" {
					return NewCodexProviderWithTokenSource("", "", CreateCodexCliTokenSource()), nil
				}
				if cfg.Providers.OpenAI.AuthMethod == "oauth" || cfg.Providers.OpenAI.AuthMethod == "token" {
					return createCodexAuthProvider()
				}
				apiKey = cfg.Providers.OpenAI.APIKey
				apiBase = cfg.Providers.OpenAI.APIBase
				if apiBase == "" {
					apiBase = "https://api.openai.com/v1"
				}
			}
		case "anthropic", "claude":
			if cfg.Providers.Anthropic.APIKey != "" || cfg.Providers.Anthropic.AuthMethod != "" {
				if cfg.Providers.Anthropic.AuthMethod == "oauth" || cfg.Providers.Anthropic.AuthMethod == "token" {
					return createClaudeAuthProvider()
				}
				apiKey = cfg.Providers.Anthropic.APIKey
				apiBase = cfg.Providers.Anthropic.APIBase
				if apiBase == "" {
					apiBase = "https://api.anthropic.com/v1"
				}
			}
		case "openrouter":
			if cfg.Providers.OpenRouter.APIKey != "" {
				apiKey = cfg.Providers.OpenRouter.APIKey
				if cfg.Providers.OpenRouter.APIBase != "" {
					apiBase = cfg.Providers.OpenRouter.APIBase
				} else {
					apiBase = "https://openrouter.ai/api/v1"
				}
			}
		case "zhipu", "glm":
			if cfg.Providers.Zhipu.APIKey != "" {
				apiKey = cfg.Providers.Zhipu.APIKey
				apiBase = cfg.Providers.Zhipu.APIBase
				if apiBase == "" {
					apiBase = "https://open.bigmodel.cn/api/paas/v4"
				}
			}
		case "gemini", "google":
			if cfg.Providers.Gemini.APIKey != "" {
				apiKey = cfg.Providers.Gemini.APIKey
				apiBase = cfg.Providers.Gemini.APIBase
				if apiBase == "" {
					apiBase = "https://generativelanguage.googleapis.com/v1beta"
				}
			}
		case "vllm":
			if cfg.Providers.VLLM.APIBase != "" {
				apiKey = cfg.Providers.VLLM.APIKey
				apiBase = cfg.Providers.VLLM.APIBase
			}
		case "shengsuanyun":
			if cfg.Providers.ShengSuanYun.APIKey != "" {
				apiKey = cfg.Providers.ShengSuanYun.APIKey
				apiBase = cfg.Providers.ShengSuanYun.APIBase
				if apiBase == "" {
					apiBase = "https://router.shengsuanyun.com/api/v1"
				}
			}
		case "claude-cli", "claudecode", "claude-code":
			workspace := cfg.WorkspacePath()
			if workspace == "" {
				workspace = "."
			}
			return NewClaudeCliProvider(workspace), nil
		case "codex-cli", "codex-code":
			workspace := cfg.WorkspacePath()
			if workspace == "" {
				workspace = "."
			}
			return NewCodexCliProvider(workspace), nil
		case "deepseek":
			if cfg.Providers.DeepSeek.APIKey != "" {
				apiKey = cfg.Providers.DeepSeek.APIKey
				apiBase = cfg.Providers.DeepSeek.APIBase
				if apiBase == "" {
					apiBase = "https://api.deepseek.com/v1"
				}
				if model != "deepseek-chat" && model != "deepseek-reasoner" {
					model = "deepseek-chat"
				}
			}
		case "github_copilot", "copilot":
			if cfg.Providers.GitHubCopilot.APIBase != "" {
				apiBase = cfg.Providers.GitHubCopilot.APIBase
			} else {
				apiBase = "localhost:4321"
			}
			return NewGitHubCopilotProvider(apiBase, cfg.Providers.GitHubCopilot.ConnectMode, model)

		}

	}

	// Fallback: detect provider from model name
	if apiKey == "" && apiBase == "" {
		switch {
		case (strings.Contains(lowerModel, "kimi") || strings.Contains(lowerModel, "moonshot") || strings.HasPrefix(model, "moonshot/")) && cfg.Providers.Moonshot.APIKey != "":
			apiKey = cfg.Providers.Moonshot.APIKey
			apiBase = cfg.Providers.Moonshot.APIBase
			proxy = cfg.Providers.Moonshot.Proxy
			if apiBase == "" {
				apiBase = "https://api.moonshot.cn/v1"
			}

		case strings.HasPrefix(model, "openrouter/") || strings.HasPrefix(model, "anthropic/") || strings.HasPrefix(model, "openai/") || strings.HasPrefix(model, "meta-llama/") || strings.HasPrefix(model, "deepseek/") || strings.HasPrefix(model, "google/"):
			apiKey = cfg.Providers.OpenRouter.APIKey
			proxy = cfg.Providers.OpenRouter.Proxy
			if cfg.Providers.OpenRouter.APIBase != "" {
				apiBase = cfg.Providers.OpenRouter.APIBase
			} else {
				apiBase = "https://openrouter.ai/api/v1"
			}

		case (strings.Contains(lowerModel, "claude") || strings.HasPrefix(model, "anthropic/")) && (cfg.Providers.Anthropic.APIKey != "" || cfg.Providers.Anthropic.AuthMethod != ""):
			if cfg.Providers.Anthropic.AuthMethod == "oauth" || cfg.Providers.Anthropic.AuthMethod == "token" {
				return createClaudeAuthProvider()
			}
			apiKey = cfg.Providers.Anthropic.APIKey
			apiBase = cfg.Providers.Anthropic.APIBase
			proxy = cfg.Providers.Anthropic.Proxy
			if apiBase == "" {
				apiBase = "https://api.anthropic.com/v1"
			}

		case (strings.Contains(lowerModel, "gpt") || strings.HasPrefix(model, "openai/")) && (cfg.Providers.OpenAI.APIKey != "" || cfg.Providers.OpenAI.AuthMethod != ""):
			if cfg.Providers.OpenAI.AuthMethod == "oauth" || cfg.Providers.OpenAI.AuthMethod == "token" {
				return createCodexAuthProvider()
			}
			apiKey = cfg.Providers.OpenAI.APIKey
			apiBase = cfg.Providers.OpenAI.APIBase
			proxy = cfg.Providers.OpenAI.Proxy
			if apiBase == "" {
				apiBase = "https://api.openai.com/v1"
			}

		case (strings.Contains(lowerModel, "gemini") || strings.HasPrefix(model, "google/")) && cfg.Providers.Gemini.APIKey != "":
			apiKey = cfg.Providers.Gemini.APIKey
			apiBase = cfg.Providers.Gemini.APIBase
			proxy = cfg.Providers.Gemini.Proxy
			if apiBase == "" {
				apiBase = "https://generativelanguage.googleapis.com/v1beta"
			}

		case (strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "zhipu") || strings.Contains(lowerModel, "zai")) && cfg.Providers.Zhipu.APIKey != "":
			apiKey = cfg.Providers.Zhipu.APIKey
			apiBase = cfg.Providers.Zhipu.APIBase
			proxy = cfg.Providers.Zhipu.Proxy
			if apiBase == "" {
				apiBase = "https://open.bigmodel.cn/api/paas/v4"
			}

		case (strings.Contains(lowerModel, "groq") || strings.HasPrefix(model, "groq/")) && cfg.Providers.Groq.APIKey != "":
			apiKey = cfg.Providers.Groq.APIKey
			apiBase = cfg.Providers.Groq.APIBase
			proxy = cfg.Providers.Groq.Proxy
			if apiBase == "" {
				apiBase = "https://api.groq.com/openai/v1"
			}

		case (strings.Contains(lowerModel, "nvidia") || strings.HasPrefix(model, "nvidia/")) && cfg.Providers.Nvidia.APIKey != "":
			apiKey = cfg.Providers.Nvidia.APIKey
			apiBase = cfg.Providers.Nvidia.APIBase
			proxy = cfg.Providers.Nvidia.Proxy
			if apiBase == "" {
				apiBase = "https://integrate.api.nvidia.com/v1"
			}
		case (strings.Contains(lowerModel, "ollama") || strings.HasPrefix(model, "ollama/")) && cfg.Providers.Ollama.APIKey != "":
			fmt.Println("Ollama provider selected based on model name prefix")
			apiKey = cfg.Providers.Ollama.APIKey
			apiBase = cfg.Providers.Ollama.APIBase
			proxy = cfg.Providers.Ollama.Proxy
			if apiBase == "" {
				apiBase = "http://localhost:11434/v1"
			}
			fmt.Println("Ollama apiBase:", apiBase)
		case cfg.Providers.VLLM.APIBase != "":
			apiKey = cfg.Providers.VLLM.APIKey
			apiBase = cfg.Providers.VLLM.APIBase
			proxy = cfg.Providers.VLLM.Proxy

		default:
			if cfg.Providers.OpenRouter.APIKey != "" {
				apiKey = cfg.Providers.OpenRouter.APIKey
				proxy = cfg.Providers.OpenRouter.Proxy
				if cfg.Providers.OpenRouter.APIBase != "" {
					apiBase = cfg.Providers.OpenRouter.APIBase
				} else {
					apiBase = "https://openrouter.ai/api/v1"
				}
			} else {
				return nil, fmt.Errorf("no API key configured for model: %s", model)
			}
		}
	}

	if apiKey == "" && !strings.HasPrefix(model, "bedrock/") {
		return nil, fmt.Errorf("no API key configured for provider (model: %s)", model)
	}

	if apiBase == "" {
		return nil, fmt.Errorf("no API base configured for provider (model: %s)", model)
	}

	return NewHTTPProvider(apiKey, apiBase, proxy), nil
}
