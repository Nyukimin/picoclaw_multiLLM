package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	archiveapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/archive"
	knowledgememoryapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/knowledgememory"
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
	scheduler := superagentapp.NewRunQueueScheduler(store, superagentapp.RunQueueProcessorFunc(func(ctx context.Context, item domainsuperagent.RunQueueItem) (string, error) {
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
		return fmt.Sprintf("route=%s job_id=%s", resp.Route, resp.JobID), nil
	}), superagentapp.RunQueueSchedulerOptions{
		Interval:   interval,
		ClaimLimit: claimLimit,
	})
	scheduler.Start(context.Background())
	log.Printf("SuperAgent run queue scheduler enabled: interval=%s claim_limit=%d", interval, claimLimit)
}
