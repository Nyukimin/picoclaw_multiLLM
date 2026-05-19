package skillgovernance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifestYAML(t *testing.T) {
	manifest := ParseManifestYAML(`skill:
  id: "core.pr-readiness"
  name: "PR Readiness"
  scope: "core"
  version: "1.0.0"
  description: "PR gate"
  human_approval_required: true
triggers:
  keywords:
    - "PR"
    - "pull request"
  intents:
    - "prepare_pr"
`)
	if manifest.SkillID != "core.pr-readiness" {
		t.Fatalf("SkillID=%q", manifest.SkillID)
	}
	if manifest.Scope != ScopeCore {
		t.Fatalf("Scope=%q", manifest.Scope)
	}
	if !manifest.HumanApprovalRequired {
		t.Fatal("expected human approval required")
	}
	if len(manifest.KeywordTriggers) != 2 || manifest.KeywordTriggers[0] != "PR" {
		t.Fatalf("KeywordTriggers=%#v", manifest.KeywordTriggers)
	}
	if len(manifest.IntentTriggers) != 1 || manifest.IntentTriggers[0] != "prepare_pr" {
		t.Fatalf("IntentTriggers=%#v", manifest.IntentTriggers)
	}
}

func TestLoadManifestsFromDirs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "core", "pr-readiness")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skill_manifest.yaml"), []byte(`skill:
  id: "core.pr-readiness"
  name: "PR Readiness"
triggers:
  keywords:
    - "PR"
`), 0644); err != nil {
		t.Fatal(err)
	}
	manifests, err := LoadManifestsFromDirs(root)
	if err != nil {
		t.Fatalf("LoadManifestsFromDirs failed: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("len=%d manifests=%#v", len(manifests), manifests)
	}
	if manifests[0].Scope != ScopeCore {
		t.Fatalf("scope=%q", manifests[0].Scope)
	}
	if manifests[0].Path != dir {
		t.Fatalf("path=%q want %q", manifests[0].Path, dir)
	}
}
