package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// PatchCommand は patch 内の1つのコマンドを表現
type PatchCommand struct {
	Type     string            `json:"type"`
	Action   string            `json:"action"`
	Target   string            `json:"target"`
	Content  string            `json:"content"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// parsePatch は patch 文字列を解析して PatchCommand のスライスを返す
func parsePatch(patch string) ([]PatchCommand, error) {
	if patch == "" {
		return nil, fmt.Errorf("empty patch")
	}

	var commands []PatchCommand
	if err := json.Unmarshal([]byte(patch), &commands); err != nil {
		return nil, fmt.Errorf("failed to parse patch as JSON: %w", err)
	}

	return commands, nil
}

// executeFileEdit はファイル編集コマンドを実行する
func (a *AgentLoop) executeFileEdit(ctx context.Context, cmd PatchCommand) (string, error) {
	target := cmd.Target

	// Security check: workspace 外への書き込みを禁止
	if !strings.HasPrefix(target, a.workspace) {
		return "", fmt.Errorf("file path outside workspace: %s", target)
	}

	switch cmd.Action {
	case "create", "update":
		err := os.WriteFile(target, []byte(cmd.Content), 0644)
		if err != nil {
			return "", fmt.Errorf("failed to write file %s: %w", target, err)
		}
		return fmt.Sprintf("File %s written successfully (%d bytes)", target, len(cmd.Content)), nil

	case "delete":
		err := os.Remove(target)
		if err != nil {
			return "", fmt.Errorf("failed to delete file %s: %w", target, err)
		}
		return fmt.Sprintf("File %s deleted", target), nil

	case "append":
		f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to open file %s: %w", target, err)
		}
		defer f.Close()

		_, err = f.WriteString(cmd.Content)
		if err != nil {
			return "", fmt.Errorf("failed to append to file %s: %w", target, err)
		}
		return fmt.Sprintf("Appended %d bytes to %s", len(cmd.Content), target), nil

	default:
		return "", fmt.Errorf("unknown action: %s", cmd.Action)
	}
}

// executeShellCommand はシェルコマンドを実行する
func (a *AgentLoop) executeShellCommand(ctx context.Context, cmd PatchCommand) (string, error) {
	command := cmd.Target
	if cmd.Content != "" {
		command += " " + cmd.Content
	}

	execCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	cmdExec := exec.CommandContext(execCtx, "bash", "-c", command)
	cmdExec.Dir = a.workspace

	output, err := cmdExec.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return string(output), nil
}

// executeCommand は PatchCommand の Type に応じて適切な実行関数を呼び出す
func (a *AgentLoop) executeCommand(ctx context.Context, cmd PatchCommand) (string, error) {
	switch cmd.Type {
	case "file_edit":
		return a.executeFileEdit(ctx, cmd)
	case "shell_command":
		return a.executeShellCommand(ctx, cmd)
	case "git_operation":
		return a.executeGitOperation(ctx, cmd)
	default:
		return "", fmt.Errorf("unknown command type: %s", cmd.Type)
	}
}

// executeGitOperation は Git 操作を実行する（未実装）
func (a *AgentLoop) executeGitOperation(ctx context.Context, cmd PatchCommand) (string, error) {
	return "", fmt.Errorf("git_operation not implemented yet")
}

// PatchExecutionResult は patch 実行の結果を表現
type PatchExecutionResult struct {
	Success      bool            `json:"success"`
	ExecutedCmds int             `json:"executed_cmds"`
	FailedCmds   int             `json:"failed_cmds"`
	Results      []CommandResult `json:"results"`
	Summary      string          `json:"summary"`
	GitCommit    string          `json:"git_commit,omitempty"`
}

// CommandResult は個別コマンドの実行結果
type CommandResult struct {
	Command  PatchCommand `json:"command"`
	Success  bool         `json:"success"`
	Output   string       `json:"output"`
	Error    string       `json:"error"`
	Duration int64        `json:"duration"`
}

// executeWorkerPatch は patch を解析して順次実行する
func (a *AgentLoop) executeWorkerPatch(ctx context.Context, patch string, sessionKey string) (*PatchExecutionResult, error) {
	commands, err := parsePatch(patch)
	if err != nil {
		return nil, fmt.Errorf("patch parse error: %w", err)
	}

	result := &PatchExecutionResult{
		Success:      true,
		ExecutedCmds: 0,
		FailedCmds:   0,
		Results:      make([]CommandResult, 0, len(commands)),
	}

	for _, cmd := range commands {
		startTime := time.Now()
		cmdResult := CommandResult{Command: cmd}

		output, err := a.executeCommand(ctx, cmd)
		duration := time.Since(startTime).Milliseconds()

		if err != nil {
			cmdResult.Success = false
			cmdResult.Error = err.Error()
			cmdResult.Duration = duration
			result.FailedCmds++
			result.Success = false
		} else {
			cmdResult.Success = true
			cmdResult.Output = output
			cmdResult.Duration = duration
			result.ExecutedCmds++
		}

		result.Results = append(result.Results, cmdResult)
	}

	result.Summary = fmt.Sprintf("実行: %d 件, 成功: %d 件, 失敗: %d 件",
		len(commands), result.ExecutedCmds, result.FailedCmds)

	return result, nil
}
