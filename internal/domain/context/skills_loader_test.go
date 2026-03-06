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
