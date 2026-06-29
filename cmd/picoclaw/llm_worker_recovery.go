package main

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	llmmiddleware "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/middleware"
)

const llmWorkerRecoveryThrottle = 60 * time.Second

var llmWorkerRecoveryState = struct {
	sync.Mutex
	last map[string]time.Time
}{last: map[string]time.Time{}}

func llmWorkerBackendTimeoutRecovery(cfg *config.Config) func(llmmiddleware.BackendTimeoutEvent) {
	if cfg == nil || !cfg.LLMOps.Enabled || strings.TrimSpace(cfg.LLMOps.BaseURL) == "" {
		return func(ev llmmiddleware.BackendTimeoutEvent) {
			log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=unavailable reason=llm_ops_not_configured message=%q",
				strings.TrimSpace(ev.Alias), "LLMサーバ側のWorkerが詰まっています")
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LLMOps.BaseURL), "/")
	return func(ev llmmiddleware.BackendTimeoutEvent) {
		alias := strings.TrimSpace(ev.Alias)
		if alias == "" {
			alias = "Worker"
		}
		token := strings.TrimSpace(os.Getenv("LLM_OPS_TOKEN"))
		if token == "" {
			log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=unavailable reason=LLM_OPS_TOKEN_missing message=%q",
				alias, "LLMサーバ側のWorkerが詰まっています")
			return
		}
		if !claimLLMWorkerRecovery(alias, time.Now()) {
			log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=suppressed reason=throttled", alias)
			return
		}
		go requestLLMWorkerStart(baseURL, token, alias)
	}
}

func claimLLMWorkerRecovery(alias string, now time.Time) bool {
	key := strings.ToLower(strings.TrimSpace(alias))
	if key == "" {
		key = "worker"
	}
	llmWorkerRecoveryState.Lock()
	defer llmWorkerRecoveryState.Unlock()
	if last := llmWorkerRecoveryState.last[key]; !last.IsZero() && now.Sub(last) < llmWorkerRecoveryThrottle {
		return false
	}
	llmWorkerRecoveryState.last[key] = now
	return true
}

func requestLLMWorkerStart(baseURL, token, alias string) {
	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/control/start", bytes.NewBufferString(`{"selection":"Worker"}`))
	if err != nil {
		log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=request_build_failed error=%q", alias, err.Error())
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 650 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=request_failed error=%q", alias, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=auth_failed status=401", alias)
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=failed status=%d", alias, resp.StatusCode)
		return
	}
	log.Printf("[LLM][worker_backend_timeout] alias=%s recovery=requested selection=Worker", alias)
}
