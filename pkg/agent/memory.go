// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const CutoverHour = 4
const CutoverTimezone = "Asia/Tokyo"

// cutoverLocation returns the timezone used for daily cutover calculations.
// Falls back to UTC if the configured timezone cannot be loaded.
func cutoverLocation() *time.Location {
	loc, err := time.LoadLocation(CutoverTimezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

// MemoryStore manages persistent memory for the agent.
// - Long-term memory: memory/MEMORY.md
// - Daily notes: memory/YYYYMM/YYYYMMDD.md
type MemoryStore struct {
	workspace  string
	memoryDir  string
	memoryFile string
}

// NewMemoryStore creates a new MemoryStore with the given workspace path.
// It ensures the memory directory exists.
func NewMemoryStore(workspace string) *MemoryStore {
	memoryDir := filepath.Join(workspace, "memory")
	memoryFile := filepath.Join(memoryDir, "MEMORY.md")

	// Ensure memory directory exists
	os.MkdirAll(memoryDir, 0755)

	return &MemoryStore{
		workspace:  workspace,
		memoryDir:  memoryDir,
		memoryFile: memoryFile,
	}
}

// getTodayFile returns the path to today's daily note file (memory/YYYYMM/YYYYMMDD.md).
func (ms *MemoryStore) getTodayFile() string {
	today := time.Now().Format("20060102") // YYYYMMDD
	monthDir := today[:6]                  // YYYYMM
	filePath := filepath.Join(ms.memoryDir, monthDir, today+".md")
	return filePath
}

// ReadLongTerm reads the long-term memory (MEMORY.md).
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadLongTerm() string {
	if data, err := os.ReadFile(ms.memoryFile); err == nil {
		return string(data)
	}
	return ""
}

// WriteLongTerm writes content to the long-term memory file (MEMORY.md).
func (ms *MemoryStore) WriteLongTerm(content string) error {
	return os.WriteFile(ms.memoryFile, []byte(content), 0644)
}

// ReadToday reads today's daily note.
// Returns empty string if the file doesn't exist.
func (ms *MemoryStore) ReadToday() string {
	todayFile := ms.getTodayFile()
	if data, err := os.ReadFile(todayFile); err == nil {
		return string(data)
	}
	return ""
}

// AppendToday appends content to today's daily note.
// If the file doesn't exist, it creates a new file with a date header.
func (ms *MemoryStore) AppendToday(content string) error {
	todayFile := ms.getTodayFile()

	// Ensure month directory exists
	monthDir := filepath.Dir(todayFile)
	os.MkdirAll(monthDir, 0755)

	var existingContent string
	if data, err := os.ReadFile(todayFile); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		// Add header for new day
		header := fmt.Sprintf("# %s\n\n", time.Now().Format("2006-01-02"))
		newContent = header + content
	} else {
		// Append to existing content
		newContent = existingContent + "\n" + content
	}

	return os.WriteFile(todayFile, []byte(newContent), 0644)
}

// GetRecentDailyNotes returns daily notes from the last N days.
// Contents are joined with "---" separator.
func (ms *MemoryStore) GetRecentDailyNotes(days int) string {
	var notes []string

	for i := 0; i < days; i++ {
		date := time.Now().AddDate(0, 0, -i)
		dateStr := date.Format("20060102") // YYYYMMDD
		monthDir := dateStr[:6]            // YYYYMM
		filePath := filepath.Join(ms.memoryDir, monthDir, dateStr+".md")

		if data, err := os.ReadFile(filePath); err == nil {
			notes = append(notes, string(data))
		}
	}

	if len(notes) == 0 {
		return ""
	}

	// Join with separator
	var result string
	for i, note := range notes {
		if i > 0 {
			result += "\n\n---\n\n"
		}
		result += note
	}
	return result
}

// GetCutoverBoundary returns the most recent daily cutover boundary.
// The cutover boundary is CutoverHour (04:00 JST) of today or yesterday,
// whichever is the most recent past time. All calculations use CutoverTimezone.
func GetCutoverBoundary(now time.Time) time.Time {
	loc := cutoverLocation()
	nowLocal := now.In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), CutoverHour, 0, 0, 0, loc)
	if nowLocal.Before(today) {
		return today.AddDate(0, 0, -1)
	}
	return today
}

// GetLogicalDate returns the "logical date" for a given time, accounting
// for the cutover hour. Activity before CutoverHour (JST) belongs to the
// previous calendar day. All calculations use CutoverTimezone.
func GetLogicalDate(t time.Time) time.Time {
	loc := cutoverLocation()
	tLocal := t.In(loc)
	if tLocal.Hour() < CutoverHour {
		tLocal = tLocal.AddDate(0, 0, -1)
	}
	return time.Date(tLocal.Year(), tLocal.Month(), tLocal.Day(), 0, 0, 0, 0, loc)
}

// SaveDailyNoteForDate writes content to the daily note for a specific date.
func (ms *MemoryStore) SaveDailyNoteForDate(date time.Time, content string) error {
	dateStr := date.Format("20060102")
	monthDir := dateStr[:6]
	dirPath := filepath.Join(ms.memoryDir, monthDir)
	os.MkdirAll(dirPath, 0755)

	filePath := filepath.Join(dirPath, dateStr+".md")

	var existingContent string
	if data, err := os.ReadFile(filePath); err == nil {
		existingContent = string(data)
	}

	var newContent string
	if existingContent == "" {
		header := fmt.Sprintf("# %s\n\n", date.Format("2006-01-02"))
		newContent = header + content
	} else {
		newContent = existingContent + "\n\n" + content
	}

	return os.WriteFile(filePath, []byte(newContent), 0644)
}

// FormatCutoverNote builds a daily note from a session summary and recent messages.
func FormatCutoverNote(summary string, recentMessages []string) string {
	var parts []string

	if summary != "" {
		parts = append(parts, "## Session Summary\n\n"+summary)
	}

	if len(recentMessages) > 0 {
		parts = append(parts, "## Last Messages\n\n"+strings.Join(recentMessages, "\n"))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// GetMemoryContext returns formatted memory context for the agent prompt.
// Includes long-term memory and recent daily notes.
func (ms *MemoryStore) GetMemoryContext() string {
	var parts []string

	// Long-term memory
	longTerm := ms.ReadLongTerm()
	if longTerm != "" {
		parts = append(parts, "## Long-term Memory\n\n"+longTerm)
	}

	// Recent daily notes (last 3 days)
	recentNotes := ms.GetRecentDailyNotes(3)
	if recentNotes != "" {
		parts = append(parts, "## Recent Daily Notes\n\n"+recentNotes)
	}

	if len(parts) == 0 {
		return ""
	}

	// Join parts with separator
	var result string
	for i, part := range parts {
		if i > 0 {
			result += "\n\n---\n\n"
		}
		result += part
	}
	return fmt.Sprintf("# Memory\n\n%s", result)
}
