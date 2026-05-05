package conversation

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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

const (
	L1StagingKindExternalFetch   = "external_fetch"
	L1StagingKindMemoryCandidate = "memory_candidate"
	L1StagingKindSearchResult    = "search_result"

	L1StagingStatusPending   = "pending"
	L1StagingStatusValidated = "validated"
	L1StagingStatusRejected  = "rejected"
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

type L1StagingItem struct {
	ID               string
	Kind             string
	Namespace        string
	EventID          string
	SourceID         string
	SourceURL        string
	FetchedAt        time.Time
	PublishedAt      time.Time
	RawText          string
	RawHash          string
	SummaryDraft     string
	Keywords         []string
	LicenseNote      string
	ValidationStatus string
	Meta             map[string]interface{}
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type L1StagingValidationPolicy struct {
	SourceTrustScores map[string]float64
	MinimumTrustScore float64
	Now               time.Time
}

type L1StagingValidationIssue struct {
	Code    string
	Message string
}

type L1StagingValidationResult struct {
	ItemID string
	Passed bool
	Status string
	Issues []L1StagingValidationIssue
}

func (r L1StagingValidationResult) HasIssue(code string) bool {
	for _, issue := range r.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
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

func (s *L1SQLiteStore) SaveStagingItem(ctx context.Context, item L1StagingItem) (*L1StagingItem, error) {
	item.Kind = strings.TrimSpace(item.Kind)
	item.Namespace = strings.TrimSpace(item.Namespace)
	item.EventID = strings.TrimSpace(item.EventID)
	item.SourceID = strings.TrimSpace(item.SourceID)
	item.SourceURL = strings.TrimSpace(item.SourceURL)
	item.ValidationStatus = strings.TrimSpace(item.ValidationStatus)
	if item.ValidationStatus == "" {
		item.ValidationStatus = L1StagingStatusPending
	}
	if err := validateL1StagingKind(item.Kind); err != nil {
		return nil, err
	}
	if err := ValidateL1Namespace(item.Namespace); err != nil {
		return nil, err
	}
	if item.EventID == "" {
		return nil, errors.New("l1 staging event_id is required")
	}
	if item.SourceID == "" {
		return nil, errors.New("l1 staging source_id is required")
	}
	if err := validateOptionalSourceURL(item.SourceURL); err != nil {
		return nil, err
	}
	if strings.TrimSpace(item.RawText) == "" {
		return nil, errors.New("l1 staging raw_text is required")
	}
	if err := validateL1StagingStatus(item.ValidationStatus); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if item.FetchedAt.IsZero() {
		item.FetchedAt = now
	}
	item.FetchedAt = item.FetchedAt.UTC()
	if !item.PublishedAt.IsZero() {
		item.PublishedAt = item.PublishedAt.UTC()
	}
	item.RawHash = rawTextHash(item.RawText)
	if item.ID == "" {
		item.ID = fmt.Sprintf("%s:%s:%s", item.Namespace, item.EventID, item.RawHash[:12])
	}
	if item.Meta == nil {
		item.Meta = map[string]interface{}{}
	}
	item.CreatedAt = now
	item.UpdatedAt = now
	keywordsJSON, err := json.Marshal(item.Keywords)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 staging keywords: %w", err)
	}
	metaJSON, err := json.Marshal(item.Meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 staging meta: %w", err)
	}
	var publishedAt interface{}
	if !item.PublishedAt.IsZero() {
		publishedAt = item.PublishedAt
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO l1_staging_item (
	id, kind, namespace, event_id, source_id, source_url, fetched_at, published_at,
	raw_text, raw_hash, summary_draft, keywords_json, license_note,
	validation_status, meta_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(namespace, event_id) DO UPDATE SET
	kind = excluded.kind,
	source_id = excluded.source_id,
	source_url = excluded.source_url,
	fetched_at = excluded.fetched_at,
	published_at = excluded.published_at,
	raw_text = excluded.raw_text,
	raw_hash = excluded.raw_hash,
	summary_draft = excluded.summary_draft,
	keywords_json = excluded.keywords_json,
	license_note = excluded.license_note,
	validation_status = excluded.validation_status,
	meta_json = excluded.meta_json,
	updated_at = excluded.updated_at
`, item.ID, item.Kind, item.Namespace, item.EventID, item.SourceID, item.SourceURL, item.FetchedAt, publishedAt,
		item.RawText, item.RawHash, item.SummaryDraft, string(keywordsJSON), item.LicenseNote,
		item.ValidationStatus, string(metaJSON), item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to save l1 staging item: %w", err)
	}
	if _, err := s.AppendEvent(ctx, "staging.item_saved", item.Namespace, "", 0, map[string]interface{}{
		"staging_id":        item.ID,
		"kind":              item.Kind,
		"event_id":          item.EventID,
		"source_id":         item.SourceID,
		"source_url":        item.SourceURL,
		"raw_hash":          item.RawHash,
		"validation_status": item.ValidationStatus,
	}, "staging"); err != nil {
		return nil, fmt.Errorf("failed to append l1 staging event log: %w", err)
	}
	return &item, nil
}

func (s *L1SQLiteStore) RecentStagingItems(ctx context.Context, validationStatus string, limit int) ([]L1StagingItem, error) {
	if err := validateL1StagingStatus(validationStatus); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, namespace, event_id, source_id, source_url, fetched_at, published_at,
       raw_text, raw_hash, summary_draft, keywords_json, license_note,
       validation_status, meta_json, created_at, updated_at
FROM l1_staging_item
WHERE validation_status = ?
ORDER BY created_at DESC
LIMIT ?
`, validationStatus, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 staging items: %w", err)
	}
	defer rows.Close()
	return scanL1StagingItems(rows)
}

func (s *L1SQLiteStore) ValidateStagingItem(ctx context.Context, id string, policy L1StagingValidationPolicy) (*L1StagingValidationResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("l1 staging item id is required")
	}
	item, err := s.stagingItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result, err := s.validateStagingItemContent(ctx, *item, policy)
	if err != nil {
		return nil, err
	}
	now := policy.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	issuesJSON, err := json.Marshal(result.Issues)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 staging validation issues: %w", err)
	}
	meta := map[string]interface{}{}
	for k, v := range item.Meta {
		meta[k] = v
	}
	meta["validation_issues"] = result.Issues
	meta["validated_at"] = now.Format(time.RFC3339)
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 staging validation meta: %w", err)
	}
	update, err := s.db.ExecContext(ctx, `
UPDATE l1_staging_item
SET validation_status = ?, meta_json = ?, updated_at = ?
WHERE id = ?
`, result.Status, string(metaJSON), now, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update l1 staging validation status: %w", err)
	}
	affected, err := update.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect l1 staging validation update: %w", err)
	}
	if affected == 0 {
		return nil, sql.ErrNoRows
	}
	if _, err := s.AppendEvent(ctx, "staging.item_validated", item.Namespace, "", 0, map[string]interface{}{
		"staging_id":        item.ID,
		"passed":            result.Passed,
		"validation_status": result.Status,
		"issues":            string(issuesJSON),
	}, "validator"); err != nil {
		return nil, fmt.Errorf("failed to append l1 staging validation event log: %w", err)
	}
	return &result, nil
}

