package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/modulebridge"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
	modulestt "github.com/Nyukimin/picoclaw_multiLLM/modules/stt"
)

// This file is the integration boundary for RenCrow_STT.
// Keep STT provider selection, URL inference, handler creation, and route
// registration here so the main server does not depend on STT wiring details.

type sttRuntime struct {
	Provider     sttinfra.Provider
	Handler      *sttinfra.Handler
	ProviderURL  string
	GatewayURL   string
	WSHandler    http.Handler
	DebugOptions viewer.DebugSystemOptions
	Module       modulestt.Provider
}

func buildSTTRuntime(cfg *config.Config) sttRuntime {
	provider := buildSTTProvider(cfg)
	providerURL := inferSTTProviderURLFromConfig(cfg)
	gatewayURL := inferSTTGatewayURL(os.Getenv("STT_GATEWAY_URL"), os.Getenv("RENCROW_STT_URL"))
	debugSTTBaseURL := inferSTTBaseURLFromConfig(cfg)
	debugSTTStreamURL := sttStreamURLFromConfig(cfg)
	if cfg != nil && cfg.RenCrow.STT.Enabled {
		if strings.TrimSpace(cfg.RenCrow.STT.Engine) == "llm_audio" {
			debugSTTBaseURL = ""
		} else if strings.TrimSpace(cfg.RenCrow.STT.BaseURL) != "" {
			debugSTTBaseURL = strings.TrimRight(strings.TrimSpace(cfg.RenCrow.STT.BaseURL), "/")
		}
		if strings.TrimSpace(cfg.RenCrow.STT.StreamURL) != "" {
			debugSTTStreamURL = strings.TrimSpace(cfg.RenCrow.STT.StreamURL)
		}
	}
	return sttRuntime{
		Provider:    provider,
		Handler:     sttinfra.NewHandler(provider),
		ProviderURL: providerURL,
		GatewayURL:  gatewayURL,
		WSHandler:   resolveSTTWebSocketHandlerWithProvider(provider, providerURL, gatewayURL),
		Module:      modulebridge.NewRuntimeSTTProviderAdapter(provider),
		DebugOptions: viewer.DebugSystemOptions{
			TTSBaseURL:    inferTTSDebugBaseURLFromConfig(cfg),
			TTSHealthPath: inferTTSDebugHealthPathFromConfig(cfg),
			STTBaseURL:    debugSTTBaseURL,
			STTStreamURL:  debugSTTStreamURL,
			STTEngine:     cfg.RenCrow.STT.Engine,
			STTLLMAudio: viewer.STTLLMAudioRuntimeConfig{
				LLMRef:         cfg.RenCrow.STT.LLMAudio.LLMRef,
				Model:          cfg.RenCrow.STT.LLMAudio.Model,
				EndpointPath:   cfg.RenCrow.STT.LLMAudio.EndpointPath,
				Prompt:         cfg.RenCrow.STT.LLMAudio.Prompt,
				ResponseFormat: cfg.RenCrow.STT.LLMAudio.ResponseFormat,
			},
		},
	}
}

func registerSTTRuntimeRoutes(mux *http.ServeMux, rt sttRuntime) {
	if mux == nil {
		return
	}
	handler := rt.Handler
	if handler == nil {
		handler = sttinfra.NewHandler(nil)
	}
	mux.HandleFunc("/stt/health", handler.Health)
	mux.HandleFunc("/stt/file", handler.File)
	mux.HandleFunc("/stt/chat-input", handler.ChatInput)
	registerSTTRoutes(mux, rt.WSHandler)
}
