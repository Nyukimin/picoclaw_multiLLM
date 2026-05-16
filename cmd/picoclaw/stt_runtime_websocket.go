package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
	"golang.org/x/net/websocket"
)

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
		go relay(conn, gw) // browser -> voice-bridge
		go relay(gw, conn) // voice-bridge -> browser
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
