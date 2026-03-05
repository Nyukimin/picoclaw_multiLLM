package health

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	domainhealth "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/health"
)

// OllamaCheck は Ollama サーバーへの接続確認（/api/tags）
type OllamaCheck struct {
	baseURL string
	client  *http.Client
}

// NewOllamaCheck は新しい OllamaCheck を作成
func NewOllamaCheck(baseURL string) *OllamaCheck {
	return &OllamaCheck{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *OllamaCheck) Name() string { return "ollama_connection" }

func (c *OllamaCheck) Run(ctx context.Context) domainhealth.CheckResult {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("request creation failed: %v", err),
			Duration: time.Since(start),
		}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("connection failed: %v", err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(body)),
			Duration: time.Since(start),
		}
	}

	return domainhealth.CheckResult{
		Name:     c.Name(),
		Status:   domainhealth.StatusOK,
		Message:  "connected",
		Duration: time.Since(start),
	}
}

// OllamaModelCheck は指定モデルの存在確認（/api/show）
type OllamaModelCheck struct {
	baseURL   string
	modelName string
	client    *http.Client
}

// NewOllamaModelCheck は新しい OllamaModelCheck を作成
func NewOllamaModelCheck(baseURL, modelName string) *OllamaModelCheck {
	return &OllamaModelCheck{
		baseURL:   baseURL,
		modelName: modelName,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *OllamaModelCheck) Name() string {
	return fmt.Sprintf("ollama_model_%s", c.modelName)
}

func (c *OllamaModelCheck) Run(ctx context.Context) domainhealth.CheckResult {
	start := time.Now()

	body, err := json.Marshal(map[string]string{"name": c.modelName})
	if err != nil {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("marshal failed: %v", err),
			Duration: time.Since(start),
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("request creation failed: %v", err),
			Duration: time.Since(start),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("connection failed: %v", err),
			Duration: time.Since(start),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDown,
			Message:  fmt.Sprintf("model %q not found", c.modelName),
			Duration: time.Since(start),
		}
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return domainhealth.CheckResult{
			Name:     c.Name(),
			Status:   domainhealth.StatusDegraded,
			Message:  fmt.Sprintf("unexpected status %d: %s", resp.StatusCode, string(respBody)),
			Duration: time.Since(start),
		}
	}

	return domainhealth.CheckResult{
		Name:     c.Name(),
		Status:   domainhealth.StatusOK,
		Message:  fmt.Sprintf("model %q available", c.modelName),
		Duration: time.Since(start),
	}
}
