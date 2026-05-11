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
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

//go:embed viewer.html assets
var viewerFS embed.FS

// HandleLogo serves the RenCrow logo image.
func HandleLogo(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, "assets/images/rencrow-logo.png")
}

// HandleMioLipSyncClosed serves Mio closed-mouth SVG.
func HandleMioLipSyncClosed(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, "assets/images/mio-lipsync-closed.svg")
}

// HandleMioLipSyncOpen serves Mio open-mouth SVG.
func HandleMioLipSyncOpen(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, "assets/images/mio-lipsync-open.svg")
}

// HandleShiroLipSyncClosed serves Shiro closed-mouth SVG.
func HandleShiroLipSyncClosed(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, "assets/images/shiro-lipsync-closed.svg")
}

// HandleShiroLipSyncOpen serves Shiro open-mouth SVG.
func HandleShiroLipSyncOpen(w http.ResponseWriter, r *http.Request) {
	serveEmbeddedAsset(w, r, "assets/images/shiro-lipsync-open.svg")
}

// HandleAsset serves modular Viewer CSS, JS, and image assets.
func HandleAsset(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/viewer/assets/")
	if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
		http.NotFound(w, r)
		return
	}
	serveEmbeddedAsset(w, r, "assets/"+name)
}

func serveEmbeddedAsset(w http.ResponseWriter, r *http.Request, name string) {
	if strings.HasSuffix(name, ".js") || strings.HasSuffix(name, ".css") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	http.ServeFileFS(w, r, viewerFS, name)
}

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

type viewerSendRequest struct {
	Message     string `json:"message"`
	ModelAlias  string `json:"model_alias,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	Model       string `json:"model,omitempty"`
	RoutePrefix string `json:"route_prefix,omitempty"`
}

type viewerLLMAliasSpec struct {
	ModelAlias  string `json:"model_alias"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	RoutePrefix string `json:"route_prefix"`
}

var viewerLLMAliasSpecs = map[string]viewerLLMAliasSpec{
	"worker": {
		ModelAlias:  "Worker",
		BaseURL:     "http://127.0.0.1:8082",
		Model:       "Worker",
		RoutePrefix: "/ops",
	},
	"coder": {
		ModelAlias:  "Coder",
		BaseURL:     "http://127.0.0.1:8082",
		Model:       "Coder",
		RoutePrefix: "/code2",
	},
	"heavy": {
		ModelAlias:  "Heavy",
		BaseURL:     "http://127.0.0.1:8083",
		Model:       "Heavy",
		RoutePrefix: "/analyze",
	},
	"wild": {
		ModelAlias:  "Wild",
		BaseURL:     "http://127.0.0.1:8084",
		Model:       "Wild",
		RoutePrefix: "/wild",
	},
}

func viewerSendAliasSpec(req viewerSendRequest) (viewerLLMAliasSpec, bool) {
	key := strings.ToLower(strings.TrimSpace(req.ModelAlias))
	if key == "" {
		key = strings.ToLower(strings.TrimSpace(req.Model))
	}
	spec, ok := viewerLLMAliasSpecs[key]
	if !ok {
		return viewerLLMAliasSpec{}, false
	}
	if v := strings.TrimSpace(req.BaseURL); v != "" {
		spec.BaseURL = v
	}
	if v := strings.TrimSpace(req.Model); v != "" {
		spec.Model = v
	}
	if v := strings.TrimSpace(req.RoutePrefix); validViewerRoutePrefix(v) {
		spec.RoutePrefix = v
	}
	return spec, ok
}

func validViewerRoutePrefix(prefix string) bool {
	switch strings.TrimSpace(prefix) {
	case "/ops", "/wild", "/heavy", "/code", "/code1", "/code2", "/code3", "/code4", "/plan", "/analyze", "/research", "/chat":
		return true
	default:
		return false
	}
}

func viewerSendHasExplicitRoute(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || trimmed[0] != '/' {
		return false
	}
	head := strings.Fields(trimmed)
	if len(head) == 0 {
		return false
	}
	switch head[0] {
	case "/ops", "/wild", "/heavy", "/code", "/code1", "/code2", "/code3", "/code4", "/plan", "/analyze", "/research", "/chat":
		return true
	default:
		return false
	}
}

func viewerEffectiveMessage(req viewerSendRequest) (string, viewerLLMAliasSpec, bool) {
	message := strings.TrimSpace(req.Message)
	spec, ok := viewerSendAliasSpec(req)
	if !ok || viewerSendHasExplicitRoute(message) {
		return message, viewerLLMAliasSpec{}, false
	}
	return spec.RoutePrefix + " " + message, spec, true
}

// HandleSend creates an HTTP handler that receives messages from the viewer input.
// onError is called with the processing error if the async handler fails (may be nil).
func HandleSend(handler MessageHandler, onError func(error)) http.HandlerFunc {
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

		var req viewerSendRequest
		if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Message) == "" {
			log.Printf("[Viewer] HandleSend: invalid JSON or empty message: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		effectiveMessage, aliasSpec, aliasApplied := viewerEffectiveMessage(req)
		if aliasApplied {
			log.Printf("[Viewer] HandleSend: message received: %q alias=%s base_url=%s model=%s route_prefix=%s",
				req.Message, aliasSpec.ModelAlias, aliasSpec.BaseURL, aliasSpec.Model, aliasSpec.RoutePrefix)
		} else {
			log.Printf("[Viewer] HandleSend: message received: %q", req.Message)
		}

		// Process asynchronously — events flow back via SSE.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			log.Printf("[Viewer] HandleSend: starting async handler for message: %q", effectiveMessage)
			response, err := handler(ctx, effectiveMessage)
			if err != nil {
				log.Printf("[Viewer] HandleSend: handler error: %v", err)
				if onError != nil {
					onError(err)
				}
			} else {
				log.Printf("[Viewer] HandleSend: handler completed successfully, response length: %d", len(response))
			}
		}()

		w.Header().Set("Content-Type", "application/json")
		if aliasApplied {
			resp := struct {
				OK          bool   `json:"ok"`
				ModelAlias  string `json:"model_alias"`
				BaseURL     string `json:"base_url"`
				Model       string `json:"model"`
				RoutePrefix string `json:"route_prefix"`
			}{
				OK:          true,
				ModelAlias:  aliasSpec.ModelAlias,
				BaseURL:     aliasSpec.BaseURL,
				Model:       aliasSpec.Model,
				RoutePrefix: aliasSpec.RoutePrefix,
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("[Viewer] HandleSend: response encode error: %v", err)
			}
		} else {
			w.Write([]byte(`{"ok":true}`))
		}
		log.Printf("[Viewer] HandleSend: sent OK response")
	}
}
