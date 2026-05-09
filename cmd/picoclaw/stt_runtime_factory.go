package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
	"golang.org/x/net/websocket"
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

func resolveSTTWebSocketHandler(sttProviderURL, sttGatewayURL string) http.Handler {
	sttWSHandler := handleSTTWebSocket(sttProviderURL)
	if strings.TrimSpace(sttGatewayURL) != "" {
		sttWSHandler = handleSTTWebSocketProxy(sttGatewayURL)
	}
	return sttWSHandler
}

func resolveSTTWebSocketHandlerWithProvider(provider sttinfra.Provider, sttProviderURL, sttGatewayURL string) http.Handler {
	sttWSHandler := handleSTTWebSocketProvider(provider)
	if provider == nil {
		sttWSHandler = handleSTTWebSocket(sttProviderURL)
	}
	if strings.TrimSpace(sttGatewayURL) != "" {
		sttWSHandler = handleSTTWebSocketProxy(sttGatewayURL)
	}
	return sttWSHandler
}

func registerSTTRoutes(mux *http.ServeMux, sttWSHandler http.Handler) {
	// Primary endpoint is /stt. Keep /stt-ws and /ws for backward compatibility.
	mux.Handle("/stt", sttWSHandler)
	mux.Handle("/stt-ws", sttWSHandler)
	mux.Handle("/ws", sttWSHandler)
}

// handleSTTWebSocketProxy は /stt を voice-bridge（STT Gateway）へ透過プロキシする。
// STT_GATEWAY_URL または RENCROW_STT_URL に voice-bridge の WebSocket URL を設定すると有効になる。
// 例: RENCROW_STT_URL=ws://192.168.1.36:8090/stt
func handleSTTWebSocketProxy(gatewayURL string) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		origin := "http://localhost/"
		gw, err := websocket.Dial(gatewayURL, "", origin)
		if err != nil {
			_ = sendSTTError(conn, "voice-bridge unavailable: "+err.Error())
			return
		}
		defer gw.Close()

		errc := make(chan error, 2)
		relay := func(src, dst *websocket.Conn) {
			for {
				var msg []byte
				if err := websocket.Message.Receive(src, &msg); err != nil {
					errc <- err
					return
				}
				var sendErr error
				if isSTTTextFramePayload(msg) {
					sendErr = websocket.Message.Send(dst, string(msg))
				} else {
					sendErr = websocket.Message.Send(dst, msg)
				}
				if sendErr != nil {
					errc <- sendErr
					return
				}
			}
		}
		go relay(conn, gw) // browser → voice-bridge
		go relay(gw, conn) // voice-bridge → browser
		<-errc
	})
}

func isSTTTextFramePayload(payload []byte) bool {
	if len(payload) == 0 {
		return true
	}
	switch payload[0] {
	case '{', '[', '"':
		return json.Valid(payload)
	default:
		return false
	}
}

