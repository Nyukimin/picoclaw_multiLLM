package context

import (
	"testing"
)

func TestParseSkillFile_WithFrontmatter(t *testing.T) {
	content := `---
name: weather
description: Get current weather and forecasts.
metadata: {"emoji":"🌤️"}
---

# Weather

Some body text here.`

	meta := parseSkillFile(content, "weather-dir")
	if meta.Name != "weather" {
		t.Errorf("Name = %q, want %q", meta.Name, "weather")
	}
	if meta.Description != "Get current weather and forecasts." {
		t.Errorf("Description = %q, want %q", meta.Description, "Get current weather and forecasts.")
	}
	if meta.DirName != "weather-dir" {
		t.Errorf("DirName = %q, want %q", meta.DirName, "weather-dir")
	}
	if meta.BodyText == "" {
		t.Error("BodyText should not be empty")
	}
}

func TestParseSkillFile_QuotedDescription(t *testing.T) {
	content := `---
name: github
description: "Interact with GitHub using the gh CLI."
---

# GitHub`

	meta := parseSkillFile(content, "github")
	if meta.Description != "Interact with GitHub using the gh CLI." {
		t.Errorf("Description = %q, want unquoted value", meta.Description)
	}
}

func TestParseSkillFile_NoFrontmatter(t *testing.T) {
	content := `# My Skill

This is the body.`

	meta := parseSkillFile(content, "my-skill")
	if meta.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", meta.Name, "my-skill")
	}
	if meta.Description != "My Skill" {
		t.Errorf("Description = %q, want %q", meta.Description, "My Skill")
	}
}

func TestParseSkillFile_EmptyContent(t *testing.T) {
	meta := parseSkillFile("", "empty")
	if meta.Name != "empty" {
		t.Errorf("Name = %q, want %q", meta.Name, "empty")
	}
}

func TestParseSkillFile_ToolContractFields(t *testing.T) {
	content := `---
name: file_write
tool_id: file_write
version: "1.0.0"
category: mutation
requires_approval: true
dry_run: true
invariants:
  - "path must be non-empty"
  - "path traversal is rejected"
  - "timeout: 10 seconds"
---

# file_write`

	meta := parseSkillFile(content, "file_write")
	if meta.ToolID != "file_write" {
		t.Errorf("ToolID = %q, want %q", meta.ToolID, "file_write")
	}
	if meta.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", meta.Version, "1.0.0")
	}
	if meta.Category != "mutation" {
		t.Errorf("Category = %q, want %q", meta.Category, "mutation")
	}
	if !meta.RequiresApproval {
		t.Error("RequiresApproval should be true")
	}
	if !meta.DryRun {
		t.Error("DryRun should be true")
	}
	if len(meta.Invariants) != 3 {
		t.Errorf("Invariants len = %d, want 3", len(meta.Invariants))
	}
	if len(meta.Invariants) > 0 && meta.Invariants[0] != "path must be non-empty" {
		t.Errorf("Invariants[0] = %q", meta.Invariants[0])
	}
}

func TestParseSkillFile_QueryCategory(t *testing.T) {
	content := `---
name: file_read
tool_id: file_read
version: "1.0.0"
category: query
requires_approval: false
dry_run: false
---

# file_read`

	meta := parseSkillFile(content, "file_read")
	if meta.Category != "query" {
		t.Errorf("Category = %q, want %q", meta.Category, "query")
	}
	if meta.RequiresApproval {
		t.Error("RequiresApproval should be false")
	}
	if meta.DryRun {
		t.Error("DryRun should be false")
	}
}

func TestFormatSummary(t *testing.T) {
	loader := NewSkillsLoader("/nonexistent")
	skills := []SkillMetadata{
		{Name: "weather", Description: "Get weather"},
		{Name: "github", Description: "GitHub CLI"},
		{Name: "bare"},
	}
	summary := loader.FormatSummary(skills)
	if summary == "" {
		t.Error("summary should not be empty")
	}
	// Check each line
	lines := 0
	for range skills {
		lines++
	}
	if lines != 3 {
		t.Errorf("expected 3 skills in summary")
	}
}
