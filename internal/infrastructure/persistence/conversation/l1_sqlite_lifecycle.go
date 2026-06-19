package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type MemoryLifecycleOptions struct {
	Now                      time.Time
	RawConversationRetention time.Duration
	CandidateReviewAfter     time.Duration
	DecayAfter               time.Duration
	RawCompactLimit          int
	CandidateReviewLimit     int
	DecayLimit               int
	VectorCleanupLimit       int
}

type MemoryLifecycleResult struct {
	RawCompacted        int
	CandidatesQueued    int
	Decayed             int
	VectorCleanupQueued int
}

func DefaultMemoryLifecycleOptions() MemoryLifecycleOptions {
	return MemoryLifecycleOptions{
		Now:                      time.Now().UTC(),
		RawConversationRetention: 30 * 24 * time.Hour,
		CandidateReviewAfter:     7 * 24 * time.Hour,
		DecayAfter:               90 * 24 * time.Hour,
		RawCompactLimit:          1000,
		CandidateReviewLimit:     200,
		DecayLimit:               200,
		VectorCleanupLimit:       200,
	}
}

func (s *L1SQLiteStore) RunMemoryLifecycleMaintenance(ctx context.Context, opts MemoryLifecycleOptions) (*MemoryLifecycleResult, error) {
	opts = normalizeMemoryLifecycleOptions(opts)
	result := &MemoryLifecycleResult{}
	if opts.RawConversationRetention > 0 {
		n, err := s.compactOldConversationRaw(ctx, opts.Now.Add(-opts.RawConversationRetention), opts.RawCompactLimit)
		if err != nil {
			return nil, err
		}
		result.RawCompacted = n
	}
	if opts.CandidateReviewAfter > 0 {
		n, err := s.queueUserMemoryCandidateReview(ctx, opts.Now, opts.Now.Add(-opts.CandidateReviewAfter), opts.CandidateReviewLimit)
		if err != nil {
			return nil, err
		}
		result.CandidatesQueued = n
	}
	if opts.DecayAfter > 0 {
		n, err := s.markDecayedUserMemories(ctx, opts.Now, opts.Now.Add(-opts.DecayAfter), opts.DecayLimit)
		if err != nil {
			return nil, err
		}
		result.Decayed = n
	}
	n, err := s.queueVectorCleanup(ctx, opts.Now, opts.VectorCleanupLimit)
	if err != nil {
		return nil, err
	}
	result.VectorCleanupQueued = n
	if result.RawCompacted > 0 || result.CandidatesQueued > 0 || result.Decayed > 0 || result.VectorCleanupQueued > 0 {
		if _, err := s.AppendEvent(ctx, "memory.lifecycle_maintenance_completed", "conv:lifecycle", "", 0, map[string]interface{}{
			"raw_compacted":         result.RawCompacted,
			"candidates_queued":     result.CandidatesQueued,
			"decayed":               result.Decayed,
			"vector_cleanup_queued": result.VectorCleanupQueued,
		}, "memory_lifecycle"); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func normalizeMemoryLifecycleOptions(opts MemoryLifecycleOptions) MemoryLifecycleOptions {
	defaults := DefaultMemoryLifecycleOptions()
	if opts.Now.IsZero() {
		opts.Now = defaults.Now
	}
	if opts.RawCompactLimit <= 0 {
		opts.RawCompactLimit = defaults.RawCompactLimit
	}
	if opts.CandidateReviewLimit <= 0 {
		opts.CandidateReviewLimit = defaults.CandidateReviewLimit
	}
	if opts.DecayLimit <= 0 {
		opts.DecayLimit = defaults.DecayLimit
	}
	if opts.VectorCleanupLimit <= 0 {
		opts.VectorCleanupLimit = defaults.VectorCleanupLimit
	}
	return opts
}

func (s *L1SQLiteStore) compactOldConversationRaw(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM l1_memory_event
WHERE id IN (
	SELECT id FROM l1_memory_event
	WHERE namespace LIKE 'conv:%'
	  AND memory_state = ?
	  AND created_at < ?
	ORDER BY created_at ASC
	LIMIT ?
)`, MemoryStateObserved, cutoff.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("failed to compact old conversation raw memory: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if affected > 0 {
		if _, err := s.AppendEvent(ctx, "memory.l1_raw_compacted", "conv:lifecycle", "", 0, map[string]interface{}{
			"cutoff": cutoff.UTC().Format(time.RFC3339),
			"count":  affected,
		}, "memory_lifecycle"); err != nil {
			return 0, err
		}
	}
	return int(affected), nil
}

func (s *L1SQLiteStore) queueUserMemoryCandidateReview(ctx context.Context, now time.Time, cutoff time.Time, limit int) (int, error) {
	events, err := s.userMemoryEventsForLifecycle(ctx, `
WHERE namespace LIKE 'user:%'
  AND memory_state = ?
  AND created_at < ?
ORDER BY created_at ASC
LIMIT ?`, MemoryStateCandidate, cutoff.UTC(), limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, ev := range events {
		if !lifecycleMemoryActive(ev.Meta) || strings.TrimSpace(metaStringValue(ev.Meta, "review_status")) == "queued" {
			continue
		}
		meta := cloneMeta(ev.Meta)
		meta["review_status"] = "queued"
		meta["review_queued_at"] = now.UTC().Format(time.RFC3339)
		if err := s.updateMemoryMeta(ctx, ev.ID, meta); err != nil {
			return count, err
		}
		if _, err := s.AppendEvent(ctx, "memory.candidate_review_queued", ev.Namespace, ev.SessionID, ev.ThreadID, map[string]interface{}{
			"memory_id": ev.ID,
		}, "memory_lifecycle"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *L1SQLiteStore) markDecayedUserMemories(ctx context.Context, now time.Time, cutoff time.Time, limit int) (int, error) {
	events, err := s.userMemoryEventsForLifecycle(ctx, `
WHERE namespace LIKE 'user:%'
  AND memory_state = ?
  AND updated_at < ?
ORDER BY updated_at ASC
LIMIT ?`, MemoryStateConfirmed, cutoff.UTC(), limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, ev := range events {
		if !lifecycleMemoryActive(ev.Meta) || strings.TrimSpace(metaStringValue(ev.Meta, "superseded_by")) != "" {
			continue
		}
		if strings.TrimSpace(metaStringValue(ev.Meta, "lifecycle_status")) == "decayed" {
			continue
		}
		meta := cloneMeta(ev.Meta)
		meta["lifecycle_status"] = "decayed"
		meta["decay_score"] = 0.5
		meta["decayed_at"] = now.UTC().Format(time.RFC3339)
		if err := s.updateMemoryMeta(ctx, ev.ID, meta); err != nil {
			return count, err
		}
		if _, err := s.AppendEvent(ctx, "memory.decayed", ev.Namespace, ev.SessionID, ev.ThreadID, map[string]interface{}{
			"memory_id":   ev.ID,
			"decay_score": 0.5,
		}, "memory_lifecycle"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *L1SQLiteStore) queueVectorCleanup(ctx context.Context, now time.Time, limit int) (int, error) {
	events, err := s.userMemoryEventsForLifecycle(ctx, `
WHERE namespace LIKE 'user:%'
ORDER BY updated_at ASC
LIMIT ?`, limit)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, ev := range events {
		if lifecycleMemoryActive(ev.Meta) && strings.TrimSpace(metaStringValue(ev.Meta, "superseded_by")) == "" {
			continue
		}
		if strings.TrimSpace(metaStringValue(ev.Meta, "vector_cleanup_status")) == "queued" {
			continue
		}
		meta := cloneMeta(ev.Meta)
		meta["vector_cleanup_status"] = "queued"
		meta["vector_cleanup_queued_at"] = now.UTC().Format(time.RFC3339)
		if err := s.updateMemoryMeta(ctx, ev.ID, meta); err != nil {
			return count, err
		}
		if _, err := s.AppendEvent(ctx, "memory.vector_cleanup_queued", ev.Namespace, ev.SessionID, ev.ThreadID, map[string]interface{}{
			"memory_id": ev.ID,
		}, "memory_lifecycle"); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *L1SQLiteStore) userMemoryEventsForLifecycle(ctx context.Context, where string, args ...interface{}) ([]L1MemoryEvent, error) {
	query := `
SELECT id, namespace, session_id, thread_id, speaker, message, meta_json,
       memory_state, layer, source, created_at, updated_at
FROM l1_memory_event
` + where
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query lifecycle user memories: %w", err)
	}
	defer rows.Close()
	return scanL1Events(rows)
}

func lifecycleMemoryActive(meta map[string]interface{}) bool {
	if meta == nil {
		return true
	}
	raw, ok := meta["active"]
	if !ok {
		return true
	}
	active, ok := raw.(bool)
	if !ok {
		return true
	}
	return active
}

func cloneMeta(meta map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range meta {
		out[k] = v
	}
	return out
}
