package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// WorkerExecutionService はPatch実行サービスのインターフェース
type WorkerExecutionService interface {
	ExecuteProposal(ctx context.Context, jobID task.JobID, p *proposal.Proposal) (*patch.PatchExecutionResult, error)
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

func (w *workerExecutionService) parseProposalCommands(p *proposal.Proposal) ([]patch.PatchCommand, error) {
	commands, err := patch.ParsePatch(p.Patch())
	if err != nil {
		return nil, fmt.Errorf("patch parse error: %w", err)
	}
	return commands, nil
}

func (w *workerExecutionService) showExecutionSummaryIfEnabled(jobID task.JobID, commands []patch.PatchCommand) {
	if w.config.ShowExecutionSummary {
		w.showExecutionSummary(jobID, commands)
	}
}

func (w *workerExecutionService) autoCommitBeforeExecution(ctx context.Context, jobID task.JobID) error {
	if !w.config.AutoCommit {
		return nil
	}
	preCommitHash, err := w.autoCommitChanges(ctx, jobID, "Before patch execution")
	if err != nil {
		return fmt.Errorf("pre-execution auto-commit failed: %w", err)
	}
	fmt.Printf("[Worker] Pre-commit succeeded: %s\n", preCommitHash)
	return nil
}

func (w *workerExecutionService) executeCommands(ctx context.Context, jobID task.JobID, commands []patch.PatchCommand) *patch.PatchExecutionResult {
	if w.config.ParallelExecution {
		return w.executeParallel(ctx, jobID, commands)
	}
	return w.executeSequential(ctx, jobID, commands)
}

func (w *workerExecutionService) autoCommitAfterExecution(ctx context.Context, jobID task.JobID, result *patch.PatchExecutionResult) {
	if !w.config.AutoCommit || result.ExecutedCmds == 0 {
		return
	}
	postCommitHash, err := w.autoCommitChanges(ctx, jobID,
		fmt.Sprintf("Patch execution: %d commands", result.ExecutedCmds))
	if err == nil {
		result.WithGitCommit(postCommitHash)
		return
	}
	fmt.Printf("[Worker] Post-commit failed: %v\n", err)
}

func (w *workerExecutionService) finalizeExecutionResult(commands []patch.PatchCommand, result *patch.PatchExecutionResult) *patch.PatchExecutionResult {
	summary := fmt.Sprintf("実行: %d 件, 成功: %d 件, 失敗: %d 件",
		len(commands), result.ExecutedCmds, result.FailedCmds)
	w.classifyExecutionFailure(result)
	result = result.WithSummary(summary)

	fmt.Printf("[Worker] Patch execution completed: %s\n", summary)

	return result
}

// executeSequential はコマンドを順次実行
func (w *workerExecutionService) executeSequential(ctx context.Context, jobID task.JobID, commands []patch.PatchCommand) *patch.PatchExecutionResult {
	result := patch.NewPatchExecutionResult()
	for i, cmd := range commands {
		cmdResult := w.executeCommand(ctx, jobID, cmd, i)
		result.AddResult(cmdResult)

		if !cmdResult.Success && w.config.StopOnError {
			fmt.Printf("[Worker] Execution stopped on error at command %d\n", i)
			break
		}
	}
	return result
}

// executeParallel はType-Based Phased Executionで並列実行
// file_edit → shell_command → git_operation のフェーズ順
// 同フェーズ内は goroutine + semaphore で並列化
func (w *workerExecutionService) executeParallel(ctx context.Context, jobID task.JobID, commands []patch.PatchCommand) *patch.PatchExecutionResult {
	// フェーズ分類
	phases := []patch.Type{patch.TypeFileEdit, patch.TypeShellCommand, patch.TypeGitOperation}
	grouped := make(map[patch.Type][]indexedCommand)

	for i, cmd := range commands {
		grouped[cmd.Type] = append(grouped[cmd.Type], indexedCommand{index: i, cmd: cmd})
	}

	maxParallel := w.config.MaxParallelism
	if maxParallel <= 0 {
		maxParallel = 4
	}

	result := patch.NewPatchExecutionResult()

	for _, phase := range phases {
		cmds := grouped[phase]
		if len(cmds) == 0 {
			continue
		}

		fmt.Printf("[Worker] Phase %s: %d commands (parallel=%d)\n", phase, len(cmds), maxParallel)

		// セマフォ付き並列実行
		sem := make(chan struct{}, maxParallel)
		var mu sync.Mutex
		var wg sync.WaitGroup

		phaseResults := make([]patch.CommandResult, len(cmds))

		for j, ic := range cmds {
			wg.Add(1)
			go func(idx int, ic indexedCommand) {
				defer wg.Done()

				sem <- struct{}{}        // acquire
				defer func() { <-sem }() // release

				cmdResult := w.executeCommand(ctx, jobID, ic.cmd, ic.index)
				mu.Lock()
				phaseResults[idx] = cmdResult
				mu.Unlock()
			}(j, ic)
		}

		wg.Wait()

		// 結果を元のインデックス順に追加
		for _, cr := range phaseResults {
			result.AddResult(cr)
		}

		// フェーズ内で失敗があり StopOnError の場合は次フェーズへ進まない
		if w.config.StopOnError && result.FailedCmds > 0 {
			fmt.Printf("[Worker] Phase %s had failures, stopping\n", phase)
			break
		}
	}

	return result
}

// indexedCommand はインデックス付きコマンド
type indexedCommand struct {
	index int
	cmd   patch.PatchCommand
}

// executeCommand は単一コマンドを実行
func (w *workerExecutionService) executeCommand(
	ctx context.Context,
	jobID task.JobID,
	cmd patch.PatchCommand,
	index int,
) patch.CommandResult {
	start := time.Now()
	var output string
	var err error

	// Type別に処理を振り分け
	switch cmd.Type {
	case patch.TypeFileEdit:
		output, err = w.executeFileEdit(ctx, cmd)
	case patch.TypeShellCommand:
		output, err = w.executeShellCommand(ctx, cmd)
	case patch.TypeGitOperation:
		output, err = w.executeGitOperation(ctx, cmd)
	default:
		err = fmt.Errorf("unknown command type: %s", cmd.Type)
	}

	duration := time.Since(start)
	success := err == nil

	// ログ出力
	if success {
		fmt.Printf("[Worker] Command %d executed: %s %s (%.2fs)\n",
			index, cmd.Type, cmd.Action, duration.Seconds())
	} else {
		fmt.Printf("[Worker] Command %d failed: %s %s - %v\n",
			index, cmd.Type, cmd.Action, err)
	}

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	return patch.CommandResult{
		Command: cmd,
		Success: success,
		Output:  output,
		Error:   errStr,
	}
}
