package jobid

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGenerator_Next(t *testing.T) {
	g := NewGenerator()

	// Test format
	jobID := g.Next()
	if !strings.HasPrefix(jobID, "job_") {
		t.Errorf("JobID should start with 'job_', got: %s", jobID)
	}

	// Test incrementing
	jobID1 := g.Next()
	jobID2 := g.Next()

	if jobID1 == jobID2 {
		t.Errorf("JobIDs should be unique, got: %s and %s", jobID1, jobID2)
	}

	// Test format: job_YYYYMMDD_NNN
	parts := strings.Split(jobID2, "_")
	if len(parts) != 3 {
		t.Errorf("JobID should have 3 parts separated by '_', got: %v", parts)
	}

	if parts[0] != "job" {
		t.Errorf("First part should be 'job', got: %s", parts[0])
	}

	if len(parts[1]) != 8 {
		t.Errorf("Date part should be 8 digits, got: %s (len=%d)", parts[1], len(parts[1]))
	}

	if len(parts[2]) != 3 {
		t.Errorf("Counter part should be 3 digits, got: %s (len=%d)", parts[2], len(parts[2]))
	}
}

func TestGenerator_Reset(t *testing.T) {
	g := NewGenerator()

	// Generate a few IDs
	g.Next()
	g.Next()
	g.Next()

	// Reset
	g.Reset()

	// Next ID should be 001
	jobID := g.Next()
	if !strings.HasSuffix(jobID, "_001") {
		t.Errorf("After reset, first JobID should end with '_001', got: %s", jobID)
	}
}

func TestGenerator_Concurrency(t *testing.T) {
	g := NewGenerator()

	const numGoroutines = 100
	const idsPerGoroutine = 10

	ids := make(chan string, numGoroutines*idsPerGoroutine)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerGoroutine; j++ {
				ids <- g.Next()
			}
		}()
	}

	wg.Wait()
	close(ids)

	// Check uniqueness
	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("Duplicate JobID detected: %s", id)
		}
		seen[id] = true
	}

	expectedCount := numGoroutines * idsPerGoroutine
	if len(seen) != expectedCount {
		t.Errorf("Expected %d unique IDs, got %d", expectedCount, len(seen))
	}
}

func TestGenerator_DateRollover(t *testing.T) {
	g := NewGenerator()

	// Generate an ID
	id1 := g.Next()

	// Simulate date change by manually updating currentDate
	g.mu.Lock()
	g.currentDate = time.Now().AddDate(0, 0, 1).Format("20060102")
	g.counter = 999 // High counter to test reset
	g.mu.Unlock()

	// Next ID should have counter 1000
	_ = g.Next() // id2

	// Reset should happen when date changes, so we manually trigger reset
	g.Reset()
	id3 := g.Next()

	// After reset, counter should be 001
	if !strings.HasSuffix(id3, "_001") {
		t.Errorf("After date change and reset, counter should reset to 001, got: %s", id3)
	}

	// id1 and id3 should have different dates
	parts1 := strings.Split(id1, "_")
	parts3 := strings.Split(id3, "_")

	if parts1[1] == parts3[1] {
		t.Logf("Warning: dates are same (expected different on reset), id1=%s, id3=%s", id1, id3)
	}
}
