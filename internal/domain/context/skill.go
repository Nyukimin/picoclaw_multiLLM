package context

// SkillMetadata はスキルのメタデータ
type SkillMetadata struct {
	Name        string // frontmatter: name
	Description string // frontmatter: description
	DirName     string // ディレクトリ名
	BodyText    string // frontmatter 以降のテキスト
}
