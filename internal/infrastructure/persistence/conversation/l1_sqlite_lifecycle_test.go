package conversation

import (
	"context"
	"path/filepath"
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

func backdateMemory(t *testing.T, store *L1SQLiteStore, id string, at time.Time) {
	t.Helper()
	if _, err := store.db.ExecContext(context.Background(), `UPDATE l1_memory_event SET created_at = ?, updated_at = ? WHERE id = ?`, at.UTC(), at.UTC(), id); err != nil {
		t.Fatalf("backdate memory %s failed: %v", id, err)
	}
}
