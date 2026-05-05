package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	_ "github.com/mattn/go-sqlite3"
)

const (
	MemoryStateObserved  = "observed"
	MemoryStateCandidate = "candidate"
	MemoryStateConfirmed = "confirmed"
	MemoryLayerL1        = "L1"
)

type L1MemoryEvent struct {
	ID          string
	Namespace   string
	SessionID   string
	ThreadID    int64
	Speaker     domconv.Speaker
	Message     string
	Meta        map[string]interface{}
	MemoryState string
	Layer       string
	Source      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type L1SearchCacheEntry struct {
	QueryHash       string
	NormalizedQuery string
	Provider        string
	RawQuery        string
	ResultsJSON     string
	SourceURLs      []string
	RetrievedAt     time.Time
	ExpiresAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type L1SQLiteStore struct {
	db *sql.DB
}

func NewL1SQLiteStore(dbPath string) (*L1SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open l1 sqlite: %w", err)
	}
	store := &L1SQLiteStore{db: db}
	if err := store.initTables(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *L1SQLiteStore) Close() error {
	return s.db.Close()
}

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
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("failed to initialize l1 sqlite schema: %w", err)
	}
	return nil
}

func (s *L1SQLiteStore) SaveMessage(ctx context.Context, sessionID string, threadID int64, namespace string, msg domconv.Message, memoryState string) error {
	if namespace == "" {
		namespace = fmt.Sprintf("conv:%d", threadID)
	}
	if memoryState == "" {
		memoryState = MemoryStateObserved
	}
	if err := validateMemoryState(memoryState); err != nil {
		return err
	}
	layer := MemoryLayerL1
	now := time.Now().UTC()
	createdAt := msg.Timestamp
	if createdAt.IsZero() {
		createdAt = now
	}
	createdAt = createdAt.UTC()
	meta := msg.Meta
	if meta == nil {
		meta = map[string]interface{}{}
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal l1 memory meta: %w", err)
	}
	id := fmt.Sprintf("%s:%d:%d:%s", sessionID, threadID, createdAt.UnixNano(), msg.Speaker)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO l1_memory_event (
	id, namespace, session_id, thread_id, speaker, message, meta_json,
	memory_state, layer, source, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	message = excluded.message,
	meta_json = excluded.meta_json,
	memory_state = excluded.memory_state,
	updated_at = excluded.updated_at
`, id, namespace, sessionID, threadID, string(msg.Speaker), msg.Msg, string(metaJSON),
		memoryState, layer, "conversation", createdAt, now)
	if err != nil {
		return fmt.Errorf("failed to save l1 memory event: %w", err)
	}
	return nil
}

func (s *L1SQLiteStore) SaveSearchCache(ctx context.Context, provider string, rawQuery string, resultsJSON string, sourceURLs []string, ttl time.Duration) (*L1SearchCacheEntry, error) {
	normalizedQuery := normalizeSearchQuery(rawQuery)
	if normalizedQuery == "" {
		return nil, errors.New("search cache query is required")
	}
	if provider == "" {
		provider = "default"
	}
	if resultsJSON == "" {
		resultsJSON = "[]"
	}
	if !json.Valid([]byte(resultsJSON)) {
		return nil, errors.New("search cache results_json must be valid JSON")
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	now := time.Now().UTC()
	entry := &L1SearchCacheEntry{
		QueryHash:       searchQueryHash(provider, normalizedQuery),
		NormalizedQuery: normalizedQuery,
		Provider:        provider,
		RawQuery:        rawQuery,
		ResultsJSON:     resultsJSON,
		SourceURLs:      sourceURLs,
		RetrievedAt:     now,
		ExpiresAt:       now.Add(ttl),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	sourceURLsJSON, err := json.Marshal(sourceURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal search cache source urls: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO l1_search_cache (
	query_hash, normalized_query, provider, raw_query, results_json, source_urls_json,
	retrieved_at, expires_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(query_hash) DO UPDATE SET
	raw_query = excluded.raw_query,
	results_json = excluded.results_json,
	source_urls_json = excluded.source_urls_json,
	retrieved_at = excluded.retrieved_at,
	expires_at = excluded.expires_at,
	updated_at = excluded.updated_at
`, entry.QueryHash, entry.NormalizedQuery, entry.Provider, entry.RawQuery, entry.ResultsJSON, string(sourceURLsJSON),
		entry.RetrievedAt, entry.ExpiresAt, entry.CreatedAt, entry.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save l1 search cache: %w", err)
	}
	return entry, nil
}

