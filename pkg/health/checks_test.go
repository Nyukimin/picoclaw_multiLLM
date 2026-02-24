// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOllamaCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checkFn := OllamaCheck(server.URL, 5*time.Second)
	ok, msg := checkFn()

	if !ok {
		t.Errorf("Expected ok=true, got ok=false with message: %s", msg)
	}
	if msg != "ok" {
		t.Errorf("Expected msg='ok', got msg='%s'", msg)
	}
}

func TestOllamaCheck_Unreachable(t *testing.T) {
	// Use an invalid URL to simulate unreachable server
	checkFn := OllamaCheck("http://localhost:99999", 1*time.Second)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false for unreachable server, got ok=true")
	}
	if !strings.Contains(msg, "unreachable") {
		t.Errorf("Expected message to contain 'unreachable', got: %s", msg)
	}
}

func TestOllamaCheck_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checkFn := OllamaCheck(server.URL, 5*time.Second)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false for 500 status, got ok=true")
	}
	if !strings.Contains(msg, "status 500") {
		t.Errorf("Expected message to contain 'status 500', got: %s", msg)
	}
}

func TestOllamaModelsCheck_AllModelsLoaded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/ps") {
			t.Errorf("Expected path to end with /api/ps, got: %s", r.URL.Path)
		}
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 8192},
				{Name: "worker-v1:latest", ContextLength: 8192},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MaxContext: 8192},
		{Name: "worker-v1:latest", MaxContext: 8192},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if !ok {
		t.Errorf("Expected ok=true, got ok=false with message: %s", msg)
	}
	if !strings.Contains(msg, "2/2 models ok") {
		t.Errorf("Expected message to contain '2/2 models ok', got: %s", msg)
	}
}

func TestOllamaModelsCheck_ModelMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 8192},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MaxContext: 8192},
		{Name: "worker-v1:latest", MaxContext: 8192},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false when model is missing, got ok=true")
	}
	if !strings.Contains(msg, "not loaded") || !strings.Contains(msg, "worker-v1:latest") {
		t.Errorf("Expected message to contain 'not loaded: worker-v1:latest', got: %s", msg)
	}
}

func TestOllamaModelsCheck_MaxContextExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 131072}, // Exceeds MaxContext
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MaxContext: 8192},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false when MaxContext is exceeded, got ok=true")
	}
	if !strings.Contains(msg, "context mismatch") {
		t.Errorf("Expected message to contain 'context mismatch', got: %s", msg)
	}
	if !strings.Contains(msg, "ctx=131072") || !strings.Contains(msg, "want<=8192") {
		t.Errorf("Expected message to show ctx=131072,want<=8192, got: %s", msg)
	}
}

func TestOllamaModelsCheck_MinContextNotMet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 2048}, // Below MinContext
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MinContext: 4096},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false when MinContext is not met, got ok=true")
	}
	if !strings.Contains(msg, "context mismatch") {
		t.Errorf("Expected message to contain 'context mismatch', got: %s", msg)
	}
	if !strings.Contains(msg, "ctx=2048") || !strings.Contains(msg, "want>=4096") {
		t.Errorf("Expected message to show ctx=2048,want>=4096, got: %s", msg)
	}
}

func TestOllamaModelsCheck_ContextInRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 8192},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MinContext: 4096, MaxContext: 16384},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if !ok {
		t.Errorf("Expected ok=true when context is in range, got ok=false with message: %s", msg)
	}
	if !strings.Contains(msg, "1/1 models ok") {
		t.Errorf("Expected message to contain '1/1 models ok', got: %s", msg)
	}
}

func TestOllamaModelsCheck_MultipleContextViolations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 131072}, // Exceeds max
				{Name: "worker-v1:latest", ContextLength: 2048}, // Below min
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{
		{Name: "chat-v1:latest", MaxContext: 8192},
		{Name: "worker-v1:latest", MinContext: 4096},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if ok {
		t.Error("Expected ok=false when multiple context violations exist, got ok=true")
	}
	if !strings.Contains(msg, "context mismatch") {
		t.Errorf("Expected message to contain 'context mismatch', got: %s", msg)
	}
	// Both violations should be reported
	if !strings.Contains(msg, "chat-v1:latest") || !strings.Contains(msg, "worker-v1:latest") {
		t.Errorf("Expected message to contain both model names, got: %s", msg)
	}
}

func TestOllamaModelsCheck_NoRequirements(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	required := []ModelRequirement{}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if !ok {
		t.Errorf("Expected ok=true when no requirements specified, got ok=false with message: %s", msg)
	}
	if !strings.Contains(msg, "0/0 models ok") {
		t.Errorf("Expected message to contain '0/0 models ok', got: %s", msg)
	}
}

func TestOllamaModelsCheck_ZeroConstraintsIgnored(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaPsResponse{
			Models: []struct {
				Name          string `json:"name"`
				ContextLength int    `json:"context_length"`
			}{
				{Name: "chat-v1:latest", ContextLength: 131072},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// MinContext and MaxContext are 0, so constraints should be ignored
	required := []ModelRequirement{
		{Name: "chat-v1:latest", MinContext: 0, MaxContext: 0},
	}

	checkFn := OllamaModelsCheck(server.URL, 5*time.Second, required)
	ok, msg := checkFn()

	if !ok {
		t.Errorf("Expected ok=true when constraints are 0 (ignored), got ok=false with message: %s", msg)
	}
	if !strings.Contains(msg, "1/1 models ok") {
		t.Errorf("Expected message to contain '1/1 models ok', got: %s", msg)
	}
}
