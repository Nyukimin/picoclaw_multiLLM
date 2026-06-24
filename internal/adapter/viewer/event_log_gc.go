package viewer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

type EventLogGCService struct {
	store      *EventLogStore
	reportPath string
	retention  time.Duration
	interval   time.Duration
	stopCh     chan struct{}
	doneCh     chan struct{}
	startOnce  sync.Once
	stopOnce   sync.Once
}

type EventGCReport struct {
	StartedAt           string `json:"started_at"`
	FinishedAt          string `json:"finished_at"`
	SourcePath          string `json:"source_path"`
	RetentionDays       int    `json:"retention_days"`
	BeforeCount         int    `json:"before_count"`
	AfterCount          int    `json:"after_count"`
	DeletedCount        int    `json:"deleted_count"`
	DecodeErrorCount    int    `json:"decode_error_count,omitempty"`
	TimestampErrorCount int    `json:"timestamp_error_count,omitempty"`
	Status              string `json:"status"`
	Error               string `json:"error,omitempty"`
}

func NewEventLogGCService(store *EventLogStore, reportPath string, retentionDays, intervalMinutes int) (*EventLogGCService, error) {
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if retentionDays < 1 {
		return nil, fmt.Errorf("retentionDays must be >= 1")
	}
	if intervalMinutes < 1 {
		return nil, fmt.Errorf("intervalMinutes must be >= 1")
	}
	dir := filepath.Dir(reportPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create gc report dir: %w", err)
	}
	f, err := os.OpenFile(reportPath, os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("touch gc report file: %w", err)
	}
	_ = f.Close()
	return &EventLogGCService{
		store:      store,
		reportPath: reportPath,
		retention:  time.Duration(retentionDays) * 24 * time.Hour,
		interval:   time.Duration(intervalMinutes) * time.Minute,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}, nil
}

func (s *EventLogGCService) Start() {
	s.startOnce.Do(func() {
		go func() {
			defer close(s.doneCh)
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					_, _ = s.RunOnce(context.Background(), time.Now().UTC())
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *EventLogGCService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	<-s.doneCh
}

func (s *EventLogGCService) RunOnce(_ context.Context, now time.Time) (EventGCReport, error) {
	s.store.mu.Lock()
	defer s.store.mu.Unlock()

	startedAt := now.UTC().Format(time.RFC3339)
	report := EventGCReport{
		StartedAt:     startedAt,
		SourcePath:    s.store.path,
		RetentionDays: int(s.retention / (24 * time.Hour)),
		Status:        "ok",
	}

	src, err := os.Open(s.store.path)
	if err != nil {
		report.Status = "error"
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = appendGCReport(s.reportPath, report)
		return report, fmt.Errorf("open source: %w", err)
	}
	defer src.Close()

	tmpPath := s.store.path + ".tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		report.Status = "error"
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = appendGCReport(s.reportPath, report)
		return report, fmt.Errorf("create temp: %w", err)
	}

	cutoff := now.Add(-s.retention)
	sc := bufio.NewScanner(src)
	enc := json.NewEncoder(tmp)
	for sc.Scan() {
		report.BeforeCount++
		var ev orchestrator.OrchestratorEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			report.DecodeErrorCount++
			continue
		}
		ts, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil {
			report.TimestampErrorCount++
			continue
		}
		if ts.Before(cutoff) {
			report.DeletedCount++
			continue
		}
		if err := enc.Encode(ev); err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			report.Status = "error"
			report.Error = err.Error()
			report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
			_ = appendGCReport(s.reportPath, report)
			return report, fmt.Errorf("encode temp: %w", err)
		}
		report.AfterCount++
	}
	if err := sc.Err(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		report.Status = "error"
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = appendGCReport(s.reportPath, report)
		return report, fmt.Errorf("scan source: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		report.Status = "error"
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = appendGCReport(s.reportPath, report)
		return report, fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.store.path); err != nil {
		_ = os.Remove(tmpPath)
		report.Status = "error"
		report.Error = err.Error()
		report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		_ = appendGCReport(s.reportPath, report)
		return report, fmt.Errorf("rename temp: %w", err)
	}

	if report.DecodeErrorCount > 0 || report.TimestampErrorCount > 0 {
		report.Status = "partial_error"
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	if err := appendGCReport(s.reportPath, report); err != nil {
		return report, err
	}
	return report, nil
}

func appendGCReport(path string, report EventGCReport) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open gc report: %w", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(report); err != nil {
		return fmt.Errorf("encode gc report: %w", err)
	}
	return nil
}
