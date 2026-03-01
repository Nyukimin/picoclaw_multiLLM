// Package jobid provides JobID generation for tracking work units across the system.
package jobid

import (
	"fmt"
	"sync"
	"time"
)

// Generator generates unique JobIDs for tracking work units.
// JobIDs follow the format: job_YYYYMMDD_NNN
// where NNN is a zero-padded 3-digit counter that resets daily.
type Generator struct {
	mu          sync.Mutex
	counter     int
	currentDate string
}

// NewGenerator creates a new JobID generator.
func NewGenerator() *Generator {
	return &Generator{
		counter:     0,
		currentDate: time.Now().Format("20060102"),
	}
}

// Next generates the next JobID.
// Format: job_YYYYMMDD_NNN
// Example: job_20260301_001
func (g *Generator) Next() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now()
	today := now.Format("20060102")

	// Reset counter if date changed
	if today != g.currentDate {
		g.currentDate = today
		g.counter = 0
	}

	g.counter++

	// Format: job_YYYYMMDD_NNN
	return fmt.Sprintf("job_%s_%03d", g.currentDate, g.counter)
}

// Reset resets the generator's counter (primarily for testing).
func (g *Generator) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.counter = 0
	g.currentDate = time.Now().Format("20060102")
}
