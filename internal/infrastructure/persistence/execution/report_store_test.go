package execution

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	domain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
)

func TestJSONLReportStore_SaveAndListRecent(t *testing.T) {
	store, err := NewJSONLReportStore(filepath.Join(t.TempDir(), "execution_report.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLReportStore failed: %v", err)
	}

	r1 := domain.ExecutionReport{
		JobID:      "j1",
		Goal:       "TTS実装して",
		Status:     "passed",
		CreatedAt:  time.Now().UTC().Add(-1 * time.Minute),
		FinishedAt: time.Now().UTC().Add(-30 * time.Second),
	}
	r2 := domain.ExecutionReport{
		JobID:      "j2",
		Goal:       "ログ確認して",
		Status:     "failed",
		CreatedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := store.Save(context.Background(), r1); err != nil {
		t.Fatalf("Save r1 failed: %v", err)
	}
	if err := store.Save(context.Background(), r2); err != nil {
		t.Fatalf("Save r2 failed: %v", err)
	}

	items, err := store.ListRecent(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListRecent failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].JobID != "j2" {
		t.Fatalf("expected most recent j2, got %s", items[0].JobID)
	}
}

func TestJSONLReportStore_GetByJobID(t *testing.T) {
	store, err := NewJSONLReportStore(filepath.Join(t.TempDir(), "execution_report.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLReportStore failed: %v", err)
	}

	r1 := domain.ExecutionReport{
		JobID:      "job-x",
		Goal:       "first",
		Status:     "failed",
		CreatedAt:  time.Now().UTC().Add(-2 * time.Minute),
		FinishedAt: time.Now().UTC().Add(-2 * time.Minute),
	}
	r2 := domain.ExecutionReport{
		JobID:      "job-x",
		Goal:       "second",
		Status:     "passed",
		CreatedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if err := store.Save(context.Background(), r1); err != nil {
		t.Fatalf("Save r1 failed: %v", err)
	}
	if err := store.Save(context.Background(), r2); err != nil {
		t.Fatalf("Save r2 failed: %v", err)
	}

	got, err := store.GetByJobID(context.Background(), "job-x")
	if err != nil {
		t.Fatalf("GetByJobID failed: %v", err)
	}
	if got.Goal != "second" || got.Status != "passed" {
		t.Fatalf("unexpected report: %+v", got)
	}
}

func TestJSONLReportStore_Summary(t *testing.T) {
	store, err := NewJSONLReportStore(filepath.Join(t.TempDir(), "execution_report.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLReportStore failed: %v", err)
	}

	items := []domain.ExecutionReport{
		{JobID: "j1", Goal: "a", Status: "passed", CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
		{JobID: "j2", Goal: "b", Status: "failed", ErrorKind: "verify", CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
		{JobID: "j3", Goal: "c", Status: "failed", ErrorKind: "apply", CreatedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
	}
	for _, it := range items {
		if err := store.Save(context.Background(), it); err != nil {
			t.Fatalf("Save failed: %v", err)
		}
	}

	s, err := store.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary failed: %v", err)
	}
	if s["status"]["passed"] != 1 || s["status"]["failed"] != 2 {
		t.Fatalf("unexpected status summary: %+v", s)
	}
	if s["error_kind"]["verify"] != 1 || s["error_kind"]["apply"] != 1 {
		t.Fatalf("unexpected error_kind summary: %+v", s)
	}
}

func TestJSONLReportStore_SaveWithTTSEvidence(t *testing.T) {
	store, err := NewJSONLReportStore(filepath.Join(t.TempDir(), "execution_report.jsonl"))
	if err != nil {
		t.Fatalf("NewJSONLReportStore failed: %v", err)
	}

	in := domain.ExecutionReport{
		JobID:        "tts-job",
		Goal:         "TTS実装して",
		Status:       "passed",
		TTSProvider:  "sbv2",
		TTSVoiceID:   "mio",
		TTSAudioFile: "/tmp/sbv2.wav",
		TTSDuration:  1234,
		PlaybackCmd:  "ffplay -autoexit -nodisp /tmp/sbv2.wav",
		PlaybackCode: 0,
		CreatedAt:    time.Now().UTC(),
		FinishedAt:   time.Now().UTC(),
	}
	if err := store.Save(context.Background(), in); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.GetByJobID(context.Background(), "tts-job")
	if err != nil {
		t.Fatalf("GetByJobID failed: %v", err)
	}
	if got.TTSProvider != "sbv2" || got.PlaybackCode != 0 {
		t.Fatalf("unexpected tts evidence: %+v", got)
	}
}
