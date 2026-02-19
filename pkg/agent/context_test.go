package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildUserContentWithMedia_IncludesMarkdownExcerpt(t *testing.T) {
	tmpDir := t.TempDir()
	mdPath := filepath.Join(tmpDir, "note.md")
	if err := os.WriteFile(mdPath, []byte("# title\nhello\n"), 0644); err != nil {
		t.Fatalf("failed to write md file: %v", err)
	}

	cb := NewContextBuilder(tmpDir)
	got := cb.buildUserContentWithMedia("please check", []string{mdPath})

	if !strings.Contains(got, "[ATTACHMENTS]") {
		t.Fatalf("expected attachments section, got: %s", got)
	}
	if !strings.Contains(got, mdPath) {
		t.Fatalf("expected attachment path in content")
	}
	if !strings.Contains(got, "[excerpt]") || !strings.Contains(got, "# title") {
		t.Fatalf("expected markdown excerpt to be embedded, got: %s", got)
	}
}

func TestBuildUserContentWithMedia_ImagesAsReferences(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	got := cb.buildUserContentWithMedia("analyze image", []string{"https://example.com/a.jpg"})

	if !strings.Contains(got, "https://example.com/a.jpg") {
		t.Fatalf("expected image path/url to be included")
	}
	if !strings.Contains(got, "[type=image]") {
		t.Fatalf("expected image type marker in content, got: %s", got)
	}
}
