package attachment

import "testing"

func TestKindFromContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        Kind
		wantOK      bool
	}{
		{"image/png", KindImage, true},
		{"image/jpeg; charset=binary", KindImage, true},
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
	if _, ok := KindFromFilename("archive.zip"); ok {
		t.Fatal("KindFromFilename accepted unsupported extension")
	}
}
