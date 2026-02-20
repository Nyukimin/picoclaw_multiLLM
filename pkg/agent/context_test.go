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
	if !strings.Contains(got, "[ATTACHMENT_POLICY]") {
		t.Fatalf("expected attachment policy section, got: %s", got)
	}
	if !strings.Contains(got, "画像の初動") {
		t.Fatalf("expected default image first action, got: %s", got)
	}
}

func TestBuildUserContentWithMedia_DefaultDocumentAction(t *testing.T) {
	tmpDir := t.TempDir()
	txtPath := filepath.Join(tmpDir, "memo.txt")
	if err := os.WriteFile(txtPath, []byte("line one\nline two\n"), 0644); err != nil {
		t.Fatalf("failed to write txt file: %v", err)
	}

	cb := NewContextBuilder(tmpDir)
	got := cb.buildUserContentWithMedia("[file: memo.txt]", []string{txtPath})

	if !strings.Contains(got, "[ATTACHMENT_POLICY]") {
		t.Fatalf("expected attachment policy section, got: %s", got)
	}
	if !strings.Contains(got, "文書の初動") {
		t.Fatalf("expected default document first action, got: %s", got)
	}
}

func TestBuildUserContentWithMedia_SaveDataIntent(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	got := cb.buildUserContentWithMedia("この画像をデータとして保存して", []string{"https://example.com/a.jpg"})

	if !strings.Contains(got, "目的: データ保存") {
		t.Fatalf("expected save_data goal, got: %s", got)
	}
	if !strings.Contains(got, "data/inbox") {
		t.Fatalf("expected inbox storage path guidance, got: %s", got)
	}
}

func TestBuildMessages_AttachesImageMediaRefs(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	msgs := cb.BuildMessages(nil, "", "画像を見て", []string{"/tmp/a.jpg", "/tmp/b.txt"}, "line", "chat1", RouteChat)

	if len(msgs) == 0 {
		t.Fatalf("expected messages")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "user" {
		t.Fatalf("expected last role user, got %s", last.Role)
	}
	if len(last.Media) != 1 {
		t.Fatalf("expected one image media ref, got %d", len(last.Media))
	}
	if last.Media[0].MIMEType != "image/jpeg" {
		t.Fatalf("unexpected mime type: %s", last.Media[0].MIMEType)
	}
}

