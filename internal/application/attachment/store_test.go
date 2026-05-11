package attachment

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domainattachment "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/attachment"
)

func TestStoreSaveAllPersistsSupportedAttachments(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	store.Now = func() time.Time { return time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC) }
	store.NewID = func() string { return "att-1" }

	got, err := store.SaveAll(context.Background(), []IncomingFile{
		{
			Filename:    "../camera image.png",
			ContentType: "image/png",
			Reader:      strings.NewReader("png-data"),
		},
	})
	if err != nil {
		t.Fatalf("SaveAll returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("SaveAll returned %d attachments, want 1", len(got))
	}
	att := got[0]
	if att.Kind != domainattachment.KindImage || att.Filename != "camera_image.png" || att.SizeBytes != int64(len("png-data")) {
		t.Fatalf("unexpected attachment: %#v", att)
	}
	wantPath := filepath.Join(dir, "viewer_uploads", "20260511", "viewer", "att-1", "camera_image.png")
	if att.Path != wantPath {
		t.Fatalf("Path = %q, want %q", att.Path, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("stored file was not readable: %v", err)
	}
	if string(data) != "png-data" {
		t.Fatalf("stored data = %q", string(data))
	}
}

func TestStoreSaveAllRejectsUnsupportedContentType(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.SaveAll(context.Background(), []IncomingFile{
		{Filename: "data.bin", ContentType: "application/octet-stream", Reader: strings.NewReader("bin")},
	})
	if err == nil {
		t.Fatal("SaveAll returned nil error for unsupported content type")
	}
}

func TestStoreSaveAllAcceptsKnownExtensionWhenContentTypeIsOctetStream(t *testing.T) {
	store := NewStore(t.TempDir())
	store.NewID = func() string { return "att-1" }

	got, err := store.SaveAll(context.Background(), []IncomingFile{
		{Filename: "memo.md", ContentType: "application/octet-stream", Reader: strings.NewReader("# memo")},
	})
	if err != nil {
		t.Fatalf("SaveAll returned error: %v", err)
	}
	if got[0].Kind != domainattachment.KindDocument {
		t.Fatalf("Kind = %q, want %q", got[0].Kind, domainattachment.KindDocument)
	}
}
