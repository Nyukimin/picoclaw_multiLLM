package conversation

import (
	"context"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func (s *L1SQLiteStore) initTables(ctx context.Context) error {
	schema := `
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS l1_memory_event (
	id TEXT PRIMARY KEY,
	namespace TEXT NOT NULL,
	session_id TEXT NOT NULL,
	thread_id INTEGER NOT NULL,
	speaker TEXT NOT NULL,
	message TEXT NOT NULL,
	meta_json TEXT NOT NULL DEFAULT '{}',
	memory_state TEXT NOT NULL,
	layer TEXT NOT NULL,
	source TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_memory_namespace_created ON l1_memory_event(namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_memory_session_created ON l1_memory_event(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_memory_state_created ON l1_memory_event(memory_state, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_memory_thread_created ON l1_memory_event(thread_id, created_at DESC);
CREATE TABLE IF NOT EXISTS l1_search_cache (
	query_hash TEXT PRIMARY KEY,
	normalized_query TEXT NOT NULL,
	provider TEXT NOT NULL,
	raw_query TEXT NOT NULL,
	results_json TEXT NOT NULL,
	source_urls_json TEXT NOT NULL DEFAULT '[]',
	retrieved_at TIMESTAMP NOT NULL,
	expires_at TIMESTAMP NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_search_cache_expires ON l1_search_cache(expires_at);
CREATE INDEX IF NOT EXISTS idx_l1_search_cache_retrieved ON l1_search_cache(retrieved_at DESC);
CREATE TABLE IF NOT EXISTS l1_event_log (
	id TEXT PRIMARY KEY,
	event_type TEXT NOT NULL,
	namespace TEXT NOT NULL,
	session_id TEXT NOT NULL DEFAULT '',
	thread_id INTEGER NOT NULL DEFAULT 0,
	payload_json TEXT NOT NULL DEFAULT '{}',
	source TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_event_log_namespace_created ON l1_event_log(namespace, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_event_log_type_created ON l1_event_log(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_event_log_session_created ON l1_event_log(session_id, created_at DESC);
CREATE TABLE IF NOT EXISTS l1_staging_item (
	id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	namespace TEXT NOT NULL,
	event_id TEXT NOT NULL,
	source_id TEXT NOT NULL,
	source_url TEXT NOT NULL DEFAULT '',
	fetched_at TIMESTAMP NOT NULL,
	published_at TIMESTAMP,
	raw_text TEXT NOT NULL,
	raw_hash TEXT NOT NULL,
	summary_draft TEXT NOT NULL DEFAULT '',
	keywords_json TEXT NOT NULL DEFAULT '[]',
	license_note TEXT NOT NULL DEFAULT '',
	validation_status TEXT NOT NULL,
	meta_json TEXT NOT NULL DEFAULT '{}',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_l1_staging_namespace_event ON l1_staging_item(namespace, event_id);
CREATE INDEX IF NOT EXISTS idx_l1_staging_status_created ON l1_staging_item(validation_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_staging_raw_hash ON l1_staging_item(raw_hash);
CREATE TABLE IF NOT EXISTS l1_source_registry (
	source_id TEXT PRIMARY KEY,
	url TEXT NOT NULL,
	kind TEXT NOT NULL,
	trust_score REAL NOT NULL,
	fetch_interval_sec INTEGER NOT NULL,
	license_note TEXT NOT NULL,
	enabled INTEGER NOT NULL,
	meta_json TEXT NOT NULL DEFAULT '{}',
	last_fetched_at TIMESTAMP,
	last_status TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_source_registry_enabled_kind ON l1_source_registry(enabled, kind);
CREATE TABLE IF NOT EXISTS l1_news_item (
	id TEXT PRIMARY KEY,
	staging_id TEXT NOT NULL UNIQUE,
	category TEXT NOT NULL,
	source_id TEXT NOT NULL,
	source_url TEXT NOT NULL DEFAULT '',
	published_at TIMESTAMP,
	fetched_at TIMESTAMP NOT NULL,
	raw_text TEXT NOT NULL,
	raw_hash TEXT NOT NULL,
	summary_draft TEXT NOT NULL DEFAULT '',
	keywords_json TEXT NOT NULL DEFAULT '[]',
	license_note TEXT NOT NULL DEFAULT '',
	meta_json TEXT NOT NULL DEFAULT '{}',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_news_category_published ON l1_news_item(category, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_news_source_published ON l1_news_item(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS idx_l1_news_raw_hash ON l1_news_item(raw_hash);
CREATE TABLE IF NOT EXISTS l1_daily_digest (
	id TEXT PRIMARY KEY,
	digest_date TEXT NOT NULL,
	category TEXT NOT NULL,
	digest_slot TEXT NOT NULL DEFAULT 'day',
	news_ids_json TEXT NOT NULL DEFAULT '[]',
	digest_text TEXT NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE TABLE IF NOT EXISTS l1_knowledge_item (
	id TEXT PRIMARY KEY,
	staging_id TEXT NOT NULL UNIQUE,
	domain TEXT NOT NULL,
	title TEXT NOT NULL,
	source_id TEXT NOT NULL,
	source_url TEXT NOT NULL DEFAULT '',
	raw_text TEXT NOT NULL,
	raw_hash TEXT NOT NULL,
	summary_draft TEXT NOT NULL DEFAULT '',
	keywords_json TEXT NOT NULL DEFAULT '[]',
	license_note TEXT NOT NULL DEFAULT '',
	meta_json TEXT NOT NULL DEFAULT '{}',
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_l1_knowledge_domain_title ON l1_knowledge_item(domain, title);
CREATE INDEX IF NOT EXISTS idx_l1_knowledge_raw_hash ON l1_knowledge_item(raw_hash);
CREATE TABLE IF NOT EXISTS l1_knowledge_item_fts (
	id TEXT PRIMARY KEY,
	domain TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	raw_text TEXT NOT NULL DEFAULT '',
	summary_draft TEXT NOT NULL DEFAULT '',
	keywords_text TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_l1_knowledge_fts_domain ON l1_knowledge_item_fts(domain);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("failed to initialize l1 sqlite schema: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE l1_daily_digest ADD COLUMN digest_slot TEXT NOT NULL DEFAULT 'day'`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("failed to migrate l1 daily digest slot: %w", err)
	}
	for _, stmt := range []string{
		`ALTER TABLE l1_source_registry ADD COLUMN last_fetched_at TIMESTAMP`,
		`ALTER TABLE l1_source_registry ADD COLUMN last_status TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE l1_source_registry ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("failed to migrate l1 source registry fetch status: %w", err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `
DROP INDEX IF EXISTS idx_l1_daily_digest_date_category;
CREATE UNIQUE INDEX IF NOT EXISTS idx_l1_daily_digest_date_category_slot ON l1_daily_digest(digest_date, category, digest_slot);
CREATE INDEX IF NOT EXISTS idx_l1_daily_digest_category_created ON l1_daily_digest(category, digest_slot, created_at DESC);
`); err != nil {
		return fmt.Errorf("failed to initialize l1 daily digest slot indexes: %w", err)
	}
	return nil
}
