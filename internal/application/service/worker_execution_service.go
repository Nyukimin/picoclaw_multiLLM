package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// 1. Patchをパース
	commands, err := patch.ParsePatch(p.Patch())
	if err != nil {
		return nil, fmt.Errorf("patch parse error: %w", err)
	}

	// 2. 実行前サマリ表示
	if w.config.ShowExecutionSummary {
		w.showExecutionSummary(jobID, commands)
	}

	// 3. Git auto-commit（実行前）
	if w.config.AutoCommit {
		preCommitHash, err := w.autoCommitChanges(ctx, jobID, "Before patch execution")
		if err != nil {
			return nil, fmt.Errorf("pre-execution auto-commit failed: %w", err)
		}
		fmt.Printf("[Worker] Pre-commit succeeded: %s\n", preCommitHash)
	}

	// 4. コマンド実行（並列 or 順次）
	var result *patch.PatchExecutionResult
	if w.config.ParallelExecution {
		result = w.executeParallel(ctx, jobID, commands)
	} else {
		result = w.executeSequential(ctx, jobID, commands)
	}

	// 5. Git auto-commit（実行後）
	if w.config.AutoCommit && result.ExecutedCmds > 0 {
		postCommitHash, err := w.autoCommitChanges(ctx, jobID,
			fmt.Sprintf("Patch execution: %d commands", result.ExecutedCmds))
		if err == nil {
			result = result.WithGitCommit(postCommitHash)
		} else {
			fmt.Printf("[Worker] Post-commit failed: %v\n", err)
		}
	}

	// 6. サマリ生成
	summary := fmt.Sprintf("実行: %d 件, 成功: %d 件, 失敗: %d 件",
		len(commands), result.ExecutedCmds, result.FailedCmds)
	result = result.WithSummary(summary)

	fmt.Printf("[Worker] Patch execution completed: %s\n", summary)

	return result, nil
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

// executeFileEdit はファイル編集コマンドを実行
func (w *workerExecutionService) executeFileEdit(
	ctx context.Context,
	cmd patch.PatchCommand,
) (string, error) {
	target := cmd.Target

	// ワークスペース外書き込み禁止
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	absWorkspace, _ := filepath.Abs(w.config.Workspace)
	if !strings.HasPrefix(absTarget, absWorkspace) {
		return "", fmt.Errorf("security error: file path outside workspace: %s", target)
	}

	// 保護ファイルチェック
	if w.isProtectedFile(target) {
		switch w.config.ActionOnProtected {
		case "error":
			return "", fmt.Errorf("protected file: %s", target)
		case "skip":
			return "Skipped (protected file)", nil
		case "log":
			fmt.Printf("[Worker] Warning: accessing protected file: %s\n", target)
		}
	}

	// Action別処理
	switch cmd.Action {
	case patch.ActionCreate, patch.ActionUpdate:
		return w.writeFile(target, cmd.Content)
	case patch.ActionDelete:
		return w.deleteFile(target)
	case patch.ActionAppend:
		return w.appendFile(target, cmd.Content)
	case patch.ActionMkdir:
		return w.createDirectory(target)
	case patch.ActionRename:
		newName := cmd.Metadata["new_name"]
		if newName == "" {
			return "", fmt.Errorf("rename: metadata 'new_name' is required")
		}
		return w.renameFile(target, newName)
	case patch.ActionCopy:
		dest := cmd.Metadata["destination"]
		if dest == "" {
			return "", fmt.Errorf("copy: metadata 'destination' is required")
		}
		return w.copyFile(target, dest)
	default:
		return "", fmt.Errorf("unknown file_edit action: %s", cmd.Action)
	}
}

