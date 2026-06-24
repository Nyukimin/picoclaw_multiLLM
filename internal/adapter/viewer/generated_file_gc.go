package viewer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type GeneratedFileGCService struct {
	dir       string
	pattern   string
	retention time.Duration
	interval  time.Duration
	stopCh    chan struct{}
	doneCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

type GeneratedFileGCReport struct {
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at"`
	Dir          string `json:"dir"`
	Pattern      string `json:"pattern"`
	BeforeCount  int    `json:"before_count"`
	DeletedCount int    `json:"deleted_count"`
	ErrorCount   int    `json:"error_count,omitempty"`
}

func NewGeneratedFileGCService(dir, pattern string, retention, interval time.Duration) (*GeneratedFileGCService, error) {
	if dir == "" {
		return nil, fmt.Errorf("dir is required")
	}
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	if retention <= 0 {
		return nil, fmt.Errorf("retention must be positive")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("interval must be positive")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create generated file gc dir: %w", err)
	}
	return &GeneratedFileGCService{
		dir:       dir,
		pattern:   pattern,
		retention: retention,
		interval:  interval,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}, nil
}

func (s *GeneratedFileGCService) Start() {
	s.startOnce.Do(func() {
		go func() {
			defer close(s.doneCh)
			_, _ = s.RunOnce(context.Background(), time.Now())
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_, _ = s.RunOnce(context.Background(), time.Now())
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *GeneratedFileGCService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	<-s.doneCh
}

func (s *GeneratedFileGCService) RunOnce(_ context.Context, now time.Time) (report GeneratedFileGCReport, err error) {
	report = GeneratedFileGCReport{
		StartedAt: now.UTC().Format(time.RFC3339),
		Dir:       s.dir,
		Pattern:   s.pattern,
	}
	defer func() {
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	}()

	matches, err := filepath.Glob(filepath.Join(s.dir, s.pattern))
	if err != nil {
		return report, fmt.Errorf("glob generated files: %w", err)
	}
	cutoff := now.Add(-s.retention)
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			report.ErrorCount++
			continue
		}
		if info.IsDir() {
			continue
		}
		report.BeforeCount++
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil {
			report.ErrorCount++
			continue
		}
		report.DeletedCount++
	}
	if report.ErrorCount > 0 {
		return report, fmt.Errorf("generated file gc completed with %d errors", report.ErrorCount)
	}
	return report, nil
}
