package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type ttsPlaybackAckRequest struct {
	ResponseID  string `json:"response_id"`
	SessionID   string `json:"session_id"`
	UtteranceID string `json:"utterance_id"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

func handleTTSPlaybackAck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var req ttsPlaybackAckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		responseID := strings.TrimSpace(req.ResponseID)
		if responseID == "" {
			http.Error(w, "response_id is required", http.StatusBadRequest)
			return
		}
		ok := notifyIdleChatTTSPlaybackCompleted(responseID)
		log.Printf("[TTSPlayback] ack response_id=%s session=%s utterance=%s status=%s matched=%t error=%s",
			responseID,
			strings.TrimSpace(req.SessionID),
			strings.TrimSpace(req.UtteranceID),
			strings.TrimSpace(req.Status),
			ok,
			strings.TrimSpace(req.Error),
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "matched": ok})
	}
}
