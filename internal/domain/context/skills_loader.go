package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillsLoader は skills/ ディレクトリからスキルメタデータを読み込む
type SkillsLoader struct {
	skillsDir string
}

// NewSkillsLoader は新しい SkillsLoader を作成
func NewSkillsLoader(skillsDir string) *SkillsLoader {
	return &SkillsLoader{skillsDir: skillsDir}
}

// LoadAll はすべてのスキルメタデータを読み込む
func (l *SkillsLoader) LoadAll() ([]SkillMetadata, error) {
	entries, err := os.ReadDir(l.skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []SkillMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillFile := filepath.Join(l.skillsDir, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		meta := parseSkillFile(string(data), entry.Name())
		if meta.Name != "" {
			skills = append(skills, meta)
		}
	}
	return skills, nil
}

// FormatSummary はスキル一覧を人間可読なテキストに変換する
func (l *SkillsLoader) FormatSummary(skills []SkillMetadata) string {
	var lines []string
	for _, s := range skills {
		if s.Description != "" {
			lines = append(lines, fmt.Sprintf("- %s: %s", s.Name, s.Description))
		} else {
			lines = append(lines, fmt.Sprintf("- %s", s.Name))
		}
	}
	return strings.Join(lines, "\n")
}

// parseSkillFile は SKILL.md の内容をパースする
// YAML frontmatter（---区切り）のキーバリューを解析
func parseSkillFile(content string, dirName string) SkillMetadata {
	meta := SkillMetadata{DirName: dirName}

	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		// frontmatter なし: 先頭行をタイトルとして使用
		firstLine, body, _ := strings.Cut(content, "\n")
		meta.Name = dirName
		meta.Description = strings.TrimPrefix(strings.TrimSpace(firstLine), "# ")
		meta.BodyText = strings.TrimSpace(body)
		return meta
	}

	// frontmatter を抽出
	rest := content[3:] // "---" を除去
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return meta
	}

	frontmatter := rest[:endIdx]
	meta.BodyText = strings.TrimSpace(rest[endIdx+4:]) // "---\n" を除去

	// key: value 行を解析（YAML list も簡易サポート）
	lines := strings.Split(frontmatter, "\n")
	var currentListKey string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			currentListKey = ""
			continue
		}

		// YAML list item: "  - item"
		if currentListKey != "" && strings.HasPrefix(trimmed, "- ") {
			item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			item = strings.Trim(item, "\"'")
			if currentListKey == "invariants" {
				meta.Invariants = append(meta.Invariants, item)
			}
			continue
		}

		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			currentListKey = ""
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// クォートを除去
		value = strings.Trim(value, "\"'")

		switch key {
		case "name":
			meta.Name = value
		case "description":
			meta.Description = value
		case "tool_id":
			meta.ToolID = value
		case "version":
			meta.Version = value
		case "category":
			meta.Category = value
		case "requires_approval":
			meta.RequiresApproval = value == "true"
		case "dry_run":
			meta.DryRun = value == "true"
		case "deprecated":
			meta.Deprecated = value == "true"
		case "invariants":
			// 次の行が "- " で始まるリストアイテム
			currentListKey = "invariants"
		default:
			currentListKey = ""
		}
	}

	if meta.Name == "" {
		meta.Name = dirName
	}

	return meta
}
