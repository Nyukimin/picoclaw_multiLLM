package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

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
		return handleVoiceChatInputAudioBridge(plan.GatewayURL, voiceDirect)
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
		viewerClientID := voiceChatViewerClientID(conn)
		log.Printf("[voice-chat] viewer connected viewer_client_id=%s gateway=%s", viewerClientID, gatewayURL)
		origin := "http://localhost/"
		gw, err := websocket.Dial(gatewayURL, "", origin)
		if err != nil {
			log.Printf("[voice-chat] gateway dial failed viewer_client_id=%s gateway=%s err=%v", viewerClientID, gatewayURL, err)
			_ = sendVoiceChatError(conn, modulevoicechat.ErrorLLMSessionUnavailable, "RenCrow LLM voice bridge unavailable: "+err.Error())
			return
		}
		defer gw.Close()
		log.Printf("[voice-chat] gateway connected viewer_client_id=%s", viewerClientID)

		tracker := newVoiceChatBridgeTracker(voiceDirect)
		errc := make(chan error, 2)
		go relayVoiceChatFrames(conn, gw, tracker, true, viewerClientID, errc)
		go relayVoiceChatFrames(gw, conn, tracker, false, viewerClientID, errc)
		err = <-errc
		log.Printf("[voice-chat] bridge closed viewer_client_id=%s err=%v", viewerClientID, err)
	})
}

func relayVoiceChatFrames(src, dst *websocket.Conn, tracker *voiceChatBridgeTracker, fromClient bool, viewerClientID string, errc chan error) {
	direction := "gateway_to_viewer"
	if fromClient {
		direction = "viewer_to_gateway"
	}
	binaryFrames := 0
	binaryBytes := 0
	for {
		var msg []byte
		if err := websocket.Message.Receive(src, &msg); err != nil {
			log.Printf("[voice-chat] relay receive closed direction=%s viewer_client_id=%s binary_frames=%d binary_bytes=%d err=%v", direction, viewerClientID, binaryFrames, binaryBytes, err)
			errc <- err
			return
		}
		if tracker != nil && modulevoicechat.IsWebSocketTextFramePayload(msg) {
			logVoiceChatTextFrame(direction, viewerClientID, msg)
			if fromClient {
				tracker.observeClientText(msg)
			} else {
				tracker.observeGatewayText(msg)
			}
		} else if !modulevoicechat.IsWebSocketTextFramePayload(msg) {
			binaryFrames++
			binaryBytes += len(msg)
			if binaryFrames == 1 || binaryFrames%50 == 0 {
				log.Printf("[voice-chat] binary relay direction=%s viewer_client_id=%s frames=%d bytes=%d last_bytes=%d", direction, viewerClientID, binaryFrames, binaryBytes, len(msg))
			}
		}
		var sendErr error
		if modulevoicechat.IsWebSocketTextFramePayload(msg) {
			sendErr = websocket.Message.Send(dst, string(msg))
		} else {
			sendErr = websocket.Message.Send(dst, msg)
		}
		if sendErr != nil {
			log.Printf("[voice-chat] relay send failed direction=%s viewer_client_id=%s binary_frames=%d binary_bytes=%d err=%v", direction, viewerClientID, binaryFrames, binaryBytes, sendErr)
			errc <- sendErr
			return
		}
	}
}

func voiceChatViewerClientID(conn *websocket.Conn) string {
	if conn == nil || conn.Request() == nil || conn.Request().URL == nil {
		return ""
	}
	return strings.TrimSpace(conn.Request().URL.Query().Get("viewer_client_id"))
}

func logVoiceChatTextFrame(direction, viewerClientID string, msg []byte) {
	var ev map[string]any
	if err := json.Unmarshal(msg, &ev); err != nil {
		log.Printf("[voice-chat] text relay direction=%s viewer_client_id=%s invalid_json bytes=%d", direction, viewerClientID, len(msg))
		return
	}
	eventType, _ := ev["type"].(string)
	if eventType == modulevoicechat.EventSessionProgress || eventType == modulevoicechat.EventLLMDelta {
		return
	}
	utteranceID, _ := ev["utterance_id"].(string)
	sessionID, _ := ev["session_id"].(string)
	text := voiceChatFirstNonEmpty(stringField(ev, "text"), stringField(ev, "message"), stringField(ev, "error_code"))
	log.Printf(
		"[voice-chat] text relay direction=%s viewer_client_id=%s type=%s utterance_id=%s session_id=%s text_len=%d text_sample=%q",
		direction,
		viewerClientID,
		eventType,
		strings.TrimSpace(utteranceID),
		strings.TrimSpace(sessionID),
		len([]rune(text)),
		voiceChatShortLogText(text, 120),
	)
}

func voiceChatShortLogText(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
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
