package main

import (
	"encoding/json"
	"net/http"

	modulevoicechat "github.com/Nyukimin/picoclaw_multiLLM/modules/voicechat"
	"golang.org/x/net/websocket"
)

func registerVoiceChatRoutes(mux *http.ServeMux, handler http.Handler) {
	if mux == nil || handler == nil {
		return
	}
	for _, path := range modulevoicechat.WebSocketRoutePaths {
		mux.Handle(path, handler)
	}
}

func resolveVoiceChatWebSocketHandler(plan modulevoicechat.BridgePlan, voiceDirect voiceDirectFinalHandler) http.Handler {
	switch {
	case plan.Disabled:
		return handleVoiceChatDisabled()
	case !plan.Available:
		return handleVoiceChatUnavailable()
	default:
		return handleVoiceChatWebSocketBridge(plan.GatewayURL, voiceDirect)
	}
}

func handleVoiceChatDisabled() http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		_ = sendVoiceChatError(conn, modulevoicechat.ErrorVoiceChatDisabled, "voice chat is disabled")
	})
}

func handleVoiceChatUnavailable() http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		_ = sendVoiceChatError(conn, modulevoicechat.ErrorLLMSessionUnavailable, "voice chat gateway is not configured")
	})
}

func handleVoiceChatWebSocketBridge(gatewayURL string, voiceDirect voiceDirectFinalHandler) http.Handler {
	return websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		origin := "http://localhost/"
		gw, err := websocket.Dial(gatewayURL, "", origin)
		if err != nil {
			_ = sendVoiceChatError(conn, modulevoicechat.ErrorLLMSessionUnavailable, "RenCrow LLM voice bridge unavailable: "+err.Error())
			return
		}
		defer gw.Close()

		tracker := newVoiceChatBridgeTracker(voiceDirect)
		errc := make(chan error, 2)
		go relayVoiceChatFrames(conn, gw, tracker, true, errc)
		go relayVoiceChatFrames(gw, conn, tracker, false, errc)
		<-errc
	})
}

func relayVoiceChatFrames(src, dst *websocket.Conn, tracker *voiceChatBridgeTracker, fromClient bool, errc chan error) {
	for {
		var msg []byte
		if err := websocket.Message.Receive(src, &msg); err != nil {
			errc <- err
			return
		}
		if tracker != nil && modulevoicechat.IsWebSocketTextFramePayload(msg) {
			if fromClient {
				tracker.observeClientText(msg)
			} else {
				tracker.observeGatewayText(msg)
			}
		}
		var sendErr error
		if modulevoicechat.IsWebSocketTextFramePayload(msg) {
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

func sendVoiceChatError(conn *websocket.Conn, errorCode, message string) error {
	payload := map[string]string{
		"type":       modulevoicechat.EventError,
		"error_code": errorCode,
		"message":    message,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return websocket.Message.Send(conn, string(data))
}

func isVoiceChatTextFramePayload(payload []byte) bool {
	return modulevoicechat.IsWebSocketTextFramePayload(payload)
}
