package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
)

func TestRegisterFeatureRoutesKeepsExistingRouteGroups(t *testing.T) {
	mux := http.NewServeMux()
	deps := &Dependencies{
		lineHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		eventHub: viewer.NewEventHub(10),
		viewerSend: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
		viewerStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
		viewerJobs: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
		viewerLogs: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusPartialContent)
		}),
		workstreamStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}),
		revenueStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		schedulerStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusResetContent)
		}),
		entryHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}),
	}
	cfg := &config.Config{WorkspaceDir: t.TempDir()}

	registerFeatureRoutes(
		mux,
		cfg,
		deps,
		sttRuntime{WSHandler: http.NotFoundHandler()},
		voiceChatRuntime{WSHandler: http.NotFoundHandler()},
		viewer.DebugSystemOptions{},
	)

	tests := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "channel webhook", method: http.MethodGet, path: "/webhook", want: http.StatusNoContent},
		{name: "viewer page", method: http.MethodGet, path: "/viewer", want: http.StatusOK},
		{name: "viewer runtime config", method: http.MethodGet, path: "/viewer/runtime-config", want: http.StatusOK},
		{name: "viewer backlog", method: http.MethodGet, path: "/viewer/backlog", want: http.StatusOK},
		{name: "viewer scheduler", method: http.MethodGet, path: "/viewer/scheduler", want: http.StatusResetContent},
		{name: "viewer ops status", method: http.MethodGet, path: "/viewer/status", want: http.StatusOK},
		{name: "viewer jobs", method: http.MethodGet, path: "/viewer/jobs", want: http.StatusAccepted},
		{name: "viewer logs", method: http.MethodGet, path: "/viewer/logs", want: http.StatusPartialContent},
		{name: "viewer workstreams", method: http.MethodGet, path: "/viewer/workstreams", want: http.StatusCreated},
		{name: "viewer revenue", method: http.MethodGet, path: "/viewer/revenue", want: http.StatusNoContent},
		{name: "viewer dynamic send", method: http.MethodGet, path: "/viewer/send", want: http.StatusAccepted},
		{name: "module manifest", method: http.MethodGet, path: moduleManifestPath, want: http.StatusOK},
		{name: "stt chat input", method: http.MethodGet, path: "/stt/chat-input", want: http.StatusMethodNotAllowed},
		{name: "entry", method: http.MethodGet, path: "/entry", want: http.StatusAccepted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			mux.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("%s %s status=%d want=%d body=%s", tt.method, tt.path, rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}
