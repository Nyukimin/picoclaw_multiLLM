package viewer

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// LLMOpsProxyOptions holds MLX management daemon connection for server-side proxying.
// Token must come from LLM_OPS_TOKEN (never send to the browser).
type LLMOpsProxyOptions struct {
	BaseURL string
	Token   string
}

func (o LLMOpsProxyOptions) ready() bool {
	return strings.TrimSpace(o.BaseURL) != "" && strings.TrimSpace(o.Token) != ""
}

func normalizeLLMOpsBase(u string) string {
	return strings.TrimRight(strings.TrimSpace(u), "/")
}

// HandleLLMOpsStatus proxies GET /v1/status to the MLX management API.
func HandleLLMOpsStatus(opts LLMOpsProxyOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !opts.ready() {
			http.Error(w, "llm ops proxy not configured", http.StatusServiceUnavailable)
			return
		}
		proxyLLMOps(w, r, opts, http.MethodGet, "/v1/status", nil)
	}
}

// HandleLLMOpsStop proxies POST /v1/control/stop. Empty body defaults to Chat+Worker.
func HandleLLMOpsStop(opts LLMOpsProxyOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !opts.ready() {
			http.Error(w, "llm ops proxy not configured", http.StatusServiceUnavailable)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if len(bytes.TrimSpace(body)) == 0 {
			body = []byte(`{"roles":["Chat","Worker"]}`)
		}
		proxyLLMOps(w, r, opts, http.MethodPost, "/v1/control/stop", body)
	}
}

// HandleLLMOpsRestart proxies POST /v1/control/restart. Empty body defaults to all roles.
func HandleLLMOpsRestart(opts LLMOpsProxyOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !opts.ready() {
			http.Error(w, "llm ops proxy not configured", http.StatusServiceUnavailable)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		if len(bytes.TrimSpace(body)) == 0 {
			body = []byte(`{"roles":"all"}`)
		}
		proxyLLMOps(w, r, opts, http.MethodPost, "/v1/control/restart", body)
	}
}

func proxyLLMOps(w http.ResponseWriter, r *http.Request, opts LLMOpsProxyOptions, method, path string, body []byte) {
	base := normalizeLLMOpsBase(opts.BaseURL)
	target := base + path
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	upReq, err := http.NewRequestWithContext(r.Context(), method, target, reqBody)
	if err != nil {
		http.Error(w, "bad upstream request", http.StatusInternalServerError)
		return
	}
	upReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(opts.Token))
	if body != nil {
		upReq.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(upReq)
	if err != nil {
		log.Printf("[viewer] llm-ops %s %s: %v", method, path, err)
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if resp.StatusCode != http.StatusNoContent {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[viewer] llm-ops response copy: %v", err)
	}
}
