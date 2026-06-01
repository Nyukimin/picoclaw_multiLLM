package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	modulewebgather "github.com/Nyukimin/picoclaw_multiLLM/modules/webgather"
)

type fakeWebGatherFetcher struct {
	resp modulewebgather.FetchResponse
	err  error
	req  modulewebgather.FetchRequest
}

func (f *fakeWebGatherFetcher) FetchURL(_ context.Context, req modulewebgather.FetchRequest) (modulewebgather.FetchResponse, error) {
	f.req = req
	return f.resp, f.err
}

type fakeWebGatherSearchCache struct{}

type fakeWebGatherSourceRegistry struct {
	entry conversationpersistence.L1SourceRegistryEntry
}

func (s *fakeWebGatherSourceRegistry) SaveSourceRegistryEntry(_ context.Context, entry conversationpersistence.L1SourceRegistryEntry) (*conversationpersistence.L1SourceRegistryEntry, error) {
	s.entry = entry
	return &entry, nil
}

func (fakeWebGatherSearchCache) Get(context.Context, string, string, time.Time) ([]modulewebgather.SearchResult, bool, error) {
	return nil, false, nil
}

func (fakeWebGatherSearchCache) Save(context.Context, string, string, []modulewebgather.SearchResult, time.Duration) error {
	return nil
}

func (fakeWebGatherSearchCache) SearchLocal(context.Context, string, int, time.Time) ([]modulewebgather.SearchResult, bool, error) {
	return []modulewebgather.SearchResult{}, false, nil
}

func TestRunWebGatherCommandURLJSON(t *testing.T) {
	fetcher := &fakeWebGatherFetcher{resp: modulewebgather.FetchResponse{
		Status:           "ok",
		URL:              "https://example.com",
		FinalURL:         "https://example.com",
		RawHash:          "sha256:abc",
		StagingID:        "stage-1",
		ValidationStatus: "pending",
	}}
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"url", "https://example.com", "--namespace", "kb:web", "--source-id", "web:example", "--json"}, webGatherCLIDeps{Fetcher: fetcher}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"staging_id": "stage-1"`) {
		t.Fatalf("expected JSON response, got %s", out.String())
	}
	if fetcher.req.Namespace != "kb:web" || fetcher.req.SourceID != "web:example" {
		t.Fatalf("unexpected request: %+v", fetcher.req)
	}
}

func TestRunWebGatherCommandUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"url"}, webGatherCLIDeps{Fetcher: &fakeWebGatherFetcher{}}, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "url is required") {
		t.Fatalf("expected usage error, code=%d stderr=%s", code, errOut.String())
	}
}

func TestRunWebGatherCommandFailureShowsErrorCode(t *testing.T) {
	fetcher := &fakeWebGatherFetcher{
		resp: modulewebgather.FetchResponse{
			Status:       "failed",
			ErrorCode:    modulewebgather.ErrFetchTimeout,
			ErrorMessage: "timeout",
		},
		err: modulewebgather.NewError(modulewebgather.ErrFetchTimeout, "timeout"),
	}
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"url", "https://example.com"}, webGatherCLIDeps{Fetcher: fetcher}, &out, &errOut)
	if code != 1 || !strings.Contains(errOut.String(), string(modulewebgather.ErrFetchTimeout)) {
		t.Fatalf("expected fetch timeout failure, code=%d stderr=%s", code, errOut.String())
	}
}

func TestRunWebGatherCommandSearchRequiresSearXNGURL(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"search", "ren crow", "--provider", "searxng"}, webGatherCLIDeps{}, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "web_gather.searxng_base_url or --searxng-url is required") {
		t.Fatalf("expected searxng url usage error, code=%d stderr=%s", code, errOut.String())
	}
}

func TestRunWebGatherCommandSearchUsesConfiguredSearXNGURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("q") != "ren crow" {
			t.Fatalf("unexpected request: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"url":"https://example.com","title":"Example","content":"Snippet","engine":"test"}]}`))
	}))
	defer server.Close()

	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"search", "ren crow", "--provider", "searxng", "--json"}, webGatherCLIDeps{
		SearchCache:    fakeWebGatherSearchCache{},
		SearXNGBaseURL: server.URL,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"provider": "searxng"`) || !strings.Contains(out.String(), `"url": "https://example.com"`) {
		t.Fatalf("expected searxng JSON response, got %s", out.String())
	}
}

func TestRunWebGatherCommandSearchAndFetchLocalCacheJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"search-and-fetch", "ren crow", "--provider", "local_cache", "--max-fetches", "1", "--json"}, webGatherCLIDeps{
		Fetcher:     &fakeWebGatherFetcher{},
		SearchCache: fakeWebGatherSearchCache{},
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"items": []`) {
		t.Fatalf("expected empty JSON items, got %s", out.String())
	}
}

func TestRunWebGatherCommandSearchAndFetchRequiresSearXNGURL(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"search-and-fetch", "ren crow", "--provider", "searxng"}, webGatherCLIDeps{Fetcher: &fakeWebGatherFetcher{}}, &out, &errOut)
	if code != 2 || !strings.Contains(errOut.String(), "web_gather.searxng_base_url or --searxng-url is required") {
		t.Fatalf("expected searxng url usage error, code=%d stderr=%s", code, errOut.String())
	}
}

func TestRunWebGatherCommandRegisterURL(t *testing.T) {
	registry := &fakeWebGatherSourceRegistry{}
	var out, errOut bytes.Buffer
	code := runWebGatherCommand([]string{"register-url", "https://example.com/article", "--namespace", "kb:research", "--interval-sec", "600", "--json"}, webGatherCLIDeps{
		SourceRegistry: registry,
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if registry.entry.Kind != conversationpersistence.L1SourceKindWebGather || registry.entry.URL != "https://example.com/article" {
		t.Fatalf("unexpected entry: %+v", registry.entry)
	}
	if registry.entry.SourceID == "" || registry.entry.Meta["namespace"] != "kb:research" || registry.entry.FetchInterval != 10*time.Minute {
		t.Fatalf("unexpected entry defaults: %+v", registry.entry)
	}
	if !strings.Contains(out.String(), `"kind":"web_gather"`) {
		t.Fatalf("expected JSON entry, got %s", out.String())
	}
}
