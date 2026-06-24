package viewer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGeneratedFileGCServiceRunOnceRemovesExpiredMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	oldWAV := filepath.Join(dir, "viewer-tts-old.wav")
	newWAV := filepath.Join(dir, "viewer-tts-new.wav")
	other := filepath.Join(dir, "manual.wav")
	for _, path := range []string{oldWAV, newWAV, other} {
		if err := os.WriteFile(path, []byte("wav"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}
	if err := os.Chtimes(oldWAV, now.Add(-45*time.Minute), now.Add(-45*time.Minute)); err != nil {
		t.Fatalf("Chtimes old failed: %v", err)
	}
	if err := os.Chtimes(newWAV, now.Add(-5*time.Minute), now.Add(-5*time.Minute)); err != nil {
		t.Fatalf("Chtimes new failed: %v", err)
	}
	if err := os.Chtimes(other, now.Add(-45*time.Minute), now.Add(-45*time.Minute)); err != nil {
		t.Fatalf("Chtimes other failed: %v", err)
	}

	svc, err := NewGeneratedFileGCService(dir, "viewer-tts-*.wav", 30*time.Minute, 10*time.Minute)
	if err != nil {
		t.Fatalf("NewGeneratedFileGCService failed: %v", err)
	}
	report, err := svc.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if report.DeletedCount != 1 {
		t.Fatalf("deleted_count = %d, want 1", report.DeletedCount)
	}
	if _, err := os.Stat(oldWAV); !os.IsNotExist(err) {
		t.Fatalf("old wav should be removed, err=%v", err)
	}
	for _, path := range []string{newWAV, other} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s should remain: %v", path, err)
		}
	}
}
