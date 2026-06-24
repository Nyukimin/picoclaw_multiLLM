package viewer

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestEventLogGCServiceRunOnceRemovesExpiredItems(t *testing.T) {
	store, err := NewEventLogStore(filepath.Join(t.TempDir(), "orchestrator_event_log.jsonl"))
	if err != nil {
		t.Fatalf("NewEventLogStore failed: %v", err)
	}
	now := time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC)
	oldTs := now.Add(-16 * 24 * time.Hour).Format(time.RFC3339)
	newTs := now.Add(-2 * time.Hour).Format(time.RFC3339)
	_ = store.Append(orchestrator.OrchestratorEvent{Type: "agent.note", From: "mio", Timestamp: oldTs})
	_ = store.Append(orchestrator.OrchestratorEvent{Type: "agent.note", From: "shiro", Timestamp: newTs})

	gcLogPath := filepath.Join(t.TempDir(), "orchestrator_event_gc.jsonl")
	svc, err := NewEventLogGCService(store, gcLogPath, 14, 60)
	if err != nil {
		t.Fatalf("NewEventLogGCService failed: %v", err)
	}

	report, err := svc.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if report.DeletedCount != 1 {
		t.Fatalf("deleted_count = %d, want 1", report.DeletedCount)
	}
	items, err := store.Query(context.Background(), LogFilter{Limit: 10})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if len(items) != 1 || items[0].From != "shiro" {
		t.Fatalf("unexpected remaining items: %+v", items)
	}
}

func TestEventLogGCServiceRunOnceReportsPartialError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orchestrator_event_log.jsonl")
	if err := os.WriteFile(path, []byte("{bad json}\n"+`{"type":"agent.note","from":"mio","timestamp":"broken"}`+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	store, err := NewEventLogStore(path)
	if err != nil {
		t.Fatalf("NewEventLogStore failed: %v", err)
	}
	gcLogPath := filepath.Join(dir, "orchestrator_event_gc.jsonl")
	svc, err := NewEventLogGCService(store, gcLogPath, 14, 60)
	if err != nil {
		t.Fatalf("NewEventLogGCService failed: %v", err)
	}

	report, err := svc.RunOnce(context.Background(), time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if report.Status != "partial_error" {
		t.Fatalf("status = %q, want partial_error", report.Status)
	}
	f, err := os.Open(gcLogPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("expected gc report line")
	}
	var got EventGCReport
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if got.DecodeErrorCount == 0 || got.TimestampErrorCount == 0 {
		t.Fatalf("expected decode/timestamp error counts, got %+v", got)
	}
}
