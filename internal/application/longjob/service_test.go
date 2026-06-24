package longjob

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	domain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/longjob"
)

func TestService_StartResumeCompleteStockLearn(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	svc := NewService(NewFileStore(dir), func() time.Time { return now })

	job, err := svc.StartStockLearn(context.Background(), StockLearnRequest{
		Universe:  "jp-liquid",
		Period:    "3y",
		Objective: "paper-research",
	})
	if err != nil {
		t.Fatalf("StartStockLearn failed: %v", err)
	}
	if job.Kind != "stock-learn" || job.Status != domain.StatusPending {
		t.Fatalf("unexpected job: %+v", job)
	}
	if job.Params["universe"] != "jp-liquid" || len(job.Plan) == 0 {
		t.Fatalf("unexpected job params/plan: %+v", job)
	}

	res, err := svc.Resume(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if res.Step.ID != "define-scope" || res.Job.Status != domain.StatusRunning {
		t.Fatalf("unexpected resume result: %+v", res)
	}
	if !strings.Contains(res.Prompt, "RenCrow Long Running Job Resume") || !strings.Contains(res.Prompt, "jp-liquid") {
		t.Fatalf("resume prompt missing expected content: %s", res.Prompt)
	}
	if _, err := os.Stat(res.ArtifactPath); err != nil {
		t.Fatalf("resume artifact not written: %v", err)
	}

	updated, err := svc.CompleteStep(context.Background(), job.ID, "define-scope", "評価条件を固定した", "")
	if err != nil {
		t.Fatalf("CompleteStep failed: %v", err)
	}
	if updated.Plan[0].Status != domain.StepCompleted || updated.ResumePoint != "snapshot-data" {
		t.Fatalf("unexpected completed job: %+v", updated)
	}
}

func TestService_CancelPersistsReason(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(NewFileStore(dir), func() time.Time {
		return time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC)
	})
	job, err := svc.StartStockLearn(context.Background(), StockLearnRequest{})
	if err != nil {
		t.Fatalf("StartStockLearn failed: %v", err)
	}
	canceled, err := svc.Cancel(context.Background(), job.ID, "stop requested")
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if canceled.Status != domain.StatusCanceled {
		t.Fatalf("expected canceled, got %s", canceled.Status)
	}
	loaded, err := svc.Load(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Status != domain.StatusCanceled || !strings.Contains(loaded.SharedContext[len(loaded.SharedContext)-1].Content, "stop requested") {
		t.Fatalf("cancel was not persisted: %+v", loaded)
	}
}
