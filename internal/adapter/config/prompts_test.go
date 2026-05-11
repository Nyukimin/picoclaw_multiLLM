package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPromptsLoadsCharacterBundlesFromWorkspace(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := t.TempDir()

	writeCharacterBundle(t, workspaceDir, "mio", map[string]string{
		"00_system.md":    "mio system",
		"10_policy.md":    "mio policy",
		"20_routing.md":   "mio routing",
		"30_knowledge.md": "mio knowledge",
	})
	writeCharacterBundle(t, workspaceDir, "shiro", map[string]string{
		"00_system.md": "shiro system",
		"10_policy.md": "shiro policy",
	})
	writeCharacterBundle(t, workspaceDir, "kuro", map[string]string{
		"00_system.md": "kuro system",
	})
	writeCharacterBundle(t, workspaceDir, "midori", map[string]string{
		"00_system.md": "midori system",
	})
	for _, name := range []string{"aka", "ao", "gin", "kin"} {
		writeCharacterBundle(t, workspaceDir, name, map[string]string{
			"00_system.md": name + " system",
		})
	}

	p := LoadPrompts(baseDir, workspaceDir)

	if !strings.Contains(p.MioPersona, "mio system") || !strings.Contains(p.MioPersona, "mio knowledge") {
		t.Fatalf("MioPersona did not load mio bundle:\n%s", p.MioPersona)
	}
	if !strings.Contains(p.Worker, "shiro system") || !strings.Contains(p.Worker, "shiro policy") {
		t.Fatalf("Worker did not load shiro bundle:\n%s", p.Worker)
	}
	if !strings.Contains(p.Heavy, "kuro system") {
		t.Fatalf("Heavy did not load kuro bundle:\n%s", p.Heavy)
	}
	if !strings.Contains(p.Wild, "midori system") {
		t.Fatalf("Wild did not load midori bundle:\n%s", p.Wild)
	}
	if got := p.CharacterPrompts["mio"]; !strings.Contains(got, "mio routing") {
		t.Fatalf("CharacterPrompts[mio] missing bundle content:\n%s", got)
	}
	for _, name := range []string{"aka", "ao", "gin", "kin"} {
		if got := p.CharacterPrompts[name]; !strings.Contains(got, name+" system") {
			t.Fatalf("CharacterPrompts[%s] missing bundle content:\n%s", name, got)
		}
	}
}

func TestLoadPromptsCharacterBundleOverridesLegacyPrompt(t *testing.T) {
	baseDir := t.TempDir()
	workspaceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspaceDir, "worker.md"), []byte("legacy worker"), 0o644); err != nil {
		t.Fatalf("write legacy worker: %v", err)
	}
	writeCharacterBundle(t, workspaceDir, "shiro", map[string]string{
		"00_system.md": "character shiro",
	})

	p := LoadPrompts(baseDir, workspaceDir)

	if strings.Contains(p.Worker, "legacy worker") || !strings.Contains(p.Worker, "character shiro") {
		t.Fatalf("character shiro bundle should override legacy worker prompt:\n%s", p.Worker)
	}
}

func writeCharacterBundle(t *testing.T, workspaceDir, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(workspaceDir, "prompts", "characters", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir character bundle: %v", err)
	}
	manifest := make([]string, 0, len(files))
	for filename, content := range files {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", filename, err)
		}
		manifest = append(manifest, filename)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(strings.Join(manifest, "\n")), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
