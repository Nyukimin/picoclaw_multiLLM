package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	domainattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
)

func TestAppendVisionAnalysisToMessageCallsVisionServer(t *testing.T) {
	var sawMultipart bool
	var sawPrompt string
	var sawKind string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/vision/analyze" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		sawMultipart = true
		sawPrompt = r.FormValue("prompt")
		sawKind = r.FormValue("kind")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("missing file: %v", err)
		}
		_ = file.Close()
		if header.Filename != "sample.png" {
			t.Fatalf("filename = %q", header.Filename)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"kind":    "image",
			"summary": "画像を受信しました",
			"text":    "1x1 PNGです",
			"model":   "Vision",
		})
	}))
	defer server.Close()

	cfg := config.VisionConfig{
		Enabled:         true,
		BaseURL:         server.URL,
		EndpointPath:    "/v1/vision/analyze",
		TimeoutMS:       5000,
		ModelAlias:      "Vision",
		OutputFormat:    "json",
		DefaultLanguage: "ja",
		MaxFrames:       8,
	}
	message := appendVisionAnalysisToMessage(
		context.Background(),
		cfg,
		"この画像を説明して",
		[]domainattachment.Attachment{{
			Kind:        domainattachment.KindImage,
			Filename:    "sample.png",
			ContentType: "image/png",
			Data:        []byte{0x89, 'P', 'N', 'G'},
		}},
		nil,
	)

	if !sawMultipart {
		t.Fatal("vision server was not called")
	}
	if sawPrompt != "この画像を説明して" {
		t.Fatalf("prompt = %q", sawPrompt)
	}
	if sawKind != "image" {
		t.Fatalf("kind = %q", sawKind)
	}
	for _, want := range []string{"[Vision analysis]", "file: sample.png", "summary: 画像を受信しました", "text: 1x1 PNGです"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q:\n%s", want, message)
		}
	}
}

func TestAppendVisionAnalysisToMessageRecordsFailureWithoutFailingChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"error_code":"VISION_PROVIDER_UNAVAILABLE"}`, http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := config.VisionConfig{
		Enabled:         true,
		BaseURL:         server.URL,
		EndpointPath:    "/v1/vision/analyze",
		TimeoutMS:       5000,
		ModelAlias:      "Vision",
		OutputFormat:    "json",
		DefaultLanguage: "ja",
		MaxFrames:       8,
	}
	message := appendVisionAnalysisToMessage(
		context.Background(),
		cfg,
		"解析して",
		[]domainattachment.Attachment{{
			Kind:        domainattachment.KindImage,
			Filename:    "broken.png",
			ContentType: "image/png",
			Data:        []byte("png"),
		}},
		nil,
	)

	if !strings.Contains(message, "[Vision analysis]") || !strings.Contains(message, "error: vision API status=503") {
		t.Fatalf("failure was not injected into message:\n%s", message)
	}
}