func (s *L1SQLiteStore) PromoteValidatedStagingItemToMemory(ctx context.Context, id string, targetNamespace string, promotedBy string) (*L1MemoryEvent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("l1 staging item id is required")
	}
	if err := ValidateL1Namespace(targetNamespace); err != nil {
		return nil, err
	}
	item, err := s.stagingItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.ValidationStatus != L1StagingStatusValidated {
		return nil, fmt.Errorf("l1 staging item must be validated before promotion: %s", item.ValidationStatus)
	}
	message := strings.TrimSpace(item.SummaryDraft)
	if message == "" {
		message = strings.TrimSpace(item.RawText)
	}
	if message == "" {
		return nil, errors.New("l1 staging item has no promotable text")
	}
	now := time.Now().UTC()
	meta := map[string]interface{}{}
	for k, v := range item.Meta {
		meta[k] = v
	}
	meta["staging_id"] = item.ID
	meta["staging_kind"] = item.Kind
	meta["staging_namespace"] = item.Namespace
	meta["event_id"] = item.EventID
	meta["source_id"] = item.SourceID
	meta["source_url"] = item.SourceURL
	meta["raw_hash"] = item.RawHash
	meta["license_note"] = item.LicenseNote
	meta["promoted_by"] = promotedBy
	meta["validation_status"] = item.ValidationStatus
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal l1 staging promoted memory meta: %w", err)
	}
	sessionID := metaString(item.Meta, "session_id")
	threadID := metaInt64(item.Meta, "thread_id")
	promoted := &L1MemoryEvent{
		ID:          fmt.Sprintf("%s:%s:%d", targetNamespace, item.ID, now.UnixNano()),
		Namespace:   targetNamespace,
		SessionID:   sessionID,
		ThreadID:    threadID,
		Speaker:     domconv.SpeakerMemory,
		Message:     message,
		Meta:        meta,
		MemoryState: MemoryStateConfirmed,
		Layer:       MemoryLayerL1,
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
		return nil, fmt.Errorf("failed to promote l1 staging item to memory: %w", err)
	}
	if _, err := s.AppendEvent(ctx, "memory.promoted_from_staging", targetNamespace, sessionID, threadID, map[string]interface{}{
		"staging_id":         item.ID,
		"promoted_memory_id": promoted.ID,
		"promoted_by":        promotedBy,
		"source_namespace":   item.Namespace,
		"memory_state":       promoted.MemoryState,
	}, "promoter"); err != nil {
		return nil, fmt.Errorf("failed to append l1 staging promoted event log: %w", err)
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

func (s *L1SQLiteStore) stagingItemByID(ctx context.Context, id string) (*L1StagingItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, namespace, event_id, source_id, source_url, fetched_at, published_at,
       raw_text, raw_hash, summary_draft, keywords_json, license_note,
       validation_status, meta_json, created_at, updated_at
FROM l1_staging_item
WHERE id = ?
LIMIT 1
`, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query l1 staging item by id: %w", err)
	}
	defer rows.Close()
	items, err := scanL1StagingItems(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	return &items[0], nil
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

func validateL1StagingKind(kind string) error {
	switch kind {
	case L1StagingKindExternalFetch, L1StagingKindMemoryCandidate, L1StagingKindSearchResult:
		return nil
	default:
		return fmt.Errorf("invalid l1 staging kind: %s", kind)
	}
}

func validateL1StagingStatus(status string) error {
	switch status {
	case L1StagingStatusPending, L1StagingStatusValidated, L1StagingStatusRejected:
		return nil
	default:
		return fmt.Errorf("invalid l1 staging validation status: %s", status)
	}
}

func (s *L1SQLiteStore) validateStagingItemContent(ctx context.Context, item L1StagingItem, policy L1StagingValidationPolicy) (L1StagingValidationResult, error) {
	now := policy.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	result := L1StagingValidationResult{
		ItemID: item.ID,
		Status: L1StagingStatusValidated,
	}
	addIssue := func(code string, message string) {
		result.Issues = append(result.Issues, L1StagingValidationIssue{Code: code, Message: message})
	}
	if err := validateL1StagingKind(item.Kind); err != nil {
		addIssue("invalid_kind", err.Error())
	}
	if err := ValidateL1Namespace(item.Namespace); err != nil {
		addIssue("invalid_namespace", err.Error())
	}
	if strings.TrimSpace(item.EventID) == "" {
		addIssue("missing_event_id", "event_id is required")
	}
	if strings.TrimSpace(item.SourceID) == "" {
		addIssue("missing_source_id", "source_id is required")
	}
	if err := validateOptionalSourceURL(item.SourceURL); err != nil {
		addIssue("invalid_source_url", err.Error())
	}
	if strings.TrimSpace(item.RawText) == "" {
		addIssue("missing_raw_text", "raw_text is required")
	}
	if rawTextHash(item.RawText) != item.RawHash {
		addIssue("raw_hash_mismatch", "raw_hash does not match raw_text")
	}
	duplicates, err := s.countStagingRawHashDuplicates(ctx, item.ID, item.RawHash)
	if err != nil {
		return result, err
	}
	if duplicates > 0 {
		addIssue("duplicate_raw_hash", "raw_hash already exists in staging")
	}
	if !item.PublishedAt.IsZero() && (item.PublishedAt.After(now) || item.PublishedAt.After(item.FetchedAt)) {
		addIssue("future_published_at", "published_at must not be in the future or after fetched_at")
	}
	if strings.TrimSpace(item.LicenseNote) == "" {
		addIssue("missing_license_note", "license_note is required before promotion")
	}
	if policy.MinimumTrustScore > 0 {
		score, ok := policy.SourceTrustScores[item.SourceID]
		if !ok {
			addIssue("missing_source_trust", "source_id has no trust score")
		} else if score < policy.MinimumTrustScore {
			addIssue("low_source_trust", "source trust score is below minimum")
		}
	}
	if item.Kind == L1StagingKindMemoryCandidate {
		memoryType, ok := item.Meta["type"].(string)
		if !ok || !isAllowedMemoryType(memoryType) {
			addIssue("missing_memory_type", "memory candidate requires an allowed type")
		}
	}
	if containsSensitiveRawText(item.RawText) {
		addIssue("sensitive_raw_text", "raw_text appears to contain sensitive secret material")
	}
	if len(result.Issues) > 0 {
		result.Status = L1StagingStatusRejected
		result.Passed = false
		return result, nil
	}
	result.Passed = true
	return result, nil
}

func (s *L1SQLiteStore) countStagingRawHashDuplicates(ctx context.Context, id string, rawHash string) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM l1_staging_item
WHERE raw_hash = ? AND id <> ?
`, rawHash, id).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count l1 staging raw hash duplicates: %w", err)
	}
	return count, nil
}

