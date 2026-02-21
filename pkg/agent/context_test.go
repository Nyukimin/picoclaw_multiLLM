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
	msgs := cb.BuildMessages(nil, "", "画像を見て", []string{"/tmp/a.jpg", "/tmp/b.txt"}, "line", "chat1", RouteChat, "")

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

func TestBuildMessages_WorkOverlayInjected(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	overlay := "仕事モード指示テスト"
	msgs := cb.BuildMessages(nil, "", "こんにちは", nil, "line", "chat1", RouteChat, overlay)

	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages (system + overlay + user), got %d", len(msgs))
	}
	// overlay は system の直後、user の直前に挿入される
	overlayMsg := msgs[len(msgs)-2]
	if overlayMsg.Role != "user" {
		t.Fatalf("expected overlay role=user, got %s", overlayMsg.Role)
	}
	if overlayMsg.Content != overlay {
		t.Fatalf("expected overlay content %q, got %q", overlay, overlayMsg.Content)
	}
	lastMsg := msgs[len(msgs)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role=user, got %s", lastMsg.Role)
	}
	if !strings.Contains(lastMsg.Content, "こんにちは") {
		t.Fatalf("expected user message content, got %q", lastMsg.Content)
	}
}

func TestBuildMessages_WorkOverlayNotInjectedForNonChat(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	overlay := "仕事モード指示テスト"
	msgs := cb.BuildMessages(nil, "", "コードを書いて", nil, "line", "chat1", RouteCode, overlay)

	// CODE ルートでは overlay が挿入されないので system + user の2メッセージのみ
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system + user) for CODE route, got %d", len(msgs))
	}
	for _, m := range msgs {
		if strings.Contains(m.Content, overlay) {
			t.Fatalf("work overlay should not be injected for CODE route")
		}
	}
}

func TestBuildMessages_WorkOverlayEmptyNotInjected(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	msgs := cb.BuildMessages(nil, "", "こんにちは", nil, "line", "chat1", RouteChat, "")

	// overlay が空なら system + user の2メッセージのみ
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages when overlay is empty, got %d", len(msgs))
	}
}

func TestLoadBootstrapFiles_IncludesPrimerMessage(t *testing.T) {
	tmpDir := t.TempDir()
	primerContent := "セッション起動Primerテスト"
	if err := os.WriteFile(filepath.Join(tmpDir, "PrimerMessage.md"), []byte(primerContent), 0644); err != nil {
		t.Fatalf("failed to write PrimerMessage.md: %v", err)
	}

	cb := NewContextBuilder(tmpDir)
	got := cb.LoadBootstrapFiles()

	if !strings.Contains(got, "## PrimerMessage.md") {
		t.Fatalf("expected PrimerMessage.md section header, got: %s", got)
	}
	if !strings.Contains(got, primerContent) {
		t.Fatalf("expected PrimerMessage content, got: %s", got)
	}
}

func TestCategorizeFewShot(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"# Few-shot Example 1（実務モード）", "work"},
		{"# Few-shot Example 2（雑談モード）", "casual"},
		{"# Few-shot Example 3（実務＋温度）", "work"},
		{"# Few-shot Example 4（NG例＋修正版）", "ng"},
		{"# Few-shot Example 5（危険操作：NG例＋修正版）", "ng"},
		{"# Few-shot Example 8（メンタル落ち込み：NG例＋修正版）", "ng"},
		{"# Few-shot Example 9（メンタル：依存誘導NG）", "ng"},
		{"# Few-shot Example 10（メンタル：過度な共感NG）", "ng"},
		{"# Unknown title", "work"},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := categorizeFewShot(tt.title)
			if got != tt.want {
				t.Fatalf("categorizeFewShot(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestLoadFewShotExamples_SelectsOnePerCategory(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "FewShot_01.md"), []byte("# Example（実務モード）\nwork1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_02.md"), []byte("# Example（雑談モード）\ncasual1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_03.md"), []byte("# Example（実務＋温度）\nwork2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_04.md"), []byte("# Example（NG例＋修正版）\nng1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_05.md"), []byte("# Example（危険操作：NG例）\nng2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_06.md"), []byte("# Example（メンタル落ち込み）\nng3"), 0644)

	cb := NewContextBuilder(tmpDir)
	got := cb.LoadFewShotExamples()

	// Should contain exactly 3 sections separated by ---
	sections := strings.Split(got, "\n\n---\n\n")
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections (work+casual+ng), got %d: %s", len(sections), got)
	}

	hasWork := false
	hasCasual := false
	hasNG := false
	for _, s := range sections {
		if strings.Contains(s, "実務") {
			hasWork = true
		}
		if strings.Contains(s, "雑談") {
			hasCasual = true
		}
		if strings.Contains(s, "NG") || strings.Contains(s, "危険") || strings.Contains(s, "メンタル") {
			hasNG = true
		}
	}
	if !hasWork {
		t.Fatalf("expected a work category example")
	}
	if !hasCasual {
		t.Fatalf("expected a casual category example")
	}
	if !hasNG {
		t.Fatalf("expected an ng category example")
	}
}

