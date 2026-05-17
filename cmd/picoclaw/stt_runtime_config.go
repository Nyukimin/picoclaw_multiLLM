package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
)

func sttStreamURLFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if raw := strings.TrimSpace(cfg.STT.StreamURL); raw != "" {
		return raw
	}
	return inferSTTStreamURLFromProviderURL(cfg.STT.ProviderURL)
}

func inferSTTStreamURLFromProviderURL(providerURL string) string {
	u, err := url.Parse(strings.TrimSpace(providerURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	scheme := "ws"
	if strings.EqualFold(u.Scheme, "https") {
		scheme = "wss"
	}
	return fmt.Sprintf("%s://%s/ws/transcribe", scheme, u.Host)
}

func inferSTTBaseURL(ttsBaseURL, sttProviderURL string) string {
	if base := extractBaseFromProviderURL(sttProviderURL); base != "" {
		return base
	}
	u, err := url.Parse(strings.TrimSpace(ttsBaseURL))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s:%d", u.Scheme, u.Hostname(), 8080)
}

func extractBaseFromProviderURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return fmt.Sprintf("%s://%s", u.Scheme, u.Host)
}

func inferSTTProviderURL(ttsBaseURL, sttProviderURL string) string {
	raw := strings.TrimSpace(sttProviderURL)
	if raw != "" {
		return raw
	}
	base := inferSTTBaseURL(ttsBaseURL, sttProviderURL)
	if base == "" {
		return ""
	}
	return strings.TrimRight(base, "/") + "/inference"
}

func inferSTTBaseURLFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.STT.ProviderURL) != "" {
		return extractBaseFromProviderURL(cfg.STT.ProviderURL)
	}
	host := strings.TrimSpace(cfg.Server.Host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	scheme := "http"
	if cfg.Server.TLS.Enabled {
		scheme = "https"
	}
	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, port)
}

func inferSTTProviderURLFromConfig(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(cfg.STT.Provider), sttinfra.ProviderExternalHTTP) || strings.TrimSpace(cfg.STT.ProviderURL) != "" {
		if strings.TrimSpace(cfg.STT.ProviderURL) != "" {
			return strings.TrimSpace(cfg.STT.ProviderURL)
		}
	}
	return strings.TrimRight(inferSTTBaseURLFromConfig(cfg), "/") + "/stt/file"
}

func buildSTTProvider(cfg *config.Config) sttinfra.Provider {
	if cfg == nil {
		return nil
	}
	if !cfg.STT.Enabled {
		return nil
	}
	providerCfg := sttinfra.Config{
		Enabled:         cfg.STT.Enabled,
		Provider:        cfg.STT.Provider,
		Language:        cfg.STT.Language,
		Model:           cfg.STT.Model,
		Timeout:         time.Duration(cfg.STT.TimeoutMS) * time.Millisecond,
		SaveAudio:       cfg.STT.Debug.SaveAudio,
		BusyPolicy:      cfg.STT.BusyPolicy,
		ExternalHTTPURL: cfg.STT.ProviderURL,
	}
	return sttinfra.NewProvider(providerCfg)
}

func inferSTTGatewayURL(sttGatewayURL, rencrowSTTURL string) string {
	if v := strings.TrimSpace(sttGatewayURL); v != "" {
		return v
	}
	return strings.TrimSpace(rencrowSTTURL)
}
