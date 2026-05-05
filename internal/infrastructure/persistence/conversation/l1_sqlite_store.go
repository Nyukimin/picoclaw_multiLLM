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

type L1EventLogEntry struct {
	ID        string
	EventType string
	Namespace string
	SessionID string
	ThreadID  int64
	Payload   map[string]interface{}
	Source    string
	CreatedAt time.Time
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
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("failed to initialize l1 sqlite schema: %w", err)
	}
	return nil
}

func (s *L1SQLiteStore) SaveMessage(ctx context.Context, sessionID string, threadID int64, namespace string, msg domconv.Message, memoryState string) error {
	if namespace == "" {
		var err error
		namespace, err = BuildL1Namespace(NamespaceKindConversation, fmt.Sprintf("%d", threadID))
		if err != nil {
			return err
		}
	}
	if err := ValidateL1Namespace(namespace); err != nil {
		return err
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
	if _, err := s.AppendEvent(ctx, "memory.message_saved", namespace, sessionID, threadID, map[string]interface{}{
		"memory_id":    id,
		"speaker":      string(msg.Speaker),
		"memory_state": memoryState,
		"layer":        layer,
	}, "conversation"); err != nil {
		return fmt.Errorf("failed to append l1 message event log: %w", err)
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
	searchNamespace, err := BuildL1Namespace(NamespaceKindKnowledge, provider)
	if err != nil {
		return nil, err
	}
	if _, err := s.AppendEvent(ctx, "search.cache_saved", searchNamespace, "", 0, map[string]interface{}{
		"query_hash":       entry.QueryHash,
		"normalized_query": entry.NormalizedQuery,
		"raw_query":        entry.RawQuery,
		"provider":         entry.Provider,
		"expires_at":       entry.ExpiresAt.Format(time.RFC3339),
		"source_urls":      entry.SourceURLs,
	}, "search_cache"); err != nil {
		return nil, fmt.Errorf("failed to append l1 search cache event log: %w", err)
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

func (s *L1SQLiteStore) AppendEvent(ctx context.Context, eventType string, namespace string, sessionID string, threadID int64, payload map[string]interface{}, source string) (*L1EventLogEntry, error) {
	eventType = strings.TrimSpace(eventType)
	namespace = strings.TrimSpace(namespace)
	if eventType == "" {
		return nil, errors.New("l1 event type is required")
	}
	if err := ValidateL1Namespace(namespace); err != nil {
		return nil, err
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 event payload: %w", err)
	}
	now := time.Now().UTC()
	entry := &L1EventLogEntry{
		ID:        fmt.Sprintf("%s:%s:%d", namespace, eventType, now.UnixNano()),
		EventType: eventType,
		Namespace: namespace,
		SessionID: sessionID,
		ThreadID:  threadID,
		Payload:   payload,
		Source:    source,
		CreatedAt: now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO l1_event_log (
	id, event_type, namespace, session_id, thread_id, payload_json, source, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, entry.ID, entry.EventType, entry.Namespace, entry.SessionID, entry.ThreadID, string(payloadJSON), entry.Source, entry.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to append l1 event log: %w", err)
	}
	return entry, nil
}

func (s *L1SQLiteStore) RecentEvents(ctx context.Context, namespace string, limit int) ([]L1EventLogEntry, error) {
	namespace = strings.TrimSpace(namespace)
	if err := ValidateL1Namespace(namespace); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, event_type, namespace, session_id, thread_id, payload_json, source, created_at
FROM l1_event_log
WHERE namespace = ?
ORDER BY created_at DESC
LIMIT ?
`, namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 event log: %w", err)
	}
	defer rows.Close()
	return scanL1EventLogEntries(rows)
}

func (s *L1SQLiteStore) UpdateMemoryState(ctx context.Context, id string, memoryState string) error {
	if id == "" {
		return errors.New("l1 memory event id is required")
	}
	if err := validateMemoryState(memoryState); err != nil {
		return err
	}
	var namespace string
	var sessionID string
	var threadID int64
	var previousState string
	if err := s.db.QueryRowContext(ctx, `
SELECT namespace, session_id, thread_id, memory_state
FROM l1_memory_event
WHERE id = ?
`, id).Scan(&namespace, &sessionID, &threadID, &previousState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sql.ErrNoRows
		}
		return fmt.Errorf("failed to load l1 memory event before state update: %w", err)
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
	if _, err := s.AppendEvent(ctx, "memory.state_updated", namespace, sessionID, threadID, map[string]interface{}{
		"memory_id":      id,
		"previous_state": previousState,
		"memory_state":   memoryState,
	}, "memory"); err != nil {
		return fmt.Errorf("failed to append l1 memory state event log: %w", err)
	}
	return nil
}

func (s *L1SQLiteStore) PromoteMemoryToNamespace(ctx context.Context, id string, targetNamespace string, promotedBy string) (*L1MemoryEvent, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("l1 memory event id is required")
	}
	if err := ValidateL1Namespace(targetNamespace); err != nil {
		return nil, err
	}
	source, err := s.memoryByID(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	meta := map[string]interface{}{}
	for k, v := range source.Meta {
		meta[k] = v
	}
	meta["promoted_from"] = source.ID
	meta["promoted_by"] = promotedBy
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal promoted l1 memory meta: %w", err)
	}
	promoted := &L1MemoryEvent{
		ID:          fmt.Sprintf("%s:%s:%d", targetNamespace, source.ID, now.UnixNano()),
		Namespace:   targetNamespace,
		SessionID:   source.SessionID,
		ThreadID:    source.ThreadID,
		Speaker:     source.Speaker,
		Message:     source.Message,
		Meta:        meta,
		MemoryState: MemoryStateConfirmed,
		Layer:       source.Layer,
		Source:      "promoter",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO l1_memory_event (
	id, namespace, session_id, thread_id, speaker, message, meta_json,
	memory_state, layer, source, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, promoted.ID, promoted.Namespace, promoted.SessionID, promoted.ThreadID, string(promoted.Speaker), promoted.Message, string(metaJSON),
		promoted.MemoryState, promoted.Layer, promoted.Source, promoted.CreatedAt, promoted.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to promote l1 memory: %w", err)
	}
	if _, err := s.AppendEvent(ctx, "memory.promoted", targetNamespace, source.SessionID, source.ThreadID, map[string]interface{}{
		"source_memory_id":   source.ID,
		"promoted_memory_id": promoted.ID,
		"promoted_by":        promotedBy,
		"memory_state":       promoted.MemoryState,
	}, "promoter"); err != nil {
		return nil, fmt.Errorf("failed to append l1 memory promoted event log: %w", err)
	}
	return promoted, nil
}

func (s *L1SQLiteStore) RecentByNamespace(ctx context.Context, namespace string, limit int) ([]L1MemoryEvent, error) {
	if err := ValidateL1Namespace(namespace); err != nil {
		return nil, err
	}
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

func (s *L1SQLiteStore) memoryByID(ctx context.Context, id string) (*L1MemoryEvent, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, namespace, session_id, thread_id, speaker, message, meta_json,
       memory_state, layer, source, created_at, updated_at
FROM l1_memory_event
WHERE id = ?
`, id)
	events, err := scanL1EventRows(row)
	if err != nil {
		return nil, err
	}
	return &events[0], nil
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

func scanL1EventLogEntries(rows *sql.Rows) ([]L1EventLogEntry, error) {
	var events []L1EventLogEntry
	for rows.Next() {
		var ev L1EventLogEntry
		var payloadJSON string
		if err := rows.Scan(
			&ev.ID,
			&ev.EventType,
			&ev.Namespace,
			&ev.SessionID,
			&ev.ThreadID,
			&payloadJSON,
			&ev.Source,
			&ev.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan l1 event log: %w", err)
		}
		if payloadJSON == "" {
			payloadJSON = "{}"
		}
		if err := json.Unmarshal([]byte(payloadJSON), &ev.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal l1 event payload: %w", err)
		}
		events = append(events, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("l1 event log rows error: %w", err)
	}
	return events, nil
}

func scanL1Events(rows *sql.Rows) ([]L1MemoryEvent, error) {
	var events []L1MemoryEvent
	for rows.Next() {
		ev, err := scanL1EventRows(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, ev...)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("l1 memory rows error: %w", err)
	}
	return events, nil
}

type l1MemoryRow interface {
	Scan(dest ...interface{}) error
}

func scanL1EventRows(row l1MemoryRow) ([]L1MemoryEvent, error) {
	var ev L1MemoryEvent
	var metaJSON string
	var speaker string
	if err := row.Scan(
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan l1 memory event: %w", err)
	}
	ev.Speaker = domconv.Speaker(speaker)
	if metaJSON == "" {
		metaJSON = "{}"
	}
	if err := json.Unmarshal([]byte(metaJSON), &ev.Meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal l1 memory meta: %w", err)
	}
	return []L1MemoryEvent{ev}, nil
}
