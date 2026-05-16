package main

import (
	"net/http"
	"os"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
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
}

func buildSTTRuntime(cfg *config.Config) sttRuntime {
	provider := buildSTTProvider(cfg)
	providerURL := inferSTTProviderURLFromConfig(cfg)
	gatewayURL := inferSTTGatewayURL(os.Getenv("STT_GATEWAY_URL"), os.Getenv("RENCROW_STT_URL"))
	return sttRuntime{
		Provider:    provider,
		Handler:     sttinfra.NewHandler(provider),
		ProviderURL: providerURL,
		GatewayURL:  gatewayURL,
		WSHandler:   resolveSTTWebSocketHandlerWithProvider(provider, providerURL, gatewayURL),
		DebugOptions: viewer.DebugSystemOptions{
			TTSBaseURL:    inferTTSDebugBaseURLFromConfig(cfg),
			TTSHealthPath: inferTTSDebugHealthPathFromConfig(cfg),
			STTBaseURL:    inferSTTBaseURLFromConfig(cfg),
			STTStreamURL:  sttStreamURLFromConfig(cfg),
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
