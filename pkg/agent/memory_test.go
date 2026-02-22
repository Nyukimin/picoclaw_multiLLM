package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func jst() *time.Location {
	loc, _ := time.LoadLocation("Asia/Tokyo")
	return loc
}

func TestGetCutoverBoundary(t *testing.T) {
	loc := jst()

	tests := []struct {
		name     string
		now      time.Time
		expected time.Time
	}{
		{
			name:     "after cutover hour in JST",
			now:      time.Date(2026, 2, 21, 10, 0, 0, 0, loc),
			expected: time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc),
		},
		{
			name:     "before cutover hour in JST",
			now:      time.Date(2026, 2, 21, 2, 0, 0, 0, loc),
			expected: time.Date(2026, 2, 20, CutoverHour, 0, 0, 0, loc),
		},
		{
			name:     "exactly at cutover hour in JST",
			now:      time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc),
			expected: time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc),
		},
		{
			name:     "UTC input is converted to JST",
			now:      time.Date(2026, 2, 21, 2, 0, 0, 0, time.UTC), // 11:00 JST
			expected: time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc),
		},
		{
			name:     "UTC evening maps to next JST day",
			now:      time.Date(2026, 2, 20, 20, 0, 0, 0, time.UTC), // Feb 21 05:00 JST
			expected: time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetCutoverBoundary(tt.now)
			if !got.Equal(tt.expected) {
				t.Errorf("GetCutoverBoundary(%v) = %v, want %v", tt.now, got, tt.expected)
			}
		})
	}
}

func TestGetLogicalDate(t *testing.T) {
	loc := jst()

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "JST afternoon belongs to same day",
			input:    time.Date(2026, 2, 21, 14, 0, 0, 0, loc),
			expected: time.Date(2026, 2, 21, 0, 0, 0, 0, loc),
		},
		{
			name:     "JST 2am belongs to previous day",
			input:    time.Date(2026, 2, 21, 2, 0, 0, 0, loc),
			expected: time.Date(2026, 2, 20, 0, 0, 0, 0, loc),
		},
		{
			name:     "UTC time is converted to JST before calculation",
			input:    time.Date(2026, 2, 20, 13, 56, 0, 0, time.UTC), // 22:56 JST
			expected: time.Date(2026, 2, 20, 0, 0, 0, 0, loc),
		},
		{
			name:     "UTC midnight = JST 09:00 belongs to same JST day",
			input:    time.Date(2026, 2, 21, 0, 0, 0, 0, time.UTC), // 09:00 JST Feb 21
			expected: time.Date(2026, 2, 21, 0, 0, 0, 0, loc),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetLogicalDate(tt.input)
			if !got.Equal(tt.expected) {
				t.Errorf("GetLogicalDate(%v) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCutoverBoundary_UTCSessionShouldTrigger(t *testing.T) {
	loc := jst()
	// Session updated at Feb 20 13:56 UTC = Feb 20 22:56 JST
	updated := time.Date(2026, 2, 20, 13, 56, 41, 0, time.UTC)
	// Now is Feb 21 02:35 UTC = Feb 21 11:35 JST
	now := time.Date(2026, 2, 21, 2, 35, 0, 0, time.UTC)

	boundary := GetCutoverBoundary(now)
	// Boundary should be Feb 21 04:00 JST = Feb 20 19:00 UTC
	expectedBoundary := time.Date(2026, 2, 21, CutoverHour, 0, 0, 0, loc)

	if !boundary.Equal(expectedBoundary) {
		t.Errorf("boundary = %v, want %v", boundary, expectedBoundary)
	}

	if !updated.Before(boundary) {
		t.Error("session updated at Feb 20 22:56 JST should be BEFORE boundary Feb 21 04:00 JST")
	}
}

func TestSaveDailyNoteForDate(t *testing.T) {
	tmpDir := t.TempDir()
	ms := NewMemoryStore(tmpDir)

	date := time.Date(2026, 2, 21, 0, 0, 0, 0, jst())
	content := "## Session Summary\n\nHad a good conversation."

	if err := ms.SaveDailyNoteForDate(date, content); err != nil {
		t.Fatalf("SaveDailyNoteForDate failed: %v", err)
	}

	expectedFile := filepath.Join(tmpDir, "memory", "202602", "20260221.md")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", expectedFile, err)
	}

	got := string(data)
	if got == "" {
		t.Fatal("daily note file is empty")
	}
	if !strings.Contains(got, "2026-02-21") {
		t.Error("daily note should contain date header")
	}
	if !strings.Contains(got, "Session Summary") {
		t.Error("daily note should contain session summary content")
	}
}

func TestSaveDailyNoteForDate_Append(t *testing.T) {
	tmpDir := t.TempDir()
	ms := NewMemoryStore(tmpDir)

	date := time.Date(2026, 2, 21, 0, 0, 0, 0, jst())

	ms.SaveDailyNoteForDate(date, "First entry.")
	ms.SaveDailyNoteForDate(date, "Second entry.")

	expectedFile := filepath.Join(tmpDir, "memory", "202602", "20260221.md")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	got := string(data)
	if !strings.Contains(got, "First entry.") {
		t.Error("should contain first entry")
	}
	if !strings.Contains(got, "Second entry.") {
		t.Error("should contain second entry")
	}
}

func TestFormatCutoverNote(t *testing.T) {
	note := FormatCutoverNote("User discussed rabbits.", []string{
		"- **user**: How are you?",
		"- **assistant**: I'm fine!",
	})

	if note == "" {
		t.Fatal("note should not be empty")
	}
	if !strings.Contains(note, "Session Summary") {
		t.Error("should contain Session Summary section")
	}
	if !strings.Contains(note, "Last Messages") {
		t.Error("should contain Last Messages section")
	}
	if !strings.Contains(note, "User discussed rabbits.") {
		t.Error("should contain the summary text")
	}
}

func TestFormatCutoverNote_EmptyInput(t *testing.T) {
	note := FormatCutoverNote("", nil)
	if note != "" {
		t.Errorf("expected empty note for empty input, got %q", note)
	}
}
