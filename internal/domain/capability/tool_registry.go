package capability

import (
	"context"
	"time"
)

// ToolSource はツールの生成元
type ToolSource string

const (
	ToolSourceBuiltin        ToolSource = "builtin"
	ToolSourceShiroGenerated ToolSource = "shiro-generated"
)

// ToolEntry はレジストリに保存されるツール定義
type ToolEntry struct {
	Name        string     // ツール名（ユニーク）
	Description string     // LLM に渡す説明文
	SchemaJSON  string     // llm.ToolDefinition の JSON 文字列
	Platforms   []string   // 対応 OS: ["linux"], ["windows"], ["linux", "windows"]
	Source      ToolSource // builtin / shiro-generated
	Trusted     bool       // 承認済みフラグ（Shiro 生成ツールは false がデフォルト）
	CreatedAt   time.Time
	CreatedBy   string // "shiro" / "builtin"
}

// ToolRegistry はツールの永続管理インターフェース
type ToolRegistry interface {
	// Register はツールを登録または更新する（冪等）
	Register(ctx context.Context, entry ToolEntry) error

	// Approve は Shiro 生成ツールを承認する（trusted = true に変更）
	Approve(ctx context.Context, name string) error

	// ListForPlatform は指定 OS で使用可能かつ承認済みのツールを返す
	ListForPlatform(ctx context.Context, platform string) ([]ToolEntry, error)

	// Get は名前でツールを取得する
	Get(ctx context.Context, name string) (ToolEntry, error)

	// Close はデータベース接続を閉じる
	Close() error
}
