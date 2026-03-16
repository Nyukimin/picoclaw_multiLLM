package audiorouter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestSSEClientRun_ReconnectsWithLastEventID(t *testing.T) {
	var (
		mu          sync.Mutex
		requests    []string
		firstServed bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("missing flusher")
		}
		mu.Lock()
		requests = append(requests, r.Header.Get("Last-Event-ID"))
		lastID := r.Header.Get("Last-Event-ID")
		servedFirst := firstServed
		if !firstServed {
			firstServed = true
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		if !servedFirst && lastID == "" {
			fmt.Fprint(w, "id: 1\n")
			fmt.Fprint(w, "data: {\"session_id\":\"s1\",\"chunk_index\":0,\"character_id\":\"mio\",\"audio_url\":\"http://example/1.wav\"}\n\n")
			flusher.Flush()
			return
		}
		if lastID == "1" {
			fmt.Fprint(w, "id: 2\n")
			fmt.Fprint(w, "data: {\"session_id\":\"s2\",\"chunk_index\":0,\"character_id\":\"shiro\",\"audio_url\":\"http://example/2.wav\"}\n\n")
			flusher.Flush()
			return
		}
		http.Error(w, "unexpected Last-Event-ID", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewSSEClient(SSEClientConfig{
		URL:            server.URL,
		ConnectTimeout: time.Second,
		RetryDelay:     10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var got []int64
	err := client.Run(ctx, func(id int64, ev Event) error {
		got = append(got, id)
		if len(got) == 2 {
			cancel()
		}
		if ev.AudioURL == "" {
			t.Fatalf("expected audio_url in event %+v", ev)
		}
		return nil
	})
	if err != context.Canceled && err != context.DeadlineExceeded {
		t.Fatalf("Run error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 events, got %d", len(got))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(requests))
	}
	if requests[1] != "1" {
		t.Fatalf("expected reconnect with Last-Event-ID=1, got %q", requests[1])
	}
}
