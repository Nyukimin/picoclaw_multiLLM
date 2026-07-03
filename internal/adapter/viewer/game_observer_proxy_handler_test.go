package viewer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleGameObserverPageRewritesLiveEndpoint(t *testing.T) {
	uiPath := filepath.Join(t.TempDir(), "index.html")
	html := `<!doctype html><html><body><input id="liveBase" value="http://127.0.0.1:18791"><button id="loadLive">Load Live</button></body></html>`
	if err := os.WriteFile(uiPath, []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/games/observer", nil)
	HandleGameObserverPage(GameObserverProxyOptions{UIPath: uiPath}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="/viewer/games/observer-api"`) {
		t.Fatalf("observer page did not rewrite live endpoint: %s", body)
	}
	if !strings.Contains(body, "rencrowAutoLoadLiveObserver") {
		t.Fatalf("observer page did not inject live autoload script: %s", body)
	}
}

func TestHandleGameObserverProxyForwardsReadOnlyGameAPI(t *testing.T) {
	var upstreamPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(upstream.Close)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/games/observer-api/games/sessions?limit=1", nil)
	HandleGameObserverProxy(GameObserverProxyOptions{
		ObserverBaseURL: upstream.URL,
		HTTPClient:      upstream.Client(),
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamPath != "/games/sessions?limit=1" {
		t.Fatalf("upstream path=%q", upstreamPath)
	}
	if strings.TrimSpace(rec.Body.String()) != `{"ok":true}` {
		t.Fatalf("body=%q", rec.Body.String())
	}
}

func TestHandleGameObserverProxyRejectsNonGamePath(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/games/observer-api/http://example.test", nil)
	HandleGameObserverProxy(GameObserverProxyOptions{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