// executeShellCommand はシェルコマンドを実行
func (w *workerExecutionService) executeShellCommand(
	ctx context.Context,
	cmd patch.PatchCommand,
) (string, error) {
	// タイムアウト設定
	timeout := time.Duration(w.config.CommandTimeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// コマンド実行
	command := cmd.Target
	shellCmd := exec.CommandContext(ctx, "sh", "-c", command)

	// ワークスペース内で実行
	shellCmd.Dir = w.config.Workspace

	// 環境変数（Metadataから取得）
	if env := cmd.Metadata["env"]; env != "" {
		shellCmd.Env = append(os.Environ(), strings.Split(env, ",")...)
	}

	output, err := shellCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("shell command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// executeGitOperation はGit操作を実行
func (w *workerExecutionService) executeGitOperation(
	ctx context.Context,
	cmd patch.PatchCommand,
) (string, error) {
	timeout := time.Duration(w.config.GitTimeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Git操作はTargetにコマンド全体が入っている
	gitArgs := strings.Fields(cmd.Target)

	gitCmd := exec.CommandContext(ctx, "git", gitArgs...)
	gitCmd.Dir = w.config.Workspace

	output, err := gitCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git operation failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// autoCommitChanges はGit auto-commitを実行
func (w *workerExecutionService) autoCommitChanges(
	ctx context.Context,
	jobID task.JobID,
	message string,
) (string, error) {
	timeout := time.Duration(w.config.GitTimeout) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// git add -A
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = w.config.Workspace
	if output, err := addCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %w, output: %s", err, string(output))
	}

	// git commit
	commitMsg := fmt.Sprintf("%s %s\n\nJobID: %s",
		w.config.CommitMessagePrefix, message, jobID.String())
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	commitCmd.Dir = w.config.Workspace
	if output, err := commitCmd.CombinedOutput(); err != nil {
		// 変更がない場合は成功扱い
		if strings.Contains(string(output), "nothing to commit") {
			return "no-changes", nil
		}
		return "", fmt.Errorf("git commit failed: %w, output: %s", err, string(output))
	}

	// 最新コミットハッシュ取得
	hashCmd := exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	hashCmd.Dir = w.config.Workspace
	hashOutput, err := hashCmd.Output()
	if err != nil {
		return "", fmt.Errorf("get commit hash failed: %w", err)
	}

	return strings.TrimSpace(string(hashOutput)), nil
}

// writeFile はファイル書き込み
func (w *workerExecutionService) writeFile(path, content string) (string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory failed: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file failed: %w", err)
	}
	return fmt.Sprintf("File written: %s (%d bytes)", path, len(content)), nil
}

// deleteFile はファイル削除
func (w *workerExecutionService) deleteFile(path string) (string, error) {
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("delete file failed: %w", err)
	}
	return fmt.Sprintf("File deleted: %s", path), nil
}

// appendFile はファイル末尾追記
func (w *workerExecutionService) appendFile(path, content string) (string, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("open file failed: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return "", fmt.Errorf("append file failed: %w", err)
	}
	return fmt.Sprintf("Content appended: %s (%d bytes)", path, len(content)), nil
}

// createDirectory はディレクトリ作成
func (w *workerExecutionService) createDirectory(path string) (string, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("create directory failed: %w", err)
	}
	return fmt.Sprintf("Directory created: %s", path), nil
}

// renameFile はファイル/ディレクトリリネーム
func (w *workerExecutionService) renameFile(oldPath, newPath string) (string, error) {
	if err := os.Rename(oldPath, newPath); err != nil {
		return "", fmt.Errorf("rename failed: %w", err)
	}
	return fmt.Sprintf("Renamed: %s -> %s", oldPath, newPath), nil
}

// copyFile はファイル/ディレクトリコピー
func (w *workerExecutionService) copyFile(src, dest string) (string, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("read source file failed: %w", err)
	}

	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", fmt.Errorf("write destination file failed: %w", err)
	}

	return fmt.Sprintf("Copied: %s -> %s (%d bytes)", src, dest, len(data)), nil
}

// isProtectedFile は保護ファイルかチェック
func (w *workerExecutionService) isProtectedFile(path string) bool {
	basename := filepath.Base(path)
	for _, pattern := range w.config.ProtectedPatterns {
		matched, _ := filepath.Match(pattern, basename)
		if matched {
			return true
		}
	}
	return false
}

// showExecutionSummary は実行前サマリを表示
func (w *workerExecutionService) showExecutionSummary(jobID task.JobID, commands []patch.PatchCommand) {
	fileEdits := 0
	shellCmds := 0
	gitOps := 0
	for _, cmd := range commands {
		switch cmd.Type {
		case patch.TypeFileEdit:
			fileEdits++
		case patch.TypeShellCommand:
			shellCmds++
		case patch.TypeGitOperation:
			gitOps++
		}
	}

	fmt.Printf("[Worker] Execution Summary (JobID: %s)\n", jobID.String())
	fmt.Printf("  Total Commands: %d\n", len(commands))
	fmt.Printf("  File Edits: %d\n", fileEdits)
	fmt.Printf("  Shell Commands: %d\n", shellCmds)
	fmt.Printf("  Git Operations: %d\n", gitOps)
}
