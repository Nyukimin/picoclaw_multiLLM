package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleSendAppliesViewerLLMAlias(t *testing.T) {
	received := make(chan string, 1)
	h := HandleSend(func(_ context.Context, message string) (string, error) {
		received <- message
		return "ok", nil
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/viewer/send", strings.NewReader(`{
		"message":"この文章を要約して",
		"model_alias":"Worker",
		"base_url":"http://127.0.0.1:8082",
		"model":"Worker",
		"route_prefix":"/ops"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		OK          bool   `json:"ok"`
		ModelAlias  string `json:"model_alias"`
		BaseURL     string `json:"base_url"`
		Model       string `json:"model"`
		RoutePrefix string `json:"route_prefix"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !body.OK || body.ModelAlias != "Worker" || body.BaseURL != "http://127.0.0.1:8082" || body.Model != "Worker" || body.RoutePrefix != "/ops" {
		t.Fatalf("unexpected response: %+v", body)
	}

	select {
	case got := <-received:
		if got != "/ops この文章を要約して" {
			t.Fatalf("unexpected handler message: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}
}

func TestHandleSendExplicitRouteWinsOverAlias(t *testing.T) {
	received := make(chan string, 1)
	h := HandleSend(func(_ context.Context, message string) (string, error) {
		received <- message
		return "ok", nil
	}, nil)

	req := httptest.NewRequest(http.MethodPost, "/viewer/send", strings.NewReader(`{
		"message":"/wild 物語にして",
		"model_alias":"Worker"
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	select {
	case got := <-received:
		if got != "/wild 物語にして" {
			t.Fatalf("unexpected handler message: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}
}
