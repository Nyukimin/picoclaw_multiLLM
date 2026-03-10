package viewer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
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
	lastSeen := parseLastEventIDHeader(r.Header.Get("Last-Event-ID"))

	// Send history first
	for _, ev := range h.History() {
		if ev.Seq > 0 && ev.Seq <= lastSeen {
			continue
		}
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		if ev.Seq > 0 {
			fmt.Fprintf(w, "id: %d\n", ev.Seq)
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
			var ev orchestrator.OrchestratorEvent
			if err := json.Unmarshal(data, &ev); err == nil && ev.Seq > 0 {
				fmt.Fprintf(w, "id: %d\n", ev.Seq)
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func parseLastEventIDHeader(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
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
		log.Printf("[Viewer] HandleSend: received request from %s", r.RemoteAddr)

		if r.Method != http.MethodPost {
			log.Printf("[Viewer] HandleSend: method not allowed: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			log.Printf("[Viewer] HandleSend: read error: %v", err)
			http.Error(w, "read error", http.StatusBadRequest)
			return
		}

		var req struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Message == "" {
			log.Printf("[Viewer] HandleSend: invalid JSON or empty message: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		log.Printf("[Viewer] HandleSend: message received: %q", req.Message)

		// Process asynchronously — events flow back via SSE.
		// Use Background context since the HTTP response is sent immediately.
		go func() {
			log.Printf("[Viewer] HandleSend: starting async handler for message: %q", req.Message)
			response, err := handler(context.Background(), req.Message)
			if err != nil {
				log.Printf("[Viewer] HandleSend: handler error: %v", err)
			} else {
				log.Printf("[Viewer] HandleSend: handler completed successfully, response length: %d", len(response))
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
		log.Printf("[Viewer] HandleSend: sent OK response")
	}
}