func isAllowedMemoryType(memoryType string) bool {
	switch strings.TrimSpace(memoryType) {
	case "profile", "preference", "project", "constraint", "relationship", "episode", "skill", "sensitive":
		return true
	default:
		return false
	}
}

func containsSensitiveRawText(rawText string) bool {
	normalized := strings.ToLower(rawText)
	sensitiveMarkers := []string{"api_key", "apikey", "password", "secret", "token:"}
	for _, marker := range sensitiveMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func metaString(meta map[string]interface{}, key string) string {
	value, ok := meta[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func metaInt64(meta map[string]interface{}, key string) int64 {
	value, ok := meta[key]
	if !ok {
		return 0
	}
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	case json.Number:
		i, _ := v.Int64()
		return i
	default:
		return 0
	}
}

func validateOptionalSourceURL(sourceURL string) error {
	if sourceURL == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid l1 staging source_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid l1 staging source_url scheme: %s", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("invalid l1 staging source_url host")
	}
	return nil
}

func rawTextHash(rawText string) string {
	sum := sha256.Sum256([]byte(rawText))
	return hex.EncodeToString(sum[:])
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

func scanL1StagingItems(rows *sql.Rows) ([]L1StagingItem, error) {
	var items []L1StagingItem
	for rows.Next() {
		var item L1StagingItem
		var keywordsJSON string
		var metaJSON string
		var publishedAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.Kind,
			&item.Namespace,
			&item.EventID,
			&item.SourceID,
			&item.SourceURL,
			&item.FetchedAt,
			&publishedAt,
			&item.RawText,
			&item.RawHash,
			&item.SummaryDraft,
			&keywordsJSON,
			&item.LicenseNote,
			&item.ValidationStatus,
			&metaJSON,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan l1 staging item: %w", err)
		}
		if publishedAt.Valid {
			item.PublishedAt = publishedAt.Time
		}
		if keywordsJSON == "" {
			keywordsJSON = "[]"
		}
		if err := json.Unmarshal([]byte(keywordsJSON), &item.Keywords); err != nil {
			return nil, fmt.Errorf("failed to unmarshal l1 staging keywords: %w", err)
		}
		if metaJSON == "" {
			metaJSON = "{}"
		}
		if err := json.Unmarshal([]byte(metaJSON), &item.Meta); err != nil {
			return nil, fmt.Errorf("failed to unmarshal l1 staging meta: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("l1 staging rows error: %w", err)
	}
	return items, nil
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
