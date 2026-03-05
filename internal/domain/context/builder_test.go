package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildContext_ChatRoute(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Agent rules"), 0644)
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("Soul values"), 0644)
	os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("Identity info"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("User prefs"), 0644)
	os.WriteFile(filepath.Join(dir, "CHAT_PERSONA.md"), []byte("Mio persona"), 0644)

	b := NewBuilder(dir)
	got := b.BuildContext("CHAT")

	for _, want := range []string{"# AGENT\nAgent rules", "# SOUL\nSoul values", "# IDENTITY\nIdentity info", "# CHAT_PERSONA\nMio persona"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in context", want)
		}
	}
}

func TestBuildContext_NonChatRoute(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Agent rules"), 0644)
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("Soul values"), 0644)
	os.WriteFile(filepath.Join(dir, "CHAT_PERSONA.md"), []byte("Mio persona"), 0644)

	b := NewBuilder(dir)
	got := b.BuildContext("CODE")

	// Chat-only files should NOT be included
	if strings.Contains(got, "SOUL") {
		t.Error("SOUL should not be in non-CHAT context")
	}
	if strings.Contains(got, "CHAT_PERSONA") {
		t.Error("CHAT_PERSONA should not be in non-CHAT context")
	}
	// Shared files should be included
	if !strings.Contains(got, "# AGENT\nAgent rules") {
		t.Error("AGENT should be in all routes")
	}
}

func TestBuildContext_WithSkills(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "weather"), 0755)
	os.WriteFile(filepath.Join(dir, "skills", "weather", "SKILL.md"), []byte("# Weather lookup"), 0644)

	b := NewBuilder(dir)
	got := b.BuildContext("CHAT")

	if !strings.Contains(got, "weather: Weather lookup") {
		t.Error("expected skills summary")
	}
}

func TestBuildContext_Empty(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder(dir)
	got := b.BuildContext("CHAT")

	if got != "" {
		t.Errorf("expected empty context, got %q", got)
	}
}

func TestBuildMessageWithTask(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Be concise"), 0644)

	b := NewBuilder(dir)
	got := b.BuildMessageWithTask("CHAT", "HEARTBEAT TASKS", "Check system status")

	if !strings.Contains(got, "# AGENT\nBe concise") {
		t.Error("expected AGENT context")
	}
	if !strings.Contains(got, "===") {
		t.Error("expected separator")
	}
	if !strings.Contains(got, "# HEARTBEAT TASKS\nCheck system status") {
		t.Error("expected task section")
	}
}

func TestBuildMessageWithTask_NoContext(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder(dir)
	got := b.BuildMessageWithTask("CHAT", "TASK", "Do something")

	if got != "Do something" {
		t.Errorf("expected plain task content, got %q", got)
	}
}

func TestBuildSkillsSummary(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "web-search"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "cron"), 0755)
	os.WriteFile(filepath.Join(dir, "skills", "web-search", "SKILL.md"), []byte("# Web search tool"), 0644)
	os.WriteFile(filepath.Join(dir, "skills", "cron", "SKILL.md"), []byte("# Scheduled tasks"), 0644)

	b := NewBuilder(dir)
	got := b.BuildSkillsSummary()

	if !strings.Contains(got, "web-search: Web search tool") {
		t.Error("expected web-search skill")
	}
	if !strings.Contains(got, "cron: Scheduled tasks") {
		t.Error("expected cron skill")
	}
}

func TestBuildSkillsSummary_Empty(t *testing.T) {
	dir := t.TempDir()
	b := NewBuilder(dir)
	got := b.BuildSkillsSummary()

	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}
