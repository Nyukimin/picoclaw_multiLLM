package health

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
)

// OpenAICompatibleChatCheck verifies an OpenAI-compatible chat completions endpoint.
type OpenAICompatibleChatCheck struct {
	role    string
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// NewOpenAICompatibleChatCheck creates a lightweight health check for local OpenAI-compatible LLMs.
func NewOpenAICompatibleChatCheck(role, baseURL, model, apiKey string, timeout time.Duration) *OpenAICompatibleChatCheck {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &OpenAICompatibleChatCheck{
		role:    strings.TrimSpace(role),
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		model:   strings.TrimSpace(model),
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *OpenAICompatibleChatCheck) Name() string {
	role := strings.ToLower(strings.TrimSpace(c.role))
	if role == "" {
		role = "llm"
	}
	return fmt.Sprintf("local_llm_%s", role)
}

func (c *OpenAICompatibleChatCheck) Run(ctx context.Context) domainhealth.CheckResult {
	start := time.Now()
	if c.baseURL == "" {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: "base_url is empty", Duration: time.Since(start)}
	}
	if c.model == "" {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: "model is empty", Duration: time.Since(start)}
	}

	body, err := json.Marshal(map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "user", "content": "health check"},
		},
		"max_tokens":         1,
		"temperature":        0,
		"parse_reasoning":    true,
		"include_reasoning":  false,
		"separate_reasoning": true,
	})
	if err != nil {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: fmt.Sprintf("marshal failed: %v", err), Duration: time.Since(start)}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: fmt.Sprintf("request creation failed: %v", err), Duration: time.Since(start)}
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if isOpenAICompatibleWorkerTimeout(c.role, err) {
			return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: "worker_backend_timeout: LLM server Worker backend is not responding", Duration: time.Since(start)}
		}
		if isOpenAICompatibleStandbyConnectionRefused(c.role, err) {
			return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDegraded, Message: "standby: Worker/Heavy/Wild are exclusive roles; start the selected role via llm_ops when needed", Duration: time.Since(start)}
		}
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: fmt.Sprintf("connection failed: %v", err), Duration: time.Since(start)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		if isOpenAICompatibleWorkerBusyStatus(c.role, resp.StatusCode, string(respBody)) {
			return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDegraded, Message: "worker_backend_busy: Worker/Ollama is busy", Duration: time.Since(start)}
		}
		if isOpenAICompatibleWorkerTimeoutStatus(c.role, resp.StatusCode, string(respBody)) {
			return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: "worker_backend_timeout: LLM server Worker backend is not responding", Duration: time.Since(start)}
		}
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(respBody)), Duration: time.Since(start)}
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDown, Message: fmt.Sprintf("decode error: %v", err), Duration: time.Since(start)}
	}
	if len(parsed.Choices) == 0 {
		return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusDegraded, Message: "empty choices", Duration: time.Since(start)}
	}

	return domainhealth.CheckResult{Name: c.Name(), Status: domainhealth.StatusOK, Message: fmt.Sprintf("%s reachable via %s", c.model, c.baseURL), Duration: time.Since(start)}
}

func isOpenAICompatibleWorkerTimeout(role string, err error) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	if r != "worker" && r != "chatworker" && !strings.HasPrefix(r, "coder") {
		return false
	}
	return isOpenAICompatibleTimeout(err)
}

func isOpenAICompatibleStandbyConnectionRefused(role string, err error) bool {
	if !isOpenAICompatibleExclusiveRole(role) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused")
}

func isOpenAICompatibleTimeout(err error) bool {
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
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "timed out") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "operation has timed out")
}

func isOpenAICompatibleWorkerBusyStatus(role string, status int, body string) bool {
	if !isOpenAICompatibleWorkerRole(role) {
		return false
	}
	return status == http.StatusTooManyRequests
}

func isOpenAICompatibleWorkerTimeoutStatus(role string, status int, body string) bool {
	if !isOpenAICompatibleWorkerRole(role) {
		return false
	}
	return status == http.StatusGatewayTimeout
}

func isOpenAICompatibleWorkerRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	return r == "worker" || r == "chatworker" || strings.HasPrefix(r, "coder")
}

func isOpenAICompatibleExclusiveRole(role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	return isOpenAICompatibleWorkerRole(r) || r == "heavy" || r == "wild"
}
