package context

// SkillMetadata はスキルのメタデータ
type SkillMetadata struct {
	Name        string // frontmatter: name
	Description string // frontmatter: description
	DirName     string // ディレクトリ名
	BodyText    string // frontmatter 以降のテキスト

	// TOOL_CONTRACT フィールド
	ToolID           string   // frontmatter: tool_id
	Version          string   // frontmatter: version
	Category         string   // frontmatter: category (query/mutation/admin)
	RequiresApproval bool     // frontmatter: requires_approval
	DryRun           bool     // frontmatter: dry_run
	Deprecated       bool     // frontmatter: deprecated
	Invariants       []string // frontmatter: invariants (YAML list)
}
