package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
)

func (d *Dependencies) handleIdleChatStart() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if !d.idleChatOrch.IsManualMode() && !d.prepareIdleChatStart(w, r) {
			return
		}
		if err := d.idleChatOrch.StartManualMode(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		d.idleChatOrch.StopManualMode()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatForecast() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if !d.idleChatOrch.IsChatActive() && !d.prepareIdleChatStart(w, r) {
			return
		}
		if err := d.idleChatOrch.StartForecastMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunForecastSession()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStory() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if !d.idleChatOrch.IsChatActive() && !d.prepareIdleChatStart(w, r) {
			return
		}
		if err := d.idleChatOrch.StartStoryMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunSimpleStorySession()
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
		})
	}
}

func (d *Dependencies) handleIdleChatStorySimple() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		if !d.idleChatOrch.IsChatActive() && !d.prepareIdleChatStart(w, r) {
			return
		}
		if err := d.idleChatOrch.StartSimpleStoryMode(); err != nil {
			status := http.StatusBadRequest
			if strings.Contains(err.Error(), "already active") {
				status = http.StatusConflict
			}
			http.Error(w, err.Error(), status)
			return
		}
		go d.idleChatOrch.RunSimpleStorySession()
		writeJSON(w, map[string]any{
			"ok":          true,
			"mode":        d.idleChatOrch.CurrentMode(),
			"chat_active": d.idleChatOrch.IsChatActive(),
		})
	}
}

func (d *Dependencies) prepareIdleChatStart(w http.ResponseWriter, r *http.Request) bool {
	if d.idleChatStartGate == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 650*time.Second)
	defer cancel()
	if err := d.idleChatStartGate.PrepareIdleChatStart(ctx); err != nil {
		var busy *viewer.LLMOpsIdleChatBusyError
		if errors.As(err, &busy) {
			http.Error(w, err.Error(), http.StatusConflict)
			return false
		}
		log.Printf("[IdleChat] llm ops prepare failed: %v", err)
		http.Error(w, "idlechat llm runtime prepare failed", http.StatusBadGateway)
		return false
	}
	return true
}

func (d *Dependencies) handleIdleChatLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if d.idleChatOrch == nil {
			http.Error(w, "idlechat not enabled", http.StatusNotFound)
			return
		}
		limit := 20
		if s := r.URL.Query().Get("limit"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		writeJSON(w, map[string]any{
			"ok":            true,
			"mode":          d.idleChatOrch.CurrentMode(),
			"manual_mode":   d.idleChatOrch.IsManualMode(),
			"chat_active":   d.idleChatOrch.IsChatActive(),
			"current_topic": d.idleChatOrch.CurrentTopic(),
			"history":       d.idleChatOrch.GetHistory(limit),
		})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
