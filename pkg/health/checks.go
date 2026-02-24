package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func OllamaCheck(baseURL string, timeout time.Duration) CheckFunc {
	client := &http.Client{Timeout: timeout}
	return func() (bool, string) {
		resp, err := client.Get(baseURL)
		if err != nil {
			return false, fmt.Sprintf("unreachable: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Sprintf("status %d", resp.StatusCode)
		}
		return true, "ok"
	}
}

type ollamaPsResponse struct {
	Models []struct {
		Name          string `json:"name"`
		ContextLength int    `json:"context_length"`
	} `json:"models"`
}

type ModelRequirement struct {
	Name       string // モデル名（例: chat-v1:latest）
	MinContext int    // 0 でなければ、これ未満は NG
	MaxContext int    // 0 でなければ、これを超えると NG（例: 8192 で 131072 は NG）
}

func OllamaModelsCheck(baseURL string, timeout time.Duration, required []ModelRequirement) CheckFunc {
	client := &http.Client{Timeout: timeout}
	psURL := strings.TrimSuffix(baseURL, "/") + "/api/ps"

	return func() (bool, string) {
		resp, err := client.Get(psURL)
		if err != nil {
			return false, fmt.Sprintf("unreachable: %v", err)
		}
		defer resp.Body.Close()

		var ps ollamaPsResponse
		if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
			return false, fmt.Sprintf("decode error: %v", err)
		}

		loaded := make(map[string]int)
		for _, m := range ps.Models {
			loaded[m.Name] = m.ContextLength
		}

		var missing []string
		var badCtx []string
		for _, req := range required {
			ctx, ok := loaded[req.Name]
			if !ok {
				missing = append(missing, req.Name)
				continue
			}
			if req.MinContext > 0 && ctx < req.MinContext {
				badCtx = append(badCtx, fmt.Sprintf("%s(ctx=%d,want>=%d)", req.Name, ctx, req.MinContext))
			}
			if req.MaxContext > 0 && ctx > req.MaxContext {
				badCtx = append(badCtx, fmt.Sprintf("%s(ctx=%d,want<=%d)", req.Name, ctx, req.MaxContext))
			}
		}

		if len(missing) > 0 {
			return false, fmt.Sprintf("not loaded: %s", strings.Join(missing, ", "))
		}
		if len(badCtx) > 0 {
			return false, fmt.Sprintf("context mismatch: %s", strings.Join(badCtx, ", "))
		}

		return true, fmt.Sprintf("%d/%d models ok", len(required), len(required))
	}
}
