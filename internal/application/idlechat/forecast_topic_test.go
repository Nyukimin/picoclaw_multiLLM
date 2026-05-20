package idlechat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizeForecastDisplayTopicFallsBackWhenEmpty(t *testing.T) {
	domain := ForecastDomain{Name: "技術"}
	got := normalizeForecastDisplayTopic(domain, "")
	if got != "技術の3年後を考える" {
		t.Fatalf("fallback topic = %q", got)
	}
}

func TestBuildForecastLLMTopicNeverStartsWithEmptyTopic(t *testing.T) {
	domain := ForecastDomain{Name: "社会"}
	got := buildForecastLLMTopic(domain, "   ", nil)
	if !strings.Contains(got, "【社会 未来展望】社会の3年後を考える") {
		t.Fatalf("LLM topic did not include fallback display topic:\n%s", got)
	}
}

func TestFetchNewsHeadlinesFromNonOKIncludesResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nhk rss unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := fetchNewsHeadlinesFrom(srv.URL, 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "nhk rss returned status 503: nhk rss unavailable") {
		t.Fatalf("error did not include upstream body: %q", got)
	}
}

func TestFetchGoogleNewsRSSNonOKIncludesResponseBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "google news throttled", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := fetchGoogleNewsRSS(srv.URL, 3)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !strings.Contains(got, "google news rss status 429: google news throttled") {
		t.Fatalf("error did not include upstream body: %q", got)
	}
}
