package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

type viewerActiveControl struct {
	mu                  sync.RWMutex
	activeAudioViewerID string
	activeInputViewerID string
}

type viewerActiveClaimRequest struct {
	ViewerClientID string `json:"viewer_client_id"`
	Kind           string `json:"kind"`
	Reason         string `json:"reason,omitempty"`
}

type viewerActiveControlSnapshot struct {
	ActiveAudioViewerID string `json:"active_audio_viewer_id"`
	ActiveInputViewerID string `json:"active_input_viewer_id"`
}

var activeViewerControl = &viewerActiveControl{}

func (c *viewerActiveControl) claim(kind, viewerClientID string) viewerActiveControlSnapshot {
	id := strings.TrimSpace(viewerClientID)
	kind = strings.TrimSpace(kind)
	c.mu.Lock()
	defer c.mu.Unlock()
	switch kind {
	case "audio":
		c.activeAudioViewerID = id
	case "input":
		c.activeInputViewerID = id
	}
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func (c *viewerActiveControl) isActiveAudio(viewerClientID string) bool {
	id := strings.TrimSpace(viewerClientID)
	c.mu.RLock()
	defer c.mu.RUnlock()
	return id != "" && c.activeAudioViewerID != "" && c.activeAudioViewerID == id
}

func (c *viewerActiveControl) snapshot() viewerActiveControlSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func resetActiveViewerControlForTest() {
	activeViewerControl = &viewerActiveControl{}
}

func handleViewerActiveClaim(emit func(orchestrator.OrchestratorEvent)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var req viewerActiveClaimRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		viewerID := strings.TrimSpace(req.ViewerClientID)
		if viewerID == "" {
			http.Error(w, "viewer_client_id is required", http.StatusBadRequest)
			return
		}
		kind := strings.TrimSpace(req.Kind)
		if kind != "audio" && kind != "input" {
			http.Error(w, "kind must be audio or input", http.StatusBadRequest)
			return
		}
		snapshot := activeViewerControl.claim(kind, viewerID)
		log.Printf("[ViewerActive] claim kind=%s viewer_client_id=%s reason=%s active_audio=%s active_input=%s",
			kind,
			viewerID,
			strings.TrimSpace(req.Reason),
			snapshot.ActiveAudioViewerID,
			snapshot.ActiveInputViewerID,
		)
		if emit != nil {
			payload, _ := json.Marshal(map[string]string{
				"kind":                   kind,
				"viewer_client_id":       viewerID,
				"active_audio_viewer_id": snapshot.ActiveAudioViewerID,
				"active_input_viewer_id": snapshot.ActiveInputViewerID,
				"reason":                 strings.TrimSpace(req.Reason),
			})
			emit(orchestrator.NewEvent("viewer.active_control", "viewer", "viewer", string(payload), "VIEWER", "", "", "", ""))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":                     true,
			"active_audio_viewer_id": snapshot.ActiveAudioViewerID,
			"active_input_viewer_id": snapshot.ActiveInputViewerID,
		})
	}
}
