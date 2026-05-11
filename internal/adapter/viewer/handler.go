package viewer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	appattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/application/attachment"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	domainattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
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
type MessageHandler func(ctx context.Context, req SendRequest) (string, error)

// AttachmentSaver persists uploaded Viewer files before they enter orchestration.
type AttachmentSaver interface {
	SaveAll(ctx context.Context, files []appattachment.IncomingFile) ([]domainattachment.Attachment, error)
}

// SendRequest is the adapter-neutral payload passed from Viewer to orchestration.
type SendRequest struct {
	Message     string
	Attachments []domainattachment.Attachment
}

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
	return HandleSendWithAttachments(handler, onError, nil)
}

// HandleSendWithAttachments receives text and optional file attachments from the Viewer.
func HandleSendWithAttachments(handler MessageHandler, onError func(error), saver AttachmentSaver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Viewer] HandleSend: received request from %s", r.RemoteAddr)

		if r.Method != http.MethodPost {
			log.Printf("[Viewer] HandleSend: method not allowed: %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		req, attachments, err := parseViewerSendRequest(r, saver)
		if err != nil {
			log.Printf("[Viewer] HandleSend: invalid request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Message) == "" && len(attachments) == 0 {
			log.Printf("[Viewer] HandleSend: empty message and no attachments")
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		effectiveMessage, aliasSpec, aliasApplied := viewerEffectiveMessage(req)
		if strings.TrimSpace(effectiveMessage) == "" && len(attachments) > 0 {
			effectiveMessage = "添付ファイルを確認してください。"
		}
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
			response, err := handler(ctx, SendRequest{Message: effectiveMessage, Attachments: attachments})
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
				Attachments int    `json:"attachment_count"`
			}{
				OK:          true,
				ModelAlias:  aliasSpec.ModelAlias,
				BaseURL:     aliasSpec.BaseURL,
				Model:       aliasSpec.Model,
				RoutePrefix: aliasSpec.RoutePrefix,
				Attachments: len(attachments),
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("[Viewer] HandleSend: response encode error: %v", err)
			}
		} else {
			resp := struct {
				OK          bool `json:"ok"`
				Attachments int  `json:"attachment_count"`
			}{OK: true, Attachments: len(attachments)}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				log.Printf("[Viewer] HandleSend: response encode error: %v", err)
			}
		}
		log.Printf("[Viewer] HandleSend: sent OK response")
	}
}

func parseViewerSendRequest(r *http.Request, saver AttachmentSaver) (viewerSendRequest, []domainattachment.Attachment, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return parseViewerMultipartSendRequest(r, saver)
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		return viewerSendRequest{}, nil, fmt.Errorf("read body: %w", err)
	}
	var req viewerSendRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return viewerSendRequest{}, nil, fmt.Errorf("json decode: %w", err)
	}
	return req, nil, nil
}

func parseViewerMultipartSendRequest(r *http.Request, saver AttachmentSaver) (viewerSendRequest, []domainattachment.Attachment, error) {
	if saver == nil {
		return viewerSendRequest{}, nil, fmt.Errorf("attachment saver is nil")
	}
	if err := r.ParseMultipartForm(domainattachment.DefaultLimits.MaxTotalBytes + (1 << 20)); err != nil {
		return viewerSendRequest{}, nil, fmt.Errorf("parse multipart: %w", err)
	}
	req := viewerSendRequest{
		Message:     r.FormValue("message"),
		ModelAlias:  r.FormValue("model_alias"),
		BaseURL:     r.FormValue("base_url"),
		Model:       r.FormValue("model"),
		RoutePrefix: r.FormValue("route_prefix"),
	}

	files, err := incomingViewerFiles(r.MultipartForm)
	if err != nil {
		return viewerSendRequest{}, nil, err
	}
	attachments, err := saver.SaveAll(r.Context(), files)
	if err != nil {
		return viewerSendRequest{}, nil, err
	}
	return req, attachments, nil
}

func incomingViewerFiles(form *multipart.Form) ([]appattachment.IncomingFile, error) {
	if form == nil || len(form.File) == 0 {
		return nil, nil
	}
	headers := append([]*multipart.FileHeader{}, form.File["attachments"]...)
	headers = append(headers, form.File["attachments[]"]...)
	files := make([]appattachment.IncomingFile, 0, len(headers))
	for _, fh := range headers {
		f, err := fh.Open()
		if err != nil {
			return nil, fmt.Errorf("open attachment: %w", err)
		}
		files = append(files, appattachment.IncomingFile{
			Filename:    fh.Filename,
			ContentType: fh.Header.Get("Content-Type"),
			SizeBytes:   fh.Size,
			Reader:      f,
		})
	}
	return files, nil
}
