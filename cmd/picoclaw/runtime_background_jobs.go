package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	archiveapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/archive"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/sourcefetcher"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

func startSourceRegistrySweeper(store *conversationpersistence.L1SQLiteStore) {
	sweep := func() {
		result, err := sourcefetcher.SweepDueSources(context.Background(), store, time.Now().UTC(), sourcefetcher.SweepOptions{
			LimitPerSource:    10,
			MinimumTrustScore: 0.5,
		})
		if err != nil {
			log.Printf("WARN: source registry sweep failed: %v", err)
			return
		}
		if result.Sources > 0 || result.Staged > 0 || result.Failed > 0 {
			log.Printf("Source registry sweep complete: sources=%d staged=%d validated=%d promoted_news=%d failed=%d",
				result.Sources, result.Staged, result.Validated, result.PromotedNews, result.Failed)
		}
	}
	go func() {
		sweep()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()
}

func startParquetExportJob(store archiveapp.ParquetExportStore) {
	outputDir := strings.TrimSpace(os.Getenv("RENCROW_PARQUET_EXPORT_DIR"))
	if outputDir == "" {
		return
	}
	interval := 24 * time.Hour
	if raw := strings.TrimSpace(os.Getenv("RENCROW_PARQUET_EXPORT_INTERVAL_SEC")); raw != "" {
		sec, err := strconv.Atoi(raw)
		if err != nil || sec <= 0 {
			log.Printf("WARN: invalid RENCROW_PARQUET_EXPORT_INTERVAL_SEC=%q", raw)
			return
		}
		interval = time.Duration(sec) * time.Second
	}
	job := archiveapp.NewParquetExportJob(store, archiveapp.ParquetExportOptions{
		OutputDir: outputDir,
		Interval:  interval,
	})
	go func() {
		result, err := job.RunOnce(context.Background())
		if err != nil {
			log.Printf("WARN: parquet export failed: %v", err)
		} else {
			log.Printf("Parquet export complete: thread=%s l1_archives=%d", result.ThreadSummariesPath, len(result.L1ArchivePaths))
		}
		for result := range job.Start(context.Background()) {
			if result.Error != nil {
				log.Printf("WARN: parquet export failed: %v", result.Error)
				continue
			}
			log.Printf("Parquet export complete: thread=%s l1_archives=%d", result.ThreadSummariesPath, len(result.L1ArchivePaths))
		}
	}()
	log.Printf("Parquet export job enabled: dir=%s interval=%s", outputDir, interval)
}
