package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ToolRunner はツール実行の実装
type ToolRunner struct {
	tools map[string]ToolFunc
}

// ToolFunc はツール実行関数の型
type ToolFunc func(ctx context.Context, args map[string]interface{}) (string, error)

// NewToolRunner は新しいToolRunnerを作成
func NewToolRunner() *ToolRunner {
	runner := &ToolRunner{
		tools: make(map[string]ToolFunc),
	}

	// ツール登録
	runner.registerTools()

	return runner
}

// registerTools は利用可能なツールを登録
func (r *ToolRunner) registerTools() {
	r.tools["shell"] = r.executeShell
	r.tools["file_read"] = r.executeFileRead
	r.tools["file_write"] = r.executeFileWrite
	r.tools["file_list"] = r.executeFileList
}

// Execute はツールを実行
func (r *ToolRunner) Execute(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	toolFunc, exists := r.tools[toolName]
	if !exists {
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	return toolFunc(ctx, args)
}

// List は利用可能なツール一覧を返す
func (r *ToolRunner) List(ctx context.Context) ([]string, error) {
	tools := make([]string, 0, len(r.tools))
	for name := range r.tools {
		tools = append(tools, name)
	}
	return tools, nil
}

// executeShell はシェルコマンドを実行
func (r *ToolRunner) executeShell(ctx context.Context, args map[string]interface{}) (string, error) {
	command, ok := args["command"].(string)
	if !ok {
		return "", fmt.Errorf("'command' argument is required and must be a string")
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return string(output), nil
}

// executeFileRead はファイルを読み込む
func (r *ToolRunner) executeFileRead(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

// executeFileWrite はファイルに書き込む
func (r *ToolRunner) executeFileWrite(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("'content' argument is required and must be a string")
	}

	// ディレクトリが存在しない場合は作成
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path), nil
}

// executeFileList はディレクトリ内のファイル一覧を取得
func (r *ToolRunner) executeFileList(ctx context.Context, args map[string]interface{}) (string, error) {
	path, ok := args["path"].(string)
	if !ok {
		return "", fmt.Errorf("'path' argument is required and must be a string")
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %w", err)
	}

	var result strings.Builder
	for i, entry := range entries {
		if entry.IsDir() {
			result.WriteString(fmt.Sprintf("%s/\n", entry.Name()))
		} else {
			result.WriteString(fmt.Sprintf("%s\n", entry.Name()))
		}
		if i >= 1000 {
			result.WriteString("... (truncated, too many entries)\n")
			break
		}
	}

	return result.String(), nil
}
