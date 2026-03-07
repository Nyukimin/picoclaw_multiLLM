package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBStore はDuckDBを使った会話記憶ストア（中期記憶warm、7日保持）
type DuckDBStore struct {
	db *sql.DB
}

// NewDuckDBStore は新しいDuckDBStoreを生成
func NewDuckDBStore(dbPath string) (*DuckDBStore, error) {
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open duckdb: %w", err)
	}

	store := &DuckDBStore{db: db}

	// テーブル初期化
	if err := store.initTables(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	return store, nil
}

// Close はDuckDB接続を閉じる
func (d *DuckDBStore) Close() error {
	return d.db.Close()
}

// initTables はテーブルを初期化
func (d *DuckDBStore) initTables(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS session_thread (
		thread_id BIGINT PRIMARY KEY,
		session_id VARCHAR NOT NULL,
		ts_start TIMESTAMP NOT NULL,
		ts_end TIMESTAMP,
		domain VARCHAR,
		summary TEXT,
		keywords VARCHAR[],
		embedding FLOAT[],
		is_novel BOOLEAN,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	-- 単一カラムインデックス（互換性維持）
	CREATE INDEX IF NOT EXISTS idx_session_thread_session_id ON session_thread(session_id);
	CREATE INDEX IF NOT EXISTS idx_session_thread_domain ON session_thread(domain);
	CREATE INDEX IF NOT EXISTS idx_session_thread_ts_start ON session_thread(ts_start);
	
	-- 複合インデックス（パフォーマンス最適化）
	CREATE INDEX IF NOT EXISTS idx_session_thread_session_ts ON session_thread(session_id, ts_start DESC);
	CREATE INDEX IF NOT EXISTS idx_session_thread_domain_ts ON session_thread(domain, ts_start DESC);
	`

	if _, err := d.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	return nil
}

// SaveThreadSummary はThread要約をDuckDBに保存
func (d *DuckDBStore) SaveThreadSummary(ctx context.Context, summary *conversation.ThreadSummary) error {
	// keywords と embedding を JSON 化（DuckDB の配列型として保存）
	keywordsJSON, err := json.Marshal(summary.Keywords)
	if err != nil {
		return fmt.Errorf("failed to marshal keywords: %w", err)
	}

	embeddingJSON, err := json.Marshal(summary.Embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}

	query := `
	INSERT INTO session_thread (thread_id, session_id, ts_start, ts_end, domain, summary, keywords, embedding, is_novel)
	VALUES (?, ?, ?, ?, ?, ?, ?::VARCHAR[], ?::FLOAT[], ?)
	ON CONFLICT (thread_id) DO UPDATE SET
		summary = excluded.summary,
		keywords = excluded.keywords,
		embedding = excluded.embedding,
		is_novel = excluded.is_novel
	`

	_, err = d.db.ExecContext(ctx, query,
		summary.ThreadID,
		"", // session_id は Thread から取得する必要がある（Phase 2.5で修正）
		summary.StartTime,
		summary.EndTime,
		summary.Domain,
		summary.Summary,
		string(keywordsJSON),
		string(embeddingJSON),
		summary.IsNovel,
	)
	if err != nil {
		return fmt.Errorf("failed to save thread summary to duckdb: %w", err)
	}

	return nil
}

// GetSessionHistory はセッションの履歴を取得（最新limit件）
func (d *DuckDBStore) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]*conversation.ThreadSummary, error) {
	query := `
	SELECT thread_id, ts_start, ts_end, domain, summary, keywords, embedding, is_novel
	FROM session_thread
	WHERE session_id = ?
	ORDER BY ts_start DESC
	LIMIT ?
	`

	rows, err := d.db.QueryContext(ctx, query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query session history: %w", err)
	}
	defer rows.Close()

	summaries := make([]*conversation.ThreadSummary, 0, limit)
	for rows.Next() {
		var summary conversation.ThreadSummary
		var keywordsJSON, embeddingJSON string

		if err := rows.Scan(
			&summary.ThreadID,
			&summary.StartTime,
			&summary.EndTime,
			&summary.Domain,
			&summary.Summary,
			&keywordsJSON,
			&embeddingJSON,
			&summary.IsNovel,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// JSON → []string, []float32
		if err := json.Unmarshal([]byte(keywordsJSON), &summary.Keywords); err != nil {
			return nil, fmt.Errorf("failed to unmarshal keywords: %w", err)
		}
		if err := json.Unmarshal([]byte(embeddingJSON), &summary.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
		}

		summaries = append(summaries, &summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return summaries, nil
}

// SearchByDomain はドメインで Thread要約を検索
func (d *DuckDBStore) SearchByDomain(ctx context.Context, domain string, limit int) ([]*conversation.ThreadSummary, error) {
	query := `
	SELECT thread_id, ts_start, ts_end, domain, summary, keywords, embedding, is_novel
	FROM session_thread
	WHERE domain = ?
	ORDER BY ts_start DESC
	LIMIT ?
	`

	rows, err := d.db.QueryContext(ctx, query, domain, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query by domain: %w", err)
	}
	defer rows.Close()

	summaries := make([]*conversation.ThreadSummary, 0, limit)
	for rows.Next() {
		var summary conversation.ThreadSummary
		var keywordsJSON, embeddingJSON string

		if err := rows.Scan(
			&summary.ThreadID,
			&summary.StartTime,
			&summary.EndTime,
			&summary.Domain,
			&summary.Summary,
			&keywordsJSON,
			&embeddingJSON,
			&summary.IsNovel,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// JSON → []string, []float32
		if err := json.Unmarshal([]byte(keywordsJSON), &summary.Keywords); err != nil {
			return nil, fmt.Errorf("failed to unmarshal keywords: %w", err)
		}
		if err := json.Unmarshal([]byte(embeddingJSON), &summary.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal embedding: %w", err)
		}

		summaries = append(summaries, &summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return summaries, nil
}

// CleanupOldRecords は7日以上経過したレコードを削除
func (d *DuckDBStore) CleanupOldRecords(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-7 * 24 * time.Hour)

	query := `DELETE FROM session_thread WHERE ts_start < ?`

	result, err := d.db.ExecContext(ctx, query, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old records: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected, nil
}
