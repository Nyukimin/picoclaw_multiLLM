package toolregistry

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBToolRegistryStore は DuckDB を使った ToolRegistry 実装
type DuckDBToolRegistryStore struct {
	db *sql.DB
}

// NewDuckDBToolRegistryStore は新しい DuckDBToolRegistryStore を作成する。
// dbPath が空の場合はインメモリ DB（":memory:"）を使用する。
func NewDuckDBToolRegistryStore(dbPath string) (*DuckDBToolRegistryStore, error) {
	if dbPath == "" {
		dbPath = ":memory:"
	}
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	store := &DuckDBToolRegistryStore{db: db}
	if err := store.initTables(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tool_registry table: %w", err)
	}
	return store, nil
}

// Close はデータベース接続を閉じる
func (s *DuckDBToolRegistryStore) Close() error {
	return s.db.Close()
}

// initTables は tool_registry テーブルを初期化する
func (s *DuckDBToolRegistryStore) initTables(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS tool_registry (
		name         TEXT PRIMARY KEY,
		description  TEXT NOT NULL,
		schema_json  TEXT NOT NULL,
		platforms    TEXT NOT NULL,
		source       TEXT NOT NULL,
		trusted      BOOLEAN NOT NULL DEFAULT FALSE,
		created_at   TIMESTAMP NOT NULL,
		created_by   TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_tool_registry_trusted ON tool_registry (trusted);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Register はツールを登録または更新する（name が同じ場合は上書き）
func (s *DuckDBToolRegistryStore) Register(ctx context.Context, entry capability.ToolEntry) error {
	platformsJSON, err := json.Marshal(entry.Platforms)
	if err != nil {
		return fmt.Errorf("marshal platforms: %w", err)
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	query := `
	INSERT INTO tool_registry (name, description, schema_json, platforms, source, trusted, created_at, created_by)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT (name) DO UPDATE SET
		description = excluded.description,
		schema_json = excluded.schema_json,
		platforms   = excluded.platforms,
		source      = excluded.source,
		created_by  = excluded.created_by
	`
	_, err = s.db.ExecContext(ctx, query,
		entry.Name,
		entry.Description,
		entry.SchemaJSON,
		string(platformsJSON),
		string(entry.Source),
		entry.Trusted,
		entry.CreatedAt,
		entry.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("register tool %q: %w", entry.Name, err)
	}
	return nil
}

// Approve は指定ツールを承認済み（trusted = true）にする。
// DuckDB の ART index 制限（PRIMARY KEY のある表の UPDATE 非対応）のため
// DELETE + INSERT で実装する。
func (s *DuckDBToolRegistryStore) Approve(ctx context.Context, name string) error {
	existing, err := s.Get(ctx, name)
	if err != nil {
		return err // "not found" を含む
	}

	platformsJSON, err := json.Marshal(existing.Platforms)
	if err != nil {
		return fmt.Errorf("marshal platforms: %w", err)
	}

	// DuckDB の ART index 制限: トランザクション内の DELETE+INSERT も
	// 同一 PRIMARY KEY を拒否するため、2 ステップを別々に実行する。
	if _, err := s.db.ExecContext(ctx, `DELETE FROM tool_registry WHERE name = ?`, name); err != nil {
		return fmt.Errorf("approve (delete) %q: %w", name, err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO tool_registry (name, description, schema_json, platforms, source, trusted, created_at, created_by)
		VALUES (?, ?, ?, ?, ?, true, ?, ?)`,
		existing.Name, existing.Description, existing.SchemaJSON,
		string(platformsJSON), string(existing.Source),
		existing.CreatedAt, existing.CreatedBy,
	); err != nil {
		return fmt.Errorf("approve (insert) %q: %w", name, err)
	}
	return nil
}

// ListForPlatform は指定プラットフォームに対応する承認済みツールを返す
func (s *DuckDBToolRegistryStore) ListForPlatform(ctx context.Context, platform string) ([]capability.ToolEntry, error) {
	// platforms は JSON 配列文字列なので LIKE で platform 名を含むかチェック
	rows, err := s.db.QueryContext(ctx, `
		SELECT name, description, schema_json, platforms, source, trusted, created_at, created_by
		FROM tool_registry
		WHERE trusted = true
		  AND platforms LIKE ?
		ORDER BY name
	`, "%\""+platform+"\"%")
	if err != nil {
		return nil, fmt.Errorf("list tools for platform %q: %w", platform, err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

// Get は名前でツールを取得する
func (s *DuckDBToolRegistryStore) Get(ctx context.Context, name string) (capability.ToolEntry, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT name, description, schema_json, platforms, source, trusted, created_at, created_by
		FROM tool_registry WHERE name = ?
	`, name)

	var e capability.ToolEntry
	var platformsJSON, source string
	var createdAt time.Time

	if err := row.Scan(
		&e.Name, &e.Description, &e.SchemaJSON,
		&platformsJSON, &source, &e.Trusted, &createdAt, &e.CreatedBy,
	); err == sql.ErrNoRows {
		return capability.ToolEntry{}, fmt.Errorf("tool %q not found", name)
	} else if err != nil {
		return capability.ToolEntry{}, fmt.Errorf("get tool %q: %w", name, err)
	}

	if err := json.Unmarshal([]byte(platformsJSON), &e.Platforms); err != nil {
		e.Platforms = []string{}
	}
	e.Source = capability.ToolSource(source)
	e.CreatedAt = createdAt
	return e, nil
}

// scanEntries は *sql.Rows から ToolEntry スライスを読み取る
func scanEntries(rows *sql.Rows) ([]capability.ToolEntry, error) {
	var entries []capability.ToolEntry
	for rows.Next() {
		var e capability.ToolEntry
		var platformsJSON, source string
		var createdAt time.Time

		if err := rows.Scan(
			&e.Name, &e.Description, &e.SchemaJSON,
			&platformsJSON, &source, &e.Trusted, &createdAt, &e.CreatedBy,
		); err != nil {
			return nil, err
		}

		if err := json.Unmarshal([]byte(platformsJSON), &e.Platforms); err != nil {
			e.Platforms = []string{}
		}
		e.Source = capability.ToolSource(source)
		e.CreatedAt = createdAt
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// コンパイル時インターフェース適合チェック
var _ capability.ToolRegistry = (*DuckDBToolRegistryStore)(nil)
