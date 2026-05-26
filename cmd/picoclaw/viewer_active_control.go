package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

type viewerActiveControl struct {
	mu                  sync.RWMutex
	activeAudioViewerID string
	activeInputViewerID string
	activeAudioUpdated  time.Time
	activeInputUpdated  time.Time
}

type viewerActiveClaimRequest struct {
	ViewerClientID string `json:"viewer_client_id"`
	Kind           string `json:"kind"`
	Reason         string `json:"reason,omitempty"`
	Action         string `json:"action,omitempty"`
}

type viewerActiveControlSnapshot struct {
	ActiveAudioViewerID string `json:"active_audio_viewer_id"`
	ActiveInputViewerID string `json:"active_input_viewer_id"`
}

var activeViewerControl = &viewerActiveControl{}

var viewerActiveOwnerTTL = 90 * time.Second

func (c *viewerActiveControl) claim(kind, viewerClientID string) viewerActiveControlSnapshot {
	id := strings.TrimSpace(viewerClientID)
	kind = strings.TrimSpace(kind)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	switch kind {
	case "audio":
		c.activeAudioViewerID = id
		c.activeAudioUpdated = now
	case "input":
		c.activeInputViewerID = id
		c.activeInputUpdated = now
	}
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func (c *viewerActiveControl) heartbeat(kind, viewerClientID string) viewerActiveControlSnapshot {
	id := strings.TrimSpace(viewerClientID)
	kind = strings.TrimSpace(kind)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	switch kind {
	case "audio":
		if id != "" && c.activeAudioViewerID == id {
			c.activeAudioUpdated = now
		}
	case "input":
		if id != "" && c.activeInputViewerID == id {
			c.activeInputUpdated = now
		}
	}
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func (c *viewerActiveControl) release(kind, viewerClientID string) viewerActiveControlSnapshot {
	id := strings.TrimSpace(viewerClientID)
	kind = strings.TrimSpace(kind)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	switch kind {
	case "audio":
		if id != "" && c.activeAudioViewerID == id {
			c.activeAudioViewerID = ""
			c.activeAudioUpdated = time.Time{}
		}
	case "input":
		if id != "" && c.activeInputViewerID == id {
			c.activeInputViewerID = ""
			c.activeInputUpdated = time.Time{}
		}
	}
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func (c *viewerActiveControl) isActiveAudio(viewerClientID string) bool {
	id := strings.TrimSpace(viewerClientID)
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	return id != "" && c.activeAudioViewerID != "" && c.activeAudioViewerID == id
}

func (c *viewerActiveControl) snapshot() viewerActiveControlSnapshot {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneExpiredLocked(now)
	return viewerActiveControlSnapshot{
		ActiveAudioViewerID: c.activeAudioViewerID,
		ActiveInputViewerID: c.activeInputViewerID,
	}
}

func (c *viewerActiveControl) pruneExpiredLocked(now time.Time) {
	if viewerActiveOwnerTTL <= 0 || now.IsZero() {
		return
	}
	if c.activeAudioViewerID != "" && !c.activeAudioUpdated.IsZero() && now.Sub(c.activeAudioUpdated) > viewerActiveOwnerTTL {
		c.activeAudioViewerID = ""
		c.activeAudioUpdated = time.Time{}
	}
	if c.activeInputViewerID != "" && !c.activeInputUpdated.IsZero() && now.Sub(c.activeInputUpdated) > viewerActiveOwnerTTL {
		c.activeInputViewerID = ""
		c.activeInputUpdated = time.Time{}
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
		action := strings.TrimSpace(req.Action)
		if action == "" {
			action = "claim"
		}
		var snapshot viewerActiveControlSnapshot
		switch action {
		case "claim":
			snapshot = activeViewerControl.claim(kind, viewerID)
		case "heartbeat":
			snapshot = activeViewerControl.heartbeat(kind, viewerID)
		case "release":
			snapshot = activeViewerControl.release(kind, viewerID)
		default:
			http.Error(w, "action must be claim, heartbeat, or release", http.StatusBadRequest)
			return
		}
		log.Printf("[ViewerActive] %s kind=%s viewer_client_id=%s reason=%s active_audio=%s active_input=%s",
			action,
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
				"action":                 action,
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