func handleSTTWebSocket(sttProviderURL string) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if strings.TrimSpace(sttProviderURL) == "" {
			_ = sendSTTError(conn, "stt provider url is not configured")
			return
		}

		autoFinalTimeout := sttFinalTimeoutFromEnv()
		silenceThreshold := sttSilenceAbsThresholdFromEnv()
		adaptiveInferTimeout := sttHTTPTimeoutFromEnv()
		speechStarted := false
		lastDraft := ""
		lastDraftAt := time.Time{}
		lastVoiceAt := time.Time{}
		inferCooldownUntil := time.Time{}
		lastTimeoutNotice := time.Time{}
		timeoutStreak := 0
		successStreak := 0
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastDraftAt.IsZero() && time.Since(lastDraftAt) >= autoFinalTimeout {
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": strings.TrimSpace(lastDraft),
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
					continue
				}
				return
			}
			if len(payload) == 0 {
				continue
			}

			control, isControl := parseSTTControlMessage(payload)
			if isControl {
				if control == "final_pending" {
					finalText := strings.TrimSpace(lastDraft)
					if finalText != "" {
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": finalText,
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
				}
				continue
			}
			audioPayload := normalizeSTTAudioPayload(payload)
			if isLikelySilentWAV(audioPayload, silenceThreshold) {
				if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastVoiceAt.IsZero() && time.Since(lastVoiceAt) >= autoFinalTimeout {
					_ = sendSTTEvent(conn, map[string]any{
						"type": "final",
						"text": strings.TrimSpace(lastDraft),
					})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
				}
				continue
			}
			lastVoiceAt = time.Now()
			if !inferCooldownUntil.IsZero() && time.Now().Before(inferCooldownUntil) {
				continue
			}
			if !speechStarted {
				speechStarted = true
				_ = sendSTTEvent(conn, map[string]any{"type": "speech_start"})
			}

			text, err := sttInferViaHTTP(sttProviderURL, audioPayload, adaptiveInferTimeout)
			if err != nil {
				if isSTTTimeoutErr(err) {
					timeoutStreak++
					successStreak = 0
					if timeoutStreak >= 2 {
						adaptiveInferTimeout = adjustAdaptiveSTTTimeout(adaptiveInferTimeout, 300*time.Millisecond, 1200*time.Millisecond, 3200*time.Millisecond)
					}
					inferCooldownUntil = time.Now().Add(800 * time.Millisecond)
					if speechStarted && strings.TrimSpace(lastDraft) != "" {
						// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
						_ = sendSTTEvent(conn, map[string]any{
							"type": "final",
							"text": strings.TrimSpace(lastDraft),
						})
						lastDraft = ""
						lastDraftAt = time.Time{}
						lastVoiceAt = time.Time{}
						speechStarted = false
					}
					// Keep UI informative without error spam when provider stalls.
					if time.Since(lastTimeoutNotice) > 3*time.Second {
						lastTimeoutNotice = time.Now()
						_ = sendSTTEvent(conn, map[string]any{
							"type": "status",
							"text": "stt provider timeout (retrying)",
						})
					}
					continue
				}
				if speechStarted && strings.TrimSpace(lastDraft) != "" {
					// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
					_ = sendSTTEvent(conn, map[string]any{
						"type": "final",
						"text": strings.TrimSpace(lastDraft),
					})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
					continue
				}
				_ = sendSTTError(conn, "stt inference failed: "+err.Error())
				continue
			}
			normalized := strings.TrimSpace(text)
			if normalized == "" {
				continue
			}
			successStreak++
			timeoutStreak = 0
			if successStreak >= 4 {
				adaptiveInferTimeout = adjustAdaptiveSTTTimeout(adaptiveInferTimeout, -100*time.Millisecond, 1200*time.Millisecond, 3200*time.Millisecond)
				successStreak = 0
			}
			inferCooldownUntil = time.Time{}
			lastDraft = normalized
			lastDraftAt = time.Now()
			_ = sendSTTEvent(conn, map[string]any{
				"type": "draft",
				"text": normalized,
			})
		}
	})
}

func handleSTTWebSocketProvider(provider sttinfra.Provider) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if provider == nil {
			_ = sendSTTError(conn, "stt provider is not configured")
			return
		}
		_ = sendSTTEvent(conn, map[string]any{
			"type":       "session_info",
			"session_id": sttinfra.NextEventID(time.Now()),
			"provider":   provider.Name(),
		})

		autoFinalTimeout := sttFinalTimeoutFromEnv()
		silenceThreshold := sttSilenceAbsThresholdFromEnv()
		speechStarted := false
		lastDraft := ""
		lastDraftAt := time.Time{}
		lastVoiceAt := time.Time{}
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastDraftAt.IsZero() && time.Since(lastDraftAt) >= autoFinalTimeout {
						_ = sendSTTEvent(conn, map[string]any{"type": "final", "text": strings.TrimSpace(lastDraft)})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
					continue
				}
				return
			}
			if len(payload) == 0 {
				continue
			}
			control, isControl := parseSTTControlMessage(payload)
			if isControl {
				if control == "final_pending" {
					finalText := strings.TrimSpace(lastDraft)
					if finalText != "" {
						_ = sendSTTEvent(conn, map[string]any{"type": "final", "text": finalText})
						lastDraft = ""
						lastDraftAt = time.Time{}
						speechStarted = false
					}
				}
				continue
			}
			audioPayload := normalizeSTTAudioPayload(payload)
			if isLikelySilentWAV(audioPayload, silenceThreshold) {
				if speechStarted && strings.TrimSpace(lastDraft) != "" && !lastVoiceAt.IsZero() && time.Since(lastVoiceAt) >= autoFinalTimeout {
					_ = sendSTTEvent(conn, map[string]any{"type": "final", "text": strings.TrimSpace(lastDraft)})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
				}
				continue
			}
			lastVoiceAt = time.Now()
			if !speechStarted {
				speechStarted = true
				_ = sendSTTEvent(conn, map[string]any{"type": "speech_start"})
			}
			result, err := provider.Transcribe(context.Background(), audioPayload)
			if err != nil {
				if speechStarted && strings.TrimSpace(lastDraft) != "" {
					_ = sendSTTEvent(conn, map[string]any{"type": "final", "text": strings.TrimSpace(lastDraft)})
					lastDraft = ""
					lastDraftAt = time.Time{}
					lastVoiceAt = time.Time{}
					speechStarted = false
					continue
				}
				_ = sendSTTError(conn, "stt inference failed: "+err.Error())
				continue
			}
			normalized := strings.TrimSpace(result.Text)
			if normalized == "" {
				continue
			}
			lastDraft = normalized
			lastDraftAt = time.Now()
			_ = sendSTTEvent(conn, map[string]any{"type": "draft", "text": normalized})
		}
	})
}

func sttFinalTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_FINAL_TIMEOUT_MS"))
	if raw == "" {
		return 1200 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 200 {
		return 1200 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func sttSilenceAbsThresholdFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("STT_SILENCE_ABS_THRESHOLD"))
	if raw == "" {
		return 220
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return 220
	}
	return v
}

func isLikelySilentWAV(wav []byte, absThreshold int) bool {
	if len(wav) <= 44 {
		return false
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		return false
	}
	sampleBytes := wav[44:]
	if len(sampleBytes) < 2 {
		return false
	}
	var sum int64
	var n int64
	for i := 0; i+1 < len(sampleBytes); i += 2 {
		s := int16(sampleBytes[i]) | int16(sampleBytes[i+1])<<8
		if s < 0 {
			sum += int64(-s)
		} else {
			sum += int64(s)
		}
		n++
	}
	if n == 0 {
		return false
	}
	avgAbs := int(sum / n)
	return avgAbs < absThreshold
}

func normalizeSTTAudioPayload(payload []byte) []byte {
	if sttinfra.IsWAV(payload) {
		return payload
	}
	if len(payload) < 2 {
		return payload
	}
	audioLen := len(payload)
	if audioLen%2 != 0 {
		audioLen--
	}
	return pcm16LEToWAV(payload[:audioLen], 16000)
}

func pcm16LEToWAV(pcm []byte, sampleRate int) []byte {
	if sampleRate <= 0 {
		sampleRate = 16000
	}
	dataSize := len(pcm)
	out := make([]byte, 44+dataSize)
	copy(out[0:4], "RIFF")
	putLE32(out[4:8], uint32(36+dataSize))
	copy(out[8:12], "WAVE")
	copy(out[12:16], "fmt ")
	putLE32(out[16:20], 16)
	putLE16(out[20:22], 1)
	putLE16(out[22:24], 1)
	putLE32(out[24:28], uint32(sampleRate))
	putLE32(out[28:32], uint32(sampleRate*2))
	putLE16(out[32:34], 2)
	putLE16(out[34:36], 16)
	copy(out[36:40], "data")
	putLE32(out[40:44], uint32(dataSize))
	copy(out[44:], pcm)
	return out
}

func putLE16(dst []byte, v uint16) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
}

func putLE32(dst []byte, v uint32) {
	dst[0] = byte(v)
	dst[1] = byte(v >> 8)
	dst[2] = byte(v >> 16)
	dst[3] = byte(v >> 24)
}

func parseSTTControlMessage(payload []byte) (string, bool) {
	if len(payload) == 0 {
		return "", false
	}
	lead := payload[0]
	if lead != '{' && lead != '[' && lead != '"' {
		return "", false
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return "", false
	}
	msgType, _ := obj["type"].(string)
	if strings.TrimSpace(msgType) != "" {
		return strings.TrimSpace(msgType), true
	}
	return "", false
}

func sendSTTEvent(conn *websocket.Conn, event map[string]any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(b))
}

func sendSTTError(conn *websocket.Conn, message string) error {
	return sendSTTEvent(conn, map[string]any{
		"type":  "error",
		"error": strings.TrimSpace(message),
	})
}

func sttInferViaHTTP(providerURL string, wav []byte, timeout time.Duration) (string, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)

	part, err := w.CreateFormFile("file", "audio.wav")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(wav); err != nil {
		return "", err
	}
	if err := w.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, providerURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return out.Text, nil
}

func adjustAdaptiveSTTTimeout(cur, delta, minV, maxV time.Duration) time.Duration {
	next := cur + delta
	if next < minV {
		return minV
	}
	if next > maxV {
		return maxV
	}
	return next
}

func sttHTTPTimeoutFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STT_TIMEOUT_MS"))
	if raw == "" {
		return 3000 * time.Millisecond
	}
	ms, err := strconv.Atoi(raw)
	if err != nil || ms < 300 {
		return 3000 * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}

func isSTTTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "client.timeout exceeded") || strings.Contains(msg, "context deadline exceeded")
}
