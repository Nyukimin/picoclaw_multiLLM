package service

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// WorkerExecutionService はPatch実行サービスのインターフェース
type WorkerExecutionService interface {
	ExecuteProposal(ctx context.Context, jobID task.JobID, p *proposal.Proposal) (*patch.PatchExecutionResult, error)
	ExecuteObservation(ctx context.Context, actions []ObservationAction) ([]ObservationActionResult, error)
}

// workerExecutionService はWorkerExecutionServiceの実装
type workerExecutionService struct {
	config config.WorkerConfig
}

// NewWorkerExecutionService は新しいWorkerExecutionServiceを作成
func NewWorkerExecutionService(cfg config.WorkerConfig) WorkerExecutionService {
	return &workerExecutionService{
		config: cfg,
	}
}

// ExecuteProposal はProposalのPatchを解析・実行する
func (w *workerExecutionService) ExecuteProposal(
	ctx context.Context,
	jobID task.JobID,
	p *proposal.Proposal,
) (*patch.PatchExecutionResult, error) {
	commands, err := w.parseProposalCommands(p)
	if err != nil {
		return nil, err
	}

	w.showExecutionSummaryIfEnabled(jobID, commands)
	if err := w.autoCommitBeforeExecution(ctx, jobID); err != nil {
		return nil, err
	}

	result := w.executeCommands(ctx, jobID, commands)
	w.autoCommitAfterExecution(ctx, jobID, result)
	return w.finalizeExecutionResult(commands, result), nil
}