func (s *L1SQLiteStore) GetFreshSearchCache(ctx context.Context, provider string, rawQuery string, now time.Time) (*L1SearchCacheEntry, error) {
	normalizedQuery := normalizeSearchQuery(rawQuery)
	if normalizedQuery == "" {
		return nil, errors.New("search cache query is required")
	}
	if provider == "" {
		provider = "default"
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	row := s.db.QueryRowContext(ctx, `
SELECT query_hash, normalized_query, provider, raw_query, results_json, source_urls_json,
       retrieved_at, expires_at, created_at, updated_at
FROM l1_search_cache
WHERE query_hash = ? AND expires_at > ?
`, searchQueryHash(provider, normalizedQuery), now.UTC())
	entry, err := scanL1SearchCacheEntry(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

func (s *L1SQLiteStore) UpdateMemoryState(ctx context.Context, id string, memoryState string) error {
	if id == "" {
		return errors.New("l1 memory event id is required")
	}
	if err := validateMemoryState(memoryState); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
UPDATE l1_memory_event
SET memory_state = ?, updated_at = ?
WHERE id = ?
`, memoryState, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("failed to update l1 memory state: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect l1 memory state update: %w", err)
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *L1SQLiteStore) RecentByNamespace(ctx context.Context, namespace string, limit int) ([]L1MemoryEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, namespace, session_id, thread_id, speaker, message, meta_json,
       memory_state, layer, source, created_at, updated_at
FROM l1_memory_event
WHERE namespace = ?
ORDER BY created_at DESC
LIMIT ?
`, namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 memory events: %w", err)
	}
	defer rows.Close()
	return scanL1Events(rows)
}

func (s *L1SQLiteStore) RecentByState(ctx context.Context, memoryState string, limit int) ([]L1MemoryEvent, error) {
	if err := validateMemoryState(memoryState); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, namespace, session_id, thread_id, speaker, message, meta_json,
       memory_state, layer, source, created_at, updated_at
FROM l1_memory_event
WHERE memory_state = ?
ORDER BY created_at DESC
LIMIT ?
`, memoryState, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 memory events by state: %w", err)
	}
	defer rows.Close()
	return scanL1Events(rows)
}

func (s *L1SQLiteStore) RecentBySession(ctx context.Context, sessionID string, limit int) ([]L1MemoryEvent, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, namespace, session_id, thread_id, speaker, message, meta_json,
       memory_state, layer, source, created_at, updated_at
FROM l1_memory_event
WHERE session_id = ?
ORDER BY created_at DESC
LIMIT ?
`, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 memory events by session: %w", err)
	}
	defer rows.Close()
	return scanL1Events(rows)
}

func validateMemoryState(memoryState string) error {
	switch memoryState {
	case MemoryStateObserved, MemoryStateCandidate, MemoryStateConfirmed:
		return nil
	default:
		return fmt.Errorf("invalid l1 memory state: %s", memoryState)
	}
}

type l1SearchCacheRow interface {
	Scan(dest ...interface{}) error
}

func scanL1SearchCacheEntry(row l1SearchCacheRow) (*L1SearchCacheEntry, error) {
	var entry L1SearchCacheEntry
	var sourceURLsJSON string
	if err := row.Scan(
		&entry.QueryHash,
		&entry.NormalizedQuery,
		&entry.Provider,
		&entry.RawQuery,
		&entry.ResultsJSON,
		&sourceURLsJSON,
		&entry.RetrievedAt,
		&entry.ExpiresAt,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan l1 search cache: %w", err)
	}
	if sourceURLsJSON == "" {
		sourceURLsJSON = "[]"
	}
	if err := json.Unmarshal([]byte(sourceURLsJSON), &entry.SourceURLs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal search cache source urls: %w", err)
	}
	return &entry, nil
}

func normalizeSearchQuery(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(query)), " ")
}

func searchQueryHash(provider string, normalizedQuery string) string {
	sum := sha256.Sum256([]byte(provider + "\x00" + normalizedQuery))
	return hex.EncodeToString(sum[:])
}

func scanL1Events(rows *sql.Rows) ([]L1MemoryEvent, error) {
	var events []L1MemoryEvent
	for rows.Next() {
		var ev L1MemoryEvent
		var metaJSON string
		var speaker string
		if err := rows.Scan(
			&ev.ID,
			&ev.Namespace,
			&ev.SessionID,
			&ev.ThreadID,
			&speaker,
			&ev.Message,
			&metaJSON,
			&ev.MemoryState,
			&ev.Layer,
			&ev.Source,
			&ev.CreatedAt,
			&ev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan l1 memory event: %w", err)
		}
		ev.Speaker = domconv.Speaker(speaker)
		if metaJSON == "" {
			metaJSON = "{}"
		}
		if err := json.Unmarshal([]byte(metaJSON), &ev.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal l1 memory meta: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("l1 memory rows error: %w", err)
	}
	return events, nil
}
