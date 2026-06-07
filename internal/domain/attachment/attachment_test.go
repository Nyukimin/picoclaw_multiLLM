package attachment

import (
	"strings"
	"testing"
)

func TestKindFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        Kind
		wantOK      bool
	}{
		{"image/png", KindImage, true},
		{"image/jpeg; charset=binary", KindImage, true},
		{"video/mp4", KindVideo, true},
		{"application/pdf", KindDocument, true},
		{"text/plain", KindDocument, true},
		{"application/json", KindDocument, true},
		{"application/octet-stream", "", false},
	}

	for _, tt := range tests {
		got, ok := KindFromContentType(tt.contentType)
		if got != tt.want || ok != tt.wantOK {
			t.Fatalf("KindFromContentType(%q) = (%q, %v), want (%q, %v)", tt.contentType, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestSafeFilename(t *testing.T) {
	tests := map[string]string{
		"../secret.pdf":     "secret.pdf",
		"camera image.png":  "camera_image.png",
		"日本語 メモ.txt":        "txt",
		"////":              "attachment",
		"report.final.json": "report.final.json",
	}
	for in, want := range tests {
		if got := SafeFilename(in); got != want {
			t.Fatalf("SafeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKindFromFilename(t *testing.T) {
	if got, ok := KindFromFilename("memo.md"); got != KindDocument || !ok {
		t.Fatalf("KindFromFilename(memo.md) = (%q, %v)", got, ok)
	}
	if got, ok := KindFromFilename("camera.webp"); got != KindImage || !ok {
		t.Fatalf("KindFromFilename(camera.webp) = (%q, %v)", got, ok)
	}
	if got, ok := KindFromFilename("clip.mp4"); got != KindVideo || !ok {
		t.Fatalf("KindFromFilename(clip.mp4) = (%q, %v)", got, ok)
	}
	if _, ok := KindFromFilename("archive.zip"); ok {
		t.Fatal("KindFromFilename accepted unsupported extension")
	}
}

func TestSummaryLineIncludesExtractedDocumentTextPreview(t *testing.T) {
	got := SummaryLine(Attachment{
		Kind:          KindDocument,
		Filename:      "memo.txt",
		ContentType:   "text/plain",
		SizeBytes:     11,
		Path:          "/tmp/memo.txt",
		ExtractedText: "hello text",
	})
	if !strings.Contains(got, "本文プレビュー: hello text") {
		t.Fatalf("SummaryLine did not include extracted text preview: %q", got)
	}
}

func TestSummaryLineIncludesExtractionError(t *testing.T) {
	got := SummaryLine(Attachment{
		Kind:            KindDocument,
		Filename:        "broken.pdf",
		ContentType:     "application/pdf",
		SizeBytes:       7,
		Path:            "/tmp/broken.pdf",
		ExtractionError: "pdf text not found",
	})
	if !strings.Contains(got, "抽出エラー: pdf text not found") {
		t.Fatalf("SummaryLine did not include extraction error: %q", got)
	}
}
