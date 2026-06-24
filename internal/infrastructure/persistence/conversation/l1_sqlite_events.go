package conversation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	_ "github.com/mattn/go-sqlite3"
)

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

func (s *L1SQLiteStore) SaveRecallTrace(ctx context.Context, trace domconv.RecallTrace) error {
	if strings.TrimSpace(trace.ResponseID) == "" {
		return errors.New("response_id is required")
	}
	if strings.TrimSpace(trace.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if trace.CreatedAt.IsZero() {
		trace.CreatedAt = time.Now().UTC()
	}
	payload := map[string]interface{}{
		"response_id": trace.ResponseID,
		"session_id":  trace.SessionID,
		"role":        trace.Role,
		"items":       trace.Items,
		"created_at":  trace.CreatedAt.UTC().Format(time.RFC3339),
	}
	_, err := s.AppendEvent(ctx, "recall.trace", "conv:"+trace.SessionID, trace.SessionID, 0, payload, "recall")
	return err
}

func (s *L1SQLiteStore) RecentRecallTraces(ctx context.Context, sessionID string, limit int) ([]domconv.RecallTrace, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(sessionID) == "" {
		rows, err = s.db.QueryContext(ctx, `
SELECT payload_json, created_at
FROM l1_event_log
WHERE event_type = 'recall.trace'
ORDER BY created_at DESC
LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
SELECT payload_json, created_at
FROM l1_event_log
WHERE event_type = 'recall.trace' AND session_id = ?
ORDER BY created_at DESC
LIMIT ?`, strings.TrimSpace(sessionID), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var traces []domconv.RecallTrace
	for rows.Next() {
		var payloadJSON string
		var createdAt time.Time
		if err := rows.Scan(&payloadJSON, &createdAt); err != nil {
			return nil, err
		}
		var payload struct {
			ResponseID string                    `json:"response_id"`
			SessionID  string                    `json:"session_id"`
			Role       string                    `json:"role"`
			Items      []domconv.RecallTraceItem `json:"items"`
			CreatedAt  string                    `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return nil, err
		}
		traceCreatedAt := createdAt
		if parsed, err := time.Parse(time.RFC3339, payload.CreatedAt); err == nil {
			traceCreatedAt = parsed
		}
		traces = append(traces, domconv.RecallTrace{
			ResponseID: payload.ResponseID,
			SessionID:  payload.SessionID,
			Role:       payload.Role,
			Items:      payload.Items,
			CreatedAt:  traceCreatedAt,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return traces, nil
}
