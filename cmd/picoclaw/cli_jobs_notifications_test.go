package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunJobsNotificationsPrintsInterrupts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/viewer/job-notifications" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"type":"job.status","level":"done","job_id":"job_1","title":"Done","assignee":"shiro","route":"CODE","status":"succeeded","summary":"ok","interrupt":true,"created_at":"2026-06-18T00:00:00Z"}]}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runJobsCommand([]string{"notifications", "--url", srv.URL}, &out, &errOut, srv.Client())
	if code != 0 {
		t.Fatalf("runJobsCommand code=%d stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "[Shiro interrupt] CODE / succeeded / job_1") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestParseJobsNotificationsOptions(t *testing.T) {
	opts, err := parseJobsNotificationsOptions([]string{"--url", "http://example.test", "--watch", "--interval", "5s", "--json", "7"})
	if err != nil {
		t.Fatalf("parseJobsNotificationsOptions: %v", err)
	}
	if opts.BaseURL != "http://example.test" || !opts.Watch || !opts.JSON || opts.Interval != 5*time.Second || opts.Limit != 7 {
		t.Fatalf("unexpected opts: %#v", opts)
	}
}
