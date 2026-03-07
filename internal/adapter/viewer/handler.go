package viewer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

//go:embed viewer.html
var viewerFS embed.FS

// HandleSSE streams orchestrator events to the client via Server-Sent Events.
func (h *EventHub) HandleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	// Send history first
	for _, ev := range h.History() {
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	// Stream new events
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// HandlePage serves the single-page viewer HTML.
func HandlePage(w http.ResponseWriter, r *http.Request) {
	data, err := viewerFS.ReadFile("viewer.html")
	if err != nil {
		http.Error(w, "viewer page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// MessageHandler processes a user message from the viewer.
type MessageHandler func(ctx context.Context, message string) (string, error)

// HandleSend creates an HTTP handler that receives messages from the viewer input.
func HandleSend(handler MessageHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var req struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Message == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		// Process asynchronously — events flow back via SSE.
		// Use Background context since the HTTP response is sent immediately.
		go handler(context.Background(), req.Message)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}
