package longjob

import (
	"context"
	"fmt"
	"strings"
	"time"

	domain "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/longjob"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

type Service struct {
	store *FileStore
	now   func() time.Time
}

type StockLearnRequest struct {
	Universe  string
	Period    string
	Objective string
	Goal      string
}

type ResumeResult struct {
	Job          domain.Job  `json:"job"`
	Step         domain.Step `json:"step"`
	ArtifactPath string      `json:"artifact_path"`
	Prompt       string      `json:"prompt"`
}

func NewService(store *FileStore, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, now: now}
}

func (s *Service) StartStockLearn(ctx context.Context, req StockLearnRequest) (domain.Job, error) {
	now := s.now()
	universe := defaultString(req.Universe, "us-liquid")
	period := defaultString(req.Period, "5y")
	objective := defaultString(req.Objective, "research")
	goal := strings.TrimSpace(req.Goal)
	if goal == "" {
		goal = fmt.Sprintf("株価推移学習: universe=%s period=%s objective=%s", universe, period, objective)
	}
	job := domain.Job{
		ID:        "lj-" + task.NewJobID().String(),
		Kind:      "stock-learn",
		Goal:      goal,
		Status:    domain.StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
		Params: map[string]string{
			"universe":  universe,
			"period":    period,
			"objective": objective,
		},
		Plan: stockLearnPlan(),
		SharedContext: []domain.ContextEntry{
			{
				Role:      "system",
				Content:   "長時間ジョブはデータsnapshot、実行条件、成果物、Heavyレビュー、再開地点を保存しながら進める。実取引判断ではなく研究・検証・紙運用境界を守る。",
				CreatedAt: now,
			},
		},
		ReviewGates: []domain.ReviewGate{
			{ID: "heavy-final-review", Reviewer: "heavy", Status: domain.StepPending, CreatedAt: now},
		},
	}
	if err := s.store.Save(ctx, job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (s *Service) Load(ctx context.Context, id string) (domain.Job, error) {
	return s.store.Load(ctx, id)
}

func (s *Service) List(ctx context.Context) ([]domain.Job, error) {
	return s.store.List(ctx)
}

func (s *Service) Resume(ctx context.Context, id string) (ResumeResult, error) {
	job, err := s.store.Load(ctx, id)
	if err != nil {
		return ResumeResult{}, err
	}
	if job.IsTerminal() {
		return ResumeResult{}, fmt.Errorf("job %s is %s", job.ID, job.Status)
	}
	idx := job.NextStepIndex()
	if idx < 0 {
		now := s.now()
		job.Status = domain.StatusCompleted
		job.UpdatedAt = now
		job.FinishedAt = &now
		if err := s.store.Save(ctx, job); err != nil {
			return ResumeResult{}, err
		}
		return ResumeResult{}, fmt.Errorf("job %s has no remaining steps", job.ID)
	}

	now := s.now()
	if job.Status == domain.StatusPending {
		job.StartedAt = &now
	}
	job.Status = domain.StatusRunning
	job.UpdatedAt = now
	job.ResumePoint = job.Plan[idx].ID
	if job.Plan[idx].Status == domain.StepPending {
		job.Plan[idx].Status = domain.StepRunning
		job.Plan[idx].StartedAt = &now
	}
	prompt := BuildResumePrompt(job, job.Plan[idx])
	path, err := s.store.WriteArtifact(job.ID, "resume_"+job.Plan[idx].ID+".md", []byte(prompt))
	if err != nil {
		return ResumeResult{}, err
	}
	artifact := domain.Artifact{
		ID:        "resume-" + job.Plan[idx].ID,
		Kind:      "resume_prompt",
		Path:      path,
		Summary:   "Worker/Heavy/Wildが同じ前提で再開するためのプロンプト",
		CreatedAt: now,
	}
	job.Artifacts = append(job.Artifacts, artifact)
	job.Plan[idx].ArtifactIDs = append(job.Plan[idx].ArtifactIDs, artifact.ID)
	if err := s.store.Save(ctx, job); err != nil {
		return ResumeResult{}, err
	}
	return ResumeResult{Job: job, Step: job.Plan[idx], ArtifactPath: path, Prompt: prompt}, nil
}

func (s *Service) CompleteStep(ctx context.Context, id, stepID, summary, artifactPath string) (domain.Job, error) {
	job, err := s.store.Load(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	if job.IsTerminal() {
		return domain.Job{}, fmt.Errorf("job %s is %s", job.ID, job.Status)
	}
	idx := findStepIndex(job, stepID)
	if idx < 0 {
		return domain.Job{}, fmt.Errorf("step %s not found", stepID)
	}
	now := s.now()
	job.Plan[idx].Status = domain.StepCompleted
	job.Plan[idx].Summary = strings.TrimSpace(summary)
	job.Plan[idx].FinishedAt = &now
	if strings.TrimSpace(artifactPath) != "" {
		artifact := domain.Artifact{
			ID:        fmt.Sprintf("artifact-%s-%d", job.Plan[idx].ID, len(job.Artifacts)+1),
			Kind:      "external",
			Path:      strings.TrimSpace(artifactPath),
			Summary:   "step output",
			CreatedAt: now,
		}
		job.Artifacts = append(job.Artifacts, artifact)
		job.Plan[idx].ArtifactIDs = append(job.Plan[idx].ArtifactIDs, artifact.ID)
	}
	next := job.NextStepIndex()
	if next < 0 {
		job.Status = domain.StatusCompleted
		job.ResumePoint = ""
		job.FinishedAt = &now
		for i := range job.ReviewGates {
			if job.ReviewGates[i].Status == domain.StepPending {
				job.ReviewGates[i].Status = domain.StepCompleted
				job.ReviewGates[i].Summary = "all planned steps completed"
			}
		}
	} else {
		job.Status = domain.StatusPending
		job.ResumePoint = job.Plan[next].ID
	}
	job.UpdatedAt = now
	if err := s.store.Save(ctx, job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func (s *Service) Cancel(ctx context.Context, id, reason string) (domain.Job, error) {
	job, err := s.store.Load(ctx, id)
	if err != nil {
		return domain.Job{}, err
	}
	if job.Status == domain.StatusCompleted {
		return domain.Job{}, fmt.Errorf("completed job cannot be canceled")
	}
	now := s.now()
	job.Status = domain.StatusCanceled
	job.UpdatedAt = now
	job.FinishedAt = &now
	reason = strings.TrimSpace(reason)
	if reason != "" {
		job.SharedContext = append(job.SharedContext, domain.ContextEntry{Role: "user", Content: "cancel reason: " + reason, CreatedAt: now})
	}
	if err := s.store.Save(ctx, job); err != nil {
		return domain.Job{}, err
	}
	return job, nil
}

func BuildResumePrompt(job domain.Job, step domain.Step) string {
	var b strings.Builder
	b.WriteString("# RenCrow Long Running Job Resume\n\n")
	b.WriteString(fmt.Sprintf("job_id: %s\nkind: %s\nstatus: %s\nresume_point: %s\n\n", job.ID, job.Kind, job.Status, step.ID))
	b.WriteString("## Goal\n")
	b.WriteString(job.Goal + "\n\n")
	if len(job.Params) > 0 {
		b.WriteString("## Params\n")
		for k, v := range job.Params {
			b.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Shared Context\n")
	for _, entry := range job.SharedContext {
		b.WriteString(fmt.Sprintf("- %s: %s\n", entry.Role, strings.TrimSpace(entry.Content)))
	}
	b.WriteString("\n## Current Step\n")
	b.WriteString(fmt.Sprintf("- id: %s\n- role: %s\n- title: %s\n- description: %s\n\n", step.ID, step.Role, step.Title, step.Description))
	b.WriteString("## Operating Rules\n")
	b.WriteString("- Worker executes concrete commands and writes artifacts.\n")
	b.WriteString("- Heavy reviews assumptions, leakage, overfitting, and evaluation quality before claims are accepted.\n")
	b.WriteString("- Wild may suggest visualizations or pattern hypotheses, but confirmed facts must be written back as artifacts or summaries.\n")
	b.WriteString("- Do not imply live trading readiness. Keep research, paper-trading, and live boundaries explicit.\n")
	return b.String()
}

func stockLearnPlan() []domain.Step {
	return []domain.Step{
		{ID: "define-scope", Role: "heavy", Title: "仮説と評価条件を固定", Description: "対象 universe、期間、禁止事項、評価指標、リーク防止ルールを明文化する。", Status: domain.StepPending},
		{ID: "snapshot-data", Role: "worker", Title: "市場データ snapshot 作成", Description: "銘柄 universe と価格系列を取得し、取得日時とソースを成果物に残す。", Status: domain.StepPending},
		{ID: "build-features", Role: "worker", Title: "特徴量生成", Description: "リターン、ボラティリティ、出来高、トレンド、カレンダー特徴量を再現可能に生成する。", Status: domain.StepPending},
		{ID: "train-baseline", Role: "worker", Title: "ベースライン学習", Description: "単純な基準モデルから開始し、ウォークフォワード評価の入力を作る。", Status: domain.StepPending},
		{ID: "backtest", Role: "worker", Title: "バックテストと紙運用候補", Description: "手数料、スリッページ、取引頻度、最大DDを含めて検証する。", Status: domain.StepPending},
		{ID: "heavy-review", Role: "heavy", Title: "Heavy レビュー", Description: "過学習、データリーク、銘柄選定バイアス、評価指標の妥当性を確認する。", Status: domain.StepPending},
		{ID: "report", Role: "worker", Title: "レポート生成", Description: "結果、制約、次回実験、未解決リスクを Markdown/JSON で保存する。", Status: domain.StepPending},
	}
}

func findStepIndex(job domain.Job, stepID string) int {
	stepID = strings.TrimSpace(stepID)
	if stepID == "" {
		return job.NextStepIndex()
	}
	for i, step := range job.Plan {
		if step.ID == stepID {
			return i
		}
	}
	return -1
}

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
