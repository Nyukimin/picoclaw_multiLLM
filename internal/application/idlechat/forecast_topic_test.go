package idlechat

import (
	"errors"
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

func TestBuildGoogleNewsRSSSearchURLEncodesJapaneseKeyword(t *testing.T) {
	got := buildGoogleNewsRSSSearchURL("はしか感染拡大")
	if strings.Contains(got, "はしか") {
		t.Fatalf("URL should percent-encode Japanese keyword: %s", got)
	}
	if !strings.Contains(got, "q=%E3%81%AF%E3%81%97%E3%81%8B%E6%84%9F%E6%9F%93%E6%8B%A1%E5%A4%A7") {
		t.Fatalf("URL did not contain encoded query: %s", got)
	}
	if !strings.Contains(got, "hl=ja&gl=JP&ceid=JP:ja") {
		t.Fatalf("URL lost locale parameters: %s", got)
	}
}

func TestForecastLLMErrorCodeClassifiesQuotaAndRateLimit(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "quota", err: errors.New("openai error: insufficient_quota"), want: "insufficient_quota"},
		{name: "429", err: errors.New("provider returned status 429"), want: "rate_limited"},
		{name: "timeout", err: errors.New("request timeout"), want: "timeout"},
		{name: "generic", err: errors.New("boom"), want: "provider_error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := forecastLLMErrorCode(tc.err); got != tc.want {
				t.Fatalf("forecastLLMErrorCode() = %q, want %q", got, tc.want)
			}
		})
	}
}