func TestLoadFewShotExamples_Empty(t *testing.T) {
	cb := NewContextBuilder(t.TempDir())
	got := cb.LoadFewShotExamples()
	if got != "" {
		t.Fatalf("expected empty string when no FewShot files, got: %s", got)
	}
}

func TestLoadFewShotExamples_DifferentSeedsCanRotate(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "FewShot_01.md"), []byte("# Example（実務モード）\nwork1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_02.md"), []byte("# Example（雑談モード）\ncasual1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_03.md"), []byte("# Example（実務＋温度）\nwork2"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_04.md"), []byte("# Example（NG例1）\nng1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_05.md"), []byte("# Example（NG例2）\nng2"), 0644)

	cb := NewContextBuilder(tmpDir)
	r1 := cb.LoadFewShotExamplesWithSeed("session-a")
	r2 := cb.LoadFewShotExamplesWithSeed("session-a")

	// Same seed produces same result
	if r1 != r2 {
		t.Fatalf("same seed should produce same result")
	}

	// Both should have exactly 3 sections
	if strings.Count(r1, "\n\n---\n\n") != 2 {
		t.Fatalf("expected 2 separators (3 sections), got %d", strings.Count(r1, "\n\n---\n\n"))
	}
}

func TestBuildMessages_WorkOverlayWithFewShot(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "FewShot_01.md"), []byte("# Example（実務モード）\nwork example"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_02.md"), []byte("# Example（雑談モード）\ncasual example"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "FewShot_03.md"), []byte("# Example（NG例）\nng example"), 0644)

	cb := NewContextBuilder(tmpDir)
	overlay := "仕事モード指示"
	msgs := cb.BuildMessages(nil, "", "こんにちは", nil, "line", "chat1", RouteChat, overlay)

	if len(msgs) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(msgs))
	}
	overlayMsg := msgs[len(msgs)-2]
	if !strings.Contains(overlayMsg.Content, overlay) {
		t.Fatalf("expected overlay directive in content")
	}
	if !strings.Contains(overlayMsg.Content, "実務") {
		t.Fatalf("expected work FewShot in overlay, got: %s", overlayMsg.Content)
	}
}

func TestParseWorkCommand(t *testing.T) {
	tests := []struct {
		input    string
		wantKind string
		wantTurn int
		wantOk   bool
	}{
		{"/work", "on", DefaultWorkOverlayTurns, true},
		{"/work 12", "on", 12, true},
		{"/work off", "off", 0, true},
		{"/work status", "status", 0, true},
		{"/normal", "off", 0, true},
		{"hello", "", 0, false},
		{"/work 0", "status", 0, true},   // invalid number, falls to status
		{"/work 100", "status", 0, true},  // over limit, falls to status
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseWorkCommand(tt.input)
			if got.Ok != tt.wantOk {
				t.Fatalf("parseWorkCommand(%q).Ok = %v, want %v", tt.input, got.Ok, tt.wantOk)
			}
			if got.Ok {
				if got.Kind != tt.wantKind {
					t.Fatalf("parseWorkCommand(%q).Kind = %q, want %q", tt.input, got.Kind, tt.wantKind)
				}
				if tt.wantKind == "on" && got.Turns != tt.wantTurn {
					t.Fatalf("parseWorkCommand(%q).Turns = %d, want %d", tt.input, got.Turns, tt.wantTurn)
				}
			}
		})
	}
}

