package cron

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveStore_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}

	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cron", "jobs.json")

	cs := NewCronService(storePath, nil)

	_, err := cs.AddJob("test", CronSchedule{Kind: "every", EveryMS: int64Ptr(60000)}, "hello", false, "cli", "direct")
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	info, err := os.Stat(storePath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("cron store has permission %04o, want 0600", perm)
	}
}

func int64Ptr(v int64) *int64 {
	return &v
}

func TestLoadStore_AutoRepairOnTruncatedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "cron", "jobs.json")
	if err := os.MkdirAll(filepath.Dir(storePath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// Simulate interrupted write.
	if err := os.WriteFile(storePath, []byte("{"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cs := NewCronService(storePath, nil)
	if err := cs.Load(); err != nil {
		t.Fatalf("Load should auto-repair truncated store, got error: %v", err)
	}

	data, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	want := "{\n  \"version\": 1,\n  \"jobs\": []\n}"
	if string(data) != want {
		t.Fatalf("unexpected repaired store content:\n%s", string(data))
	}
}
