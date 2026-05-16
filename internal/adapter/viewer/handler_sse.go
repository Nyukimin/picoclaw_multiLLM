package viewer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

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
