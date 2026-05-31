package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	sttinfra "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/stt"
	modulestt "github.com/Nyukimin/picoclaw_multiLLM/modules/stt"
	"golang.org/x/net/websocket"
)

func resolveSTTWebSocketHandler(sttProviderURL, sttGatewayURL string) http.Handler {
	plan := modulestt.BuildWebSocketHandlerPlan(false, sttProviderURL, sttGatewayURL)
	if plan.Mode == modulestt.WebSocketModeGateway {
		return handleSTTWebSocketProxy(plan.GatewayURL)
	}
	return handleSTTWebSocket(plan.ProviderURL)
}

func resolveSTTWebSocketHandlerWithProvider(provider sttinfra.Provider, sttProviderURL, sttGatewayURL string) http.Handler {
	plan := modulestt.BuildWebSocketHandlerPlan(provider != nil, sttProviderURL, sttGatewayURL)
	switch plan.Mode {
	case modulestt.WebSocketModeGateway:
		return handleSTTWebSocketProxy(plan.GatewayURL)
	case modulestt.WebSocketModeProvider:
		return handleSTTWebSocketProvider(provider)
	default:
		return handleSTTWebSocket(plan.ProviderURL)
	}
}

func registerSTTRoutes(mux *http.ServeMux, sttWSHandler http.Handler) {
	// Primary endpoint is /stt. Keep /stt-ws and /ws for backward compatibility.
	for _, path := range modulestt.WebSocketRoutePaths {
		mux.Handle(path, sttWSHandler)
	}
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
	return modulestt.IsWebSocketTextFramePayload(payload)
}

func handleSTTWebSocket(sttProviderURL string) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		if strings.TrimSpace(sttProviderURL) == "" {
			_ = sendSTTError(conn, "stt provider url is not configured")
			return
		}
		sendSTTSessionReady(conn, "http")

		autoFinalTimeout := sttFinalTimeoutFromEnv()
		silenceThreshold := sttSilenceAbsThresholdFromEnv()
		draftState := modulestt.DraftState{}
		adaptiveState := modulestt.AdaptiveTimeoutState{
			Timeout: sttHTTPTimeoutFromEnv(),
		}
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if finalText, ok := modulestt.FinalTextAfterDraftTimeout(draftState, time.Now(), autoFinalTimeout); ok {
						_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
						draftState = modulestt.ResetDraftAfterFinal(draftState, false)
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
					if finalText, ok := modulestt.FinalTextForPending(draftState); ok {
						_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
						draftState = modulestt.ResetDraftAfterFinal(draftState, false)
					}
				}
				continue
			}
			audioPayload := normalizeSTTAudioPayload(payload)
			if isLikelySilentWAV(audioPayload, silenceThreshold) {
				if finalText, ok := modulestt.FinalTextAfterSilence(draftState, time.Now(), autoFinalTimeout); ok {
					_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
					draftState = modulestt.ResetDraftAfterFinal(draftState, true)
				}
				continue
			}
			draftState = modulestt.MarkVoiceObserved(draftState, time.Now())
			if modulestt.InferenceInCooldown(adaptiveState, time.Now()) {
				continue
			}
			var started bool
			draftState, started = modulestt.MarkSpeechStarted(draftState)
			if started {
				_ = sendSTTEvent(conn, modulestt.BuildSpeechStartEvent())
			}

			text, err := sttInferViaHTTP(sttProviderURL, audioPayload, adaptiveState.Timeout)
			if err != nil {
				if isSTTTimeoutErr(err) {
					update := modulestt.ApplyTimeoutFailure(adaptiveState, time.Now(), 1200*time.Millisecond, 3200*time.Millisecond)
					adaptiveState = update.State
					if finalText, ok := modulestt.FinalTextOnProviderError(draftState); ok {
						// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
						_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
						draftState = modulestt.ResetDraftAfterFinal(draftState, true)
					}
					// Keep UI informative without error spam when provider stalls.
					if update.ShouldSendNotice {
						_ = sendSTTEvent(conn, modulestt.BuildTimeoutStatusEvent())
					}
					continue
				}
				if finalText, ok := modulestt.FinalTextOnProviderError(draftState); ok {
					// Fail-open: if provider stalls, finalize with the latest draft so UX does not hang.
					_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
					draftState = modulestt.ResetDraftAfterFinal(draftState, true)
					continue
				}
				_ = sendSTTError(conn, "stt inference failed: "+err.Error())
				continue
			}
			normalized := modulestt.NormalizeTranscriptText(text)
			if normalized == "" {
				continue
			}
			adaptiveState = modulestt.ApplyInferenceSuccess(adaptiveState, time.Now(), 1200*time.Millisecond, 3200*time.Millisecond)
			draftState = modulestt.ApplyDraftTranscript(draftState, normalized, time.Now())
			_ = sendSTTEvent(conn, modulestt.BuildDraftEvent(normalized))
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
		sendSTTSessionReady(conn, provider.Name())

		autoFinalTimeout := sttFinalTimeoutFromEnv()
		silenceThreshold := sttSilenceAbsThresholdFromEnv()
		draftState := modulestt.DraftState{}
		for {
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			var payload []byte
			if err := websocket.Message.Receive(conn, &payload); err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					if finalText, ok := modulestt.FinalTextAfterDraftTimeout(draftState, time.Now(), autoFinalTimeout); ok {
						_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
						draftState = modulestt.ResetDraftAfterFinal(draftState, false)
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
					if finalText, ok := modulestt.FinalTextForPending(draftState); ok {
						_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
						draftState = modulestt.ResetDraftAfterFinal(draftState, false)
					}
				}
				continue
			}
			audioPayload := normalizeSTTAudioPayload(payload)
			if isLikelySilentWAV(audioPayload, silenceThreshold) {
				if finalText, ok := modulestt.FinalTextAfterSilence(draftState, time.Now(), autoFinalTimeout); ok {
					_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
					draftState = modulestt.ResetDraftAfterFinal(draftState, true)
				}
				continue
			}
			draftState = modulestt.MarkVoiceObserved(draftState, time.Now())
			var started bool
			draftState, started = modulestt.MarkSpeechStarted(draftState)
			if started {
				_ = sendSTTEvent(conn, modulestt.BuildSpeechStartEvent())
			}
			result, err := provider.Transcribe(context.Background(), audioPayload)
			if err != nil {
				if finalText, ok := modulestt.FinalTextOnProviderError(draftState); ok {
					_ = sendSTTEvent(conn, modulestt.BuildFinalEvent(finalText))
					draftState = modulestt.ResetDraftAfterFinal(draftState, true)
					continue
				}
				_ = sendSTTError(conn, "stt inference failed: "+err.Error())
				continue
			}
			normalized := modulestt.NormalizeTranscriptText(result.Text)
			if normalized == "" {
				continue
			}
			draftState = modulestt.ApplyDraftTranscript(draftState, normalized, time.Now())
			_ = sendSTTEvent(conn, modulestt.BuildDraftEvent(normalized))
		}
	})
}

func sendSTTSessionReady(conn *websocket.Conn, provider string) {
	_ = sendSTTEvent(conn, modulestt.BuildSessionInfoEvent(sttinfra.NextEventID(time.Now()), provider))
	_ = sendSTTEvent(conn, modulestt.BuildReadyEvent())
}

func parseSTTControlMessage(payload []byte) (string, bool) {
	return modulestt.ParseControlMessage(payload)
}

func sendSTTEvent(conn *websocket.Conn, event map[string]any) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(b))
}

func sendSTTError(conn *websocket.Conn, message string) error {
	return sendSTTEvent(conn, modulestt.BuildErrorEvent(message))
}
