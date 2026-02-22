package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"telegram:123456", "telegram_123456"},
		{"discord:987654321", "discord_987654321"},
		{"slack:C01234", "slack_C01234"},
		{"no-colons-here", "no-colons-here"},
		{"multiple:colons:here", "multiple_colons_here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSave_WithColonInKey(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session with a key containing colon (typical channel session key).
	key := "telegram:123456"
	sm.GetOrCreate(key)
	sm.AddMessage(key, "user", "hello")

	// Save should succeed even though the key contains ':'
	if err := sm.Save(key); err != nil {
		t.Fatalf("Save(%q) failed: %v", key, err)
	}

	// The file on disk should use sanitized name.
	expectedFile := filepath.Join(tmpDir, "telegram_123456.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("expected session file %s to exist", expectedFile)
	}

	// Load into a fresh manager and verify the session round-trips.
	sm2 := NewSessionManager(tmpDir)
	history := sm2.GetHistory(key)
	if len(history) != 1 {
		t.Fatalf("expected 1 message after reload, got %d", len(history))
	}
	if history[0].Content != "hello" {
		t.Errorf("expected message content %q, got %q", "hello", history[0].Content)
	}
}

func TestSave_RejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	badKeys := []string{"", ".", "..", "foo/bar", "foo\\bar"}
	for _, key := range badKeys {
		sm.GetOrCreate(key)
		if err := sm.Save(key); err == nil {
			t.Errorf("Save(%q) should have failed but didn't", key)
		}
	}
}

func TestSessionFlags_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	key := "slack:C0123"
	flags := SessionFlags{
		LocalOnly:        true,
		PrevPrimaryRoute: "PLAN",
	}
	sm.SetFlags(key, flags)
	if err := sm.Save(key); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	sm2 := NewSessionManager(tmpDir)
	got := sm2.GetFlags(key)
	if !got.LocalOnly {
		t.Fatalf("expected LocalOnly to be true")
	}
	if got.PrevPrimaryRoute != "PLAN" {
		t.Fatalf("expected PrevPrimaryRoute PLAN, got %s", got.PrevPrimaryRoute)
	}
}

func TestWorkOverlayFlags_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	key := "telegram:99999"
	flags := SessionFlags{
		WorkOverlayTurnsLeft: 5,
		WorkOverlayDirective: "test directive",
	}
	sm.SetFlags(key, flags)
	if err := sm.Save(key); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	sm2 := NewSessionManager(tmpDir)
	got := sm2.GetFlags(key)
	if got.WorkOverlayTurnsLeft != 5 {
		t.Fatalf("expected WorkOverlayTurnsLeft 5, got %d", got.WorkOverlayTurnsLeft)
	}
	if got.WorkOverlayDirective != "test directive" {
		t.Fatalf("expected WorkOverlayDirective 'test directive', got %q", got.WorkOverlayDirective)
	}
}

func TestWorkOverlayFlags_OmitEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	key := "line:omit"
	flags := SessionFlags{
		WorkOverlayTurnsLeft: 0,
		WorkOverlayDirective: "",
	}
	sm.SetFlags(key, flags)
	if err := sm.Save(key); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	sm2 := NewSessionManager(tmpDir)
	got := sm2.GetFlags(key)
	if got.WorkOverlayTurnsLeft != 0 {
		t.Fatalf("expected WorkOverlayTurnsLeft 0, got %d", got.WorkOverlayTurnsLeft)
	}
	if got.WorkOverlayDirective != "" {
		t.Fatalf("expected empty WorkOverlayDirective, got %q", got.WorkOverlayDirective)
	}
}

func TestGetUpdatedTime(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	key := "line:U12345"
	got := sm.GetUpdatedTime(key)
	if !got.IsZero() {
		t.Fatalf("expected zero time for non-existent session, got %v", got)
	}

	sm.GetOrCreate(key)
	got = sm.GetUpdatedTime(key)
	if got.IsZero() {
		t.Fatal("expected non-zero time after GetOrCreate")
	}
}

func TestResetSession(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	key := "line:U99999"
	sm.GetOrCreate(key)
	sm.AddMessage(key, "user", "hello")
	sm.AddMessage(key, "assistant", "hi there")
	sm.SetSummary(key, "User greeted assistant.")

	if len(sm.GetHistory(key)) != 2 {
		t.Fatalf("expected 2 messages before reset, got %d", len(sm.GetHistory(key)))
	}
	if sm.GetSummary(key) == "" {
		t.Fatal("expected non-empty summary before reset")
	}

	beforeReset := sm.GetUpdatedTime(key)
	time.Sleep(10 * time.Millisecond)

	sm.ResetSession(key)

	history := sm.GetHistory(key)
	if len(history) != 0 {
		t.Fatalf("expected 0 messages after reset, got %d", len(history))
	}
	if sm.GetSummary(key) != "" {
		t.Fatalf("expected empty summary after reset, got %q", sm.GetSummary(key))
	}

	afterReset := sm.GetUpdatedTime(key)
	if !afterReset.After(beforeReset) {
		t.Error("expected Updated to advance after reset")
	}

	// Flags should be preserved
	sm.SetFlags(key, SessionFlags{PrevPrimaryRoute: "CHAT"})
	sm.ResetSession(key)
	if sm.GetFlags(key).PrevPrimaryRoute != "CHAT" {
		t.Error("expected flags to be preserved after reset")
	}
}

func TestResetSession_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Should not panic
	sm.ResetSession("nonexistent:key")
}
