package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildHTTPMessages_WithImageURLPart(t *testing.T) {
	msgs := []Message{
		{
			Role:    "user",
			Content: "この画像を見て",
			Media: []MediaRef{
				{Path: "https://example.com/img.png", MIMEType: "image/png"},
			},
		},
	}

	out := buildHTTPMessages(msgs)
	if len(out) != 1 {
		t.Fatalf("expected one message, got %d", len(out))
	}
	content, ok := out[0]["content"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected multipart content, got %#v", out[0]["content"])
	}
	if len(content) != 2 {
		t.Fatalf("expected text + image parts, got %d", len(content))
	}
	if content[1]["type"] != "image_url" {
		t.Fatalf("expected image_url type, got %#v", content[1]["type"])
	}
}

func TestBuildHTTPMessagesWithAudit_LocalImage(t *testing.T) {
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "sample.jpg")
	if err := os.WriteFile(imgPath, []byte("fake-jpeg-bytes"), 0644); err != nil {
		t.Fatalf("failed to write temp image: %v", err)
	}

	msgs := []Message{
		{
			Role:    "user",
			Content: "この画像を見て",
			Media: []MediaRef{
				{Path: imgPath, MIMEType: "image/jpeg"},
			},
		},
	}

	out, audits := buildHTTPMessagesWithAudit(msgs)
	if len(out) != 1 {
		t.Fatalf("expected one message, got %d", len(out))
	}
	content, ok := out[0]["content"].([]map[string]interface{})
	if !ok || len(content) < 2 {
		t.Fatalf("expected multipart content with image, got %#v", out[0]["content"])
	}
	if len(audits) != 1 {
		t.Fatalf("expected one audit entry, got %d", len(audits))
	}
	a := audits[0]
	if !a.Included {
		t.Fatalf("expected audit included=true, got false")
	}
	if a.URLType != "data_uri" {
		t.Fatalf("expected data_uri, got %s", a.URLType)
	}
	if !a.LocalExistsBefore {
		t.Fatalf("expected local_exists_before=true")
	}
	if a.LocalSizeBeforeBytes <= 0 {
		t.Fatalf("expected positive local_size_before_bytes, got %d", a.LocalSizeBeforeBytes)
	}
	if !strings.HasPrefix(a.ImageURL, "data:image/jpeg;base64,[omitted]") {
		t.Fatalf("unexpected image_url preview: %s", a.ImageURL)
	}
	if a.ImageURLLength <= len(a.ImageURL) {
		t.Fatalf("expected full url length greater than preview length")
	}
}

func TestEnrichAuditAfterTimeout_LocalFileExistsAndMissing(t *testing.T) {
	tmpDir := t.TempDir()
	existing := filepath.Join(tmpDir, "ok.jpg")
	if err := os.WriteFile(existing, []byte("abc"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	missing := filepath.Join(tmpDir, "missing.jpg")

	input := []imagePayloadAudit{
		{SourcePath: existing, URLType: "data_uri"},
		{SourcePath: missing, URLType: "data_uri"},
		{SourcePath: "https://example.com/x.png", URLType: "remote_url"},
	}
	got := enrichAuditAfterTimeout(input)
	if got[0].LocalExistsAfterTimer == nil || !*got[0].LocalExistsAfterTimer {
		t.Fatalf("expected existing file to remain present after timeout")
	}
	if got[0].LocalSizeAfterBytes == nil || *got[0].LocalSizeAfterBytes <= 0 {
		t.Fatalf("expected existing file size after timeout")
	}
	if got[1].LocalExistsAfterTimer == nil || *got[1].LocalExistsAfterTimer {
		t.Fatalf("expected missing file to be reported as absent")
	}
	if got[2].LocalExistsAfterTimer != nil {
		t.Fatalf("expected remote url audit to skip local existence check")
	}
}

func TestIsTimeoutError(t *testing.T) {
	if !isTimeoutError(context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded to be timeout error")
	}
	if isTimeoutError(nil) {
		t.Fatalf("expected nil error to be non-timeout")
	}
	if isTimeoutError(os.ErrNotExist) {
		t.Fatalf("expected non-timeout error for os.ErrNotExist")
	}
}
