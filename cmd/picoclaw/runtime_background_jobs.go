package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	archiveapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/archive"
	knowledgememoryapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/knowledgememory"
	moviecatalogapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/moviecatalog"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/sourcefetcher"
	superagentapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/superagent"
	domainsuperagent "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/superagent"
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

func startMemoryLifecycleJob(store *conversationpersistence.L1SQLiteStore) {
	if store == nil {
		return
	}
	run := func() {
		result, err := store.RunMemoryLifecycleMaintenance(context.Background(), conversationpersistence.DefaultMemoryLifecycleOptions())
		if err != nil {
			log.Printf("WARN: memory lifecycle maintenance failed: %v", err)
			return
		}
		if result.RawCompacted > 0 || result.CandidatesQueued > 0 || result.MonthlyHighlightsBuilt > 0 || result.ThreadSummarySeedsQueued > 0 || result.Decayed > 0 || result.VectorCleanupQueued > 0 || result.VectorCleanupExecuted > 0 {
			log.Printf("Memory lifecycle maintenance complete: raw_compacted=%d candidates_queued=%d monthly_highlights_built=%d thread_summary_seeds_queued=%d decayed=%d vector_cleanup_queued=%d vector_cleanup_executed=%d",
				result.RawCompacted, result.CandidatesQueued, result.MonthlyHighlightsBuilt, result.ThreadSummarySeedsQueued, result.Decayed, result.VectorCleanupQueued, result.VectorCleanupExecuted)
		}
	}
	go func() {
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}

func startDailyIntakeSweeper(rules knowledgememoryapp.DailyIntakeRuleStore, registry knowledgememoryapp.DailyIntakeRegistryStore) {
	if rules == nil || registry == nil {
		return
	}
	sweep := func() {
		result, err := knowledgememoryapp.RunDailyIntakeSweep(context.Background(), rules, registry, knowledgememoryapp.DailyIntakeSweepOptions{
			RuleLimit:         100,
			SourceLimit:       10,
			MinimumTrustScore: 0.5,
		})
		if err != nil {
			log.Printf("WARN: daily intake sweep failed: %v", err)
			return
		}
		if result.SourcesEnabled > 0 || result.RegistrySweep.Staged > 0 || result.RegistrySweep.Failed > 0 {
			log.Printf("Daily intake sweep complete: rules=%d enabled=%d skipped=%d staged=%d promoted_knowledge=%d failed=%d",
				result.RulesScanned, result.SourcesEnabled, result.SourcesSkipped, result.RegistrySweep.Staged, result.RegistrySweep.PromotedKnowledge, result.RegistrySweep.Failed)
		}
	}
	go func() {
		sweep()
		ticker := time.NewTicker(24 * time.Hour)
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

func startMovieCatalogBackfillJob(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if movieCatalogBackfillDisabled() {
		log.Printf("[MovieCatalogBackfill] disabled by environment")
		return
	}
	dbPath := resolveMovieCatalogBackfillDBPath()
	if dbPath == "" {
		log.Printf("[MovieCatalogBackfill] skipped: movie catalog DB not found")
		return
	}
	interval := movieCatalogBackfillDurationEnv("PICOCLAW_MOVIE_CATALOG_BACKFILL_INTERVAL_SEC", 5*time.Minute, time.Minute)
	initialDelay := movieCatalogBackfillDurationEnv("PICOCLAW_MOVIE_CATALOG_BACKFILL_INITIAL_DELAY_SEC", 10*time.Second, 0)
	timeout := movieCatalogBackfillDurationEnv("PICOCLAW_MOVIE_CATALOG_BACKFILL_TIMEOUT_SEC", 90*time.Second, 10*time.Second)
	maxPages := movieCatalogBackfillIntEnv("PICOCLAW_MOVIE_CATALOG_BACKFILL_MAX_PAGES", 1, 1, 3)
	crawlerDelay := movieCatalogBackfillDurationEnv("PICOCLAW_MOVIE_CATALOG_BACKFILL_CRAWLER_DELAY_SEC", 2*time.Second, time.Second)

	job := moviecatalogapp.NewBackfillService(moviecatalogapp.BackfillOptions{
		DBPath:       dbPath,
		WorkspaceDir: ".",
		Interval:     interval,
		InitialDelay: initialDelay,
		Timeout:      timeout,
		MaxPages:     maxPages,
		CrawlerDelay: crawlerDelay,
	})
	go func() {
		for result := range job.Start(context.Background()) {
			if result.Status == "idle" {
				continue
			}
			moviecatalogapp.LogBackfillResult("[MovieCatalogBackfill]", result)
		}
	}()
	log.Printf("[MovieCatalogBackfill] enabled: db=%s interval=%s initial_delay=%s timeout=%s max_pages=%d crawler_delay=%s",
		dbPath, interval, initialDelay, timeout, maxPages, crawlerDelay)
}

func movieCatalogBackfillDisabled() bool {
	disabled := strings.ToLower(strings.TrimSpace(os.Getenv("PICOCLAW_MOVIE_CATALOG_BACKFILL_DISABLED")))
	switch disabled {
	case "1", "true", "yes", "on":
		return true
	}
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("PICOCLAW_MOVIE_CATALOG_BACKFILL")))
	switch enabled {
	case "0", "false", "no", "off", "disabled":
		return true
	}
	return false
}

func resolveMovieCatalogBackfillDBPath() string {
	candidates := []string{}
	if env := strings.TrimSpace(os.Getenv("PICOCLAW_MOVIE_CATALOG_DB")); env != "" {
		candidates = append(candidates, env)
	}
	candidates = append(candidates,
		filepath.Join("tmp", "eiga_catalog", "eiga_catalog.sqlite"),
		filepath.Join("tmp", "eiga_catalog_smoke", "eiga_catalog.sqlite"),
	)
	for _, p := range candidates {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func movieCatalogBackfillDurationEnv(name string, fallback time.Duration, min time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 0 {
		log.Printf("WARN: invalid %s=%q; using %s", name, raw, fallback)
		return fallback
	}
	d := time.Duration(sec) * time.Second
	if d < min {
		log.Printf("WARN: %s=%s is too small; using minimum %s", name, d, min)
		return min
	}
	return d
}

func movieCatalogBackfillIntEnv(name string, fallback int, min int, max int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("WARN: invalid %s=%q; using %d", name, raw, fallback)
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

type superAgentRunQueueMessageProcessor interface {
	ProcessMessage(context.Context, orchestrator.ProcessMessageRequest) (orchestrator.ProcessMessageResponse, error)
}

func startSuperAgentRunQueueScheduler(cfg *config.Config, store superagentapp.RunQueueStore, processor superAgentRunQueueMessageProcessor) {
	if cfg == nil || !cfg.SuperAgentHarness.RunQueueSchedulerEnabled {
		return
	}
	if store == nil || processor == nil {
		log.Printf("WARN: superagent run queue scheduler requested but store or processor is unavailable")
		return
	}
	interval := time.Duration(cfg.SuperAgentHarness.RunQueueSchedulerIntervalSec) * time.Second
	claimLimit := cfg.SuperAgentHarness.RunQueueSchedulerClaimLimit
	scheduler := superagentapp.NewRunQueueScheduler(store, newSuperAgentRunQueueProcessor(processor), superagentapp.RunQueueSchedulerOptions{
		Interval:   interval,
		ClaimLimit: claimLimit,
	})
	scheduler.Start(context.Background())
	log.Printf("SuperAgent run queue scheduler enabled: interval=%s claim_limit=%d", interval, claimLimit)
}

func newSuperAgentRunQueueProcessor(processor superAgentRunQueueMessageProcessor) superagentapp.RunQueueProcessorFunc {
	return superagentapp.RunQueueProcessorFunc(func(ctx context.Context, item domainsuperagent.RunQueueItem) (string, error) {
		action := strings.TrimSpace(item.Action)
		if action != "resume" && action != "process_message" && action != "chat" {
			return "", fmt.Errorf("unsupported run queue action: %s", action)
		}
		sessionID := strings.TrimSpace(item.WorkstreamID)
		if sessionID == "" {
			sessionID = strings.TrimSpace(item.RunID)
		}
		if sessionID == "" {
			sessionID = "superagent:" + strings.TrimSpace(item.QueueID)
		}
		resp, err := processor.ProcessMessage(ctx, orchestrator.ProcessMessageRequest{
			SessionID:   sessionID,
			Channel:     "superagent",
			ChatID:      strings.TrimSpace(item.QueueID),
			UserMessage: strings.TrimSpace(item.Goal),
		})
		if err != nil {
			return "", err
		}
		if resp.Route == "" {
			return "", fmt.Errorf("run queue item did not produce a route")
		}
		if action != "chat" && resp.Route == "CHAT" {
			return "", fmt.Errorf("run queue item fell back to CHAT route")
		}
		if strings.TrimSpace(resp.JobID) == "" {
			return "", fmt.Errorf("run queue item did not produce a job_id")
		}
		return fmt.Sprintf("route=%s job_id=%s", resp.Route, resp.JobID), nil
	})
}
