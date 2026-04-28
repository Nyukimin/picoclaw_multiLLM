package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInferSTTGatewayURL_PrioritizesExplicitGateway(t *testing.T) {
	got := inferSTTGatewayURL(" ws://192.168.1.36:8090/stt ", "ws://192.168.1.36:8090/stt-ws")
	want := "ws://192.168.1.36:8090/stt"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInferSTTGatewayURL_FallsBackToRencrowSTTURL(t *testing.T) {
	got := inferSTTGatewayURL("", " ws://192.168.1.36:8090/stt ")
	want := "ws://192.168.1.36:8090/stt"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInferSTTGatewayURL_EmptyWhenBothUnset(t *testing.T) {
	got := inferSTTGatewayURL(" ", " ")
	if got != "" {
		t.Fatalf("expected empty gateway url, got %q", got)
	}
}

func TestRegisterSTTRoutes_RegistersPrimaryAndCompatiblePaths(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	registerSTTRoutes(mux, handler)

	for _, path := range []string{"/stt", "/stt-ws", "/ws"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("path %s expected %d, got %d", path, http.StatusNoContent, rec.Code)
		}
	}
}
