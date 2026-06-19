package conversation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	domainmemory "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/memory"
)

func TestL1SQLiteStore_RunMemoryLifecycleMaintenance(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	old := now.Add(-120 * 24 * time.Hour)
	if err := store.SaveMessage(ctx, "session-old", 1, "conv:1", domconv.NewMessage(domconv.SpeakerUser, "old raw", nil), MemoryStateObserved); err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE l1_memory_event SET created_at = ?, updated_at = ? WHERE namespace = 'conv:1'`, old, old); err != nil {
		t.Fatalf("backdate conv memory failed: %v", err)
	}

	candidate, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:      "ren",
		Type:        domainmemory.UserMemoryTypePreference,
		Statement:   "candidate review",
		State:       MemoryStateCandidate,
		Sensitivity: "normal",
		Scope:       "all_personas",
	})
	if err != nil {
		t.Fatalf("Create candidate failed: %v", err)
	}
	backdateMemory(t, store, candidate.ID, old)

	confirmed, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypePreference,
		Statement:        "old confirmed",
		State:            MemoryStateConfirmed,
		EvidenceEventIDs: []string{"evt-confirmed"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create confirmed failed: %v", err)
	}
	backdateMemory(t, store, confirmed.ID, old)

	pinned, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypeConstraint,
		Statement:        "old pinned",
		State:            MemoryStatePinned,
		EvidenceEventIDs: []string{"evt-pinned"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create pinned failed: %v", err)
	}
	backdateMemory(t, store, pinned.ID, old)

	forgotten, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypePreference,
		Statement:        "forgotten",
		State:            MemoryStateConfirmed,
		EvidenceEventIDs: []string{"evt-forgotten"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create forgotten failed: %v", err)
	}
	if _, err := store.ForgetUserMemory(ctx, forgotten.ID, "test"); err != nil {
		t.Fatalf("ForgetUserMemory failed: %v", err)
	}

	result, err := store.RunMemoryLifecycleMaintenance(ctx, MemoryLifecycleOptions{
		Now:                      now,
		RawConversationRetention: 30 * 24 * time.Hour,
		CandidateReviewAfter:     7 * 24 * time.Hour,
		DecayAfter:               90 * 24 * time.Hour,
		RawCompactLimit:          10,
		CandidateReviewLimit:     10,
		DecayLimit:               10,
		VectorCleanupLimit:       10,
	})
	if err != nil {
		t.Fatalf("RunMemoryLifecycleMaintenance failed: %v", err)
	}
	if result.RawCompacted != 1 || result.CandidatesQueued != 1 || result.Decayed != 1 || result.VectorCleanupQueued != 1 {
		t.Fatalf("unexpected lifecycle result: %+v", result)
	}

	candidateEvent, err := store.memoryByID(ctx, candidate.ID)
	if err != nil {
		t.Fatalf("candidate memory missing: %v", err)
	}
	if got := metaStringValue(candidateEvent.Meta, "review_status"); got != "queued" {
		t.Fatalf("candidate review_status=%q, want queued", got)
	}
	confirmedEvent, err := store.memoryByID(ctx, confirmed.ID)
	if err != nil {
		t.Fatalf("confirmed memory missing: %v", err)
	}
	confirmedMemory := l1EventToUserMemory(*confirmedEvent)
	if confirmedMemory.LifecycleStatus != "decayed" || domainmemory.IsUserMemoryPromptInjectable(*confirmedMemory, "mio") {
		t.Fatalf("confirmed memory should be decayed and not prompt-injectable: %+v", confirmedMemory)
	}
	pinnedEvent, err := store.memoryByID(ctx, pinned.ID)
	if err != nil {
		t.Fatalf("pinned memory missing: %v", err)
	}
	if got := metaStringValue(pinnedEvent.Meta, "lifecycle_status"); got != "" {
		t.Fatalf("pinned memory should not decay, got lifecycle_status=%q", got)
	}
	forgottenEvent, err := store.memoryByID(ctx, forgotten.ID)
	if err != nil {
		t.Fatalf("forgotten memory missing: %v", err)
	}
	if got := metaStringValue(forgottenEvent.Meta, "vector_cleanup_status"); got != "queued" {
		t.Fatalf("forgotten memory vector_cleanup_status=%q, want queued", got)
	}
}

func TestL1SQLiteStore_RunMemoryLifecycleMaintenanceExecutesVectorCleanup(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()
	sink := &stubVectorCleanupSink{}
	store.WithVectorCleanupSink(sink)

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	forgotten, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypePreference,
		Statement:        "forgotten vector memory",
		State:            MemoryStateConfirmed,
		EvidenceEventIDs: []string{"evt-forgotten"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create forgotten failed: %v", err)
	}
	if _, err := store.ForgetUserMemory(ctx, forgotten.ID, "test"); err != nil {
		t.Fatalf("ForgetUserMemory failed: %v", err)
	}

	result, err := store.RunMemoryLifecycleMaintenance(ctx, MemoryLifecycleOptions{
		Now:                    now,
		RawCompactLimit:        10,
		CandidateReviewLimit:   10,
		MonthlyHighlightLimit:  10,
		ThreadSummarySeedLimit: 10,
		DecayLimit:             10,
		VectorCleanupLimit:     10,
	})
	if err != nil {
		t.Fatalf("RunMemoryLifecycleMaintenance failed: %v", err)
	}
	if result.VectorCleanupQueued != 1 || result.VectorCleanupExecuted != 1 {
		t.Fatalf("unexpected vector cleanup result: %+v", result)
	}
	if len(sink.items) != 1 || sink.items[0].MemoryID != forgotten.ID {
		t.Fatalf("unexpected cleanup sink items: %+v", sink.items)
	}
	forgottenEvent, err := store.memoryByID(ctx, forgotten.ID)
	if err != nil {
		t.Fatalf("forgotten memory missing: %v", err)
	}
	if got := metaStringValue(forgottenEvent.Meta, "vector_cleanup_status"); got != "done" {
		t.Fatalf("vector cleanup status=%q, want done", got)
	}
}

func TestL1SQLiteStore_RunMemoryLifecycleMaintenanceBuildsMonthlyHighlightsAndThreadSeeds(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	insertDailyDigest(t, store, "digest:2026-05-01:ai", "2026-05-01", "ai", "day", "AI daily one")
	insertDailyDigest(t, store, "digest:2026-05-02:ai", "2026-05-02", "ai", "day", "AI daily two")
	insertThreadSummary(t, store, "summary-1", "conv:thread", 42, "thread summary for May", now.Add(-20*24*time.Hour))

	result, err := store.RunMemoryLifecycleMaintenance(ctx, MemoryLifecycleOptions{
		Now:                    now,
		MonthlyHighlightAfter:  7 * 24 * time.Hour,
		ThreadSummarySeedAfter: 7 * 24 * time.Hour,
		MonthlyHighlightLimit:  10,
		ThreadSummarySeedLimit: 10,
		RawCompactLimit:        10,
		CandidateReviewLimit:   10,
		DecayLimit:             10,
		VectorCleanupLimit:     10,
	})
	if err != nil {
		t.Fatalf("RunMemoryLifecycleMaintenance failed: %v", err)
	}
	if result.MonthlyHighlightsBuilt != 1 || result.ThreadSummarySeedsQueued != 1 {
		t.Fatalf("unexpected lifecycle result: %+v", result)
	}
	var highlight string
	if err := store.db.QueryRowContext(ctx, `SELECT highlight_text FROM l1_monthly_highlight WHERE month = ? AND category = ?`, "2026-05", "ai").Scan(&highlight); err != nil {
		t.Fatalf("monthly highlight missing: %v", err)
	}
	if !strings.Contains(highlight, "AI daily one") || !strings.Contains(highlight, "AI daily two") {
		t.Fatalf("monthly highlight did not include daily digests: %q", highlight)
	}
	summaryEvent, err := store.memoryByID(ctx, "summary-1")
	if err != nil {
		t.Fatalf("thread summary missing: %v", err)
	}
	if got := metaStringValue(summaryEvent.Meta, "monthly_highlight_seed_status"); got != "queued" {
		t.Fatalf("thread summary seed status=%q, want queued", got)
	}

	result, err = store.RunMemoryLifecycleMaintenance(ctx, MemoryLifecycleOptions{
		Now:                    now,
		MonthlyHighlightAfter:  7 * 24 * time.Hour,
		ThreadSummarySeedAfter: 7 * 24 * time.Hour,
		MonthlyHighlightLimit:  10,
		ThreadSummarySeedLimit: 10,
		RawCompactLimit:        10,
		CandidateReviewLimit:   10,
		DecayLimit:             10,
		VectorCleanupLimit:     10,
	})
	if err != nil {
		t.Fatalf("second RunMemoryLifecycleMaintenance failed: %v", err)
	}
	if result.MonthlyHighlightsBuilt != 0 || result.ThreadSummarySeedsQueued != 0 {
		t.Fatalf("monthly highlight and thread seed should be idempotent, got %+v", result)
	}
}

func TestL1SQLiteStore_RunMemoryLifecycleMaintenanceUsesDomainDecayPolicy(t *testing.T) {
	ctx := context.Background()
	store, err := NewL1SQLiteStore(filepath.Join(t.TempDir(), "l1.db"))
	if err != nil {
		t.Fatalf("NewL1SQLiteStore failed: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	project, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypeProject,
		Statement:        "project memory survives normal decay",
		State:            MemoryStateConfirmed,
		EvidenceEventIDs: []string{"evt-project"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create project failed: %v", err)
	}
	backdateMemory(t, store, project.ID, now.Add(-120*24*time.Hour))
	episode, err := store.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
		UserID:           "ren",
		Type:             domainmemory.UserMemoryTypeEpisode,
		Statement:        "episode memory decays sooner",
		State:            MemoryStateConfirmed,
		EvidenceEventIDs: []string{"evt-episode"},
		Sensitivity:      "normal",
		Scope:            "all_personas",
		Source:           "user_explicit",
	})
	if err != nil {
		t.Fatalf("Create episode failed: %v", err)
	}
	backdateMemory(t, store, episode.ID, now.Add(-45*24*time.Hour))

	result, err := store.RunMemoryLifecycleMaintenance(ctx, MemoryLifecycleOptions{
		Now:                    now,
		DecayAfter:             90 * 24 * time.Hour,
		RawCompactLimit:        10,
		CandidateReviewLimit:   10,
		MonthlyHighlightLimit:  10,
		ThreadSummarySeedLimit: 10,
		DecayLimit:             10,
		VectorCleanupLimit:     10,
	})
	if err != nil {
		t.Fatalf("RunMemoryLifecycleMaintenance failed: %v", err)
	}
	if result.Decayed != 1 {
		t.Fatalf("unexpected decay count: %+v", result)
	}
	projectEvent, err := store.memoryByID(ctx, project.ID)
	if err != nil {
		t.Fatalf("project memory missing: %v", err)
	}
	if got := metaStringValue(projectEvent.Meta, "lifecycle_status"); got != "" {
		t.Fatalf("project memory should not decay at 120 days, got %q", got)
	}
	episodeEvent, err := store.memoryByID(ctx, episode.ID)
	if err != nil {
		t.Fatalf("episode memory missing: %v", err)
	}
	if got := metaStringValue(episodeEvent.Meta, "lifecycle_status"); got != "decayed" {
		t.Fatalf("episode lifecycle_status=%q, want decayed", got)
	}
	if got := metaStringValue(episodeEvent.Meta, "decay_policy"); got != "short" {
		t.Fatalf("episode decay_policy=%q, want short", got)
	}
}

type stubVectorCleanupSink struct {
	items []L1VectorCleanupItem
}

func (s *stubVectorCleanupSink) CleanupMemoryVectors(_ context.Context, items []L1VectorCleanupItem) (*L1VectorCleanupResult, error) {
	s.items = append(s.items, items...)
	return &L1VectorCleanupResult{Deleted: len(items)}, nil
}

func backdateMemory(t *testing.T, store *L1SQLiteStore, id string, at time.Time) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `UPDATE l1_memory_event SET created_at = ?, updated_at = ? WHERE id = ?`, at.UTC(), at.UTC(), id); err != nil {
		t.Fatalf("backdate memory %s failed: %v", id, err)
	}
}

func insertDailyDigest(t *testing.T, store *L1SQLiteStore, id string, date string, category string, slot string, text string) {
	t.Helper()
	newsIDs, err := json.Marshal([]string{"news:" + id})
	if err != nil {
		t.Fatalf("marshal news ids failed: %v", err)
	}
	now := time.Now().UTC()
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO l1_daily_digest (
	id, digest_date, category, digest_slot, news_ids_json, digest_text, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, date, category, slot, string(newsIDs), text, now, now); err != nil {
		t.Fatalf("insert daily digest failed: %v", err)
	}
}

func insertThreadSummary(t *testing.T, store *L1SQLiteStore, id string, namespace string, threadID int64, text string, at time.Time) {
	t.Helper()
	meta, err := json.Marshal(map[string]interface{}{"kind": "thread_summary"})
	if err != nil {
		t.Fatalf("marshal thread summary meta failed: %v", err)
	}
	if _, err := store.db.ExecContext(context.Background(), `
INSERT INTO l1_memory_event (
	id, namespace, session_id, thread_id, speaker, message, meta_json,
	memory_state, layer, source, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, namespace, "session-thread", threadID, string(domconv.SpeakerSystem), text, string(meta),
		MemoryStateConfirmed, "L2", "thread_summary", at.UTC(), at.UTC()); err != nil {
		t.Fatalf("insert thread summary failed: %v", err)
	}
}
