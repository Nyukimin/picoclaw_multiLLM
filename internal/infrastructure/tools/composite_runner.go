package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

const registeredToolTimeout = 30 * time.Second

// CompositeRunnerV2 は基本 ToolRunner をラップし、ToolRegistry 登録済みツールへの
// フォールバック実行を提供する。
//
// 動作:
//  1. base.ExecuteV2() を試行
//  2. "unknown tool" エラーの場合、registry.Get() でツール検索
//  3. workspaceDir/tools/<name>.sh が存在 → sh で実行
//  4. それ以外 → 元のエラーを返す
type CompositeRunnerV2 struct {
	base         tool.RunnerV2
	registry     capability.ToolRegistry
	workspaceDir string
}

// NewCompositeRunnerV2 は CompositeRunnerV2 を作成する
func NewCompositeRunnerV2(base tool.RunnerV2, registry capability.ToolRegistry, workspaceDir string) *CompositeRunnerV2 {
	return &CompositeRunnerV2{
		base:         base,
		registry:     registry,
		workspaceDir: workspaceDir,
	}
}

// ExecuteV2 はツールを実行する。基本 Runner で未知のツールは ToolRegistry にフォールバックする。
func (c *CompositeRunnerV2) ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*tool.ToolResponse, error) {
	resp, err := c.base.ExecuteV2(ctx, toolName, args)
	if err == nil {
		return resp, nil
	}

	// 基本 Runner で未知のツールのみ ToolRegistry にフォールバック
	if !isUnknownToolError(err) {
		return nil, err
	}

	return c.executeRegistered(ctx, toolName, args, err)
}

func isUnknownToolError(err error) bool {
	return errors.Is(err, ErrUnknownTool) || strings.Contains(err.Error(), "unknown tool")
}

// ListTools は基本 Runner と ToolRegistry のツール一覧をマージして返す
func (c *CompositeRunnerV2) ListTools(ctx context.Context) ([]tool.ToolMetadata, error) {
	baseMetas, err := c.base.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	if c.registry == nil {
		return baseMetas, nil
	}

	entries, err := c.registry.ListForPlatform(ctx, runtime.GOOS)
	if err != nil {
		return baseMetas, nil // registry エラーは無視して base のみ返す
	}

	existing := make(map[string]bool, len(baseMetas))
	for _, m := range baseMetas {
		existing[m.ToolID] = true
	}

	result := make([]tool.ToolMetadata, len(baseMetas))
	copy(result, baseMetas)
	for _, e := range entries {
		if existing[e.Name] {
			continue
		}
		result = append(result, tool.ToolMetadata{
			ToolID:      e.Name,
			Version:     "1.0.0",
			Category:    "registered",
			Description: e.Description,
		})
	}
	return result, nil
}

// executeRegistered は ToolRegistry のツールをシェルスクリプトとして実行する
func (c *CompositeRunnerV2) executeRegistered(ctx context.Context, toolName string, args map[string]any, origErr error) (*tool.ToolResponse, error) {
	if c.registry == nil {
		return nil, origErr
	}

	if _, err := c.registry.Get(ctx, toolName); err != nil {
		return nil, origErr // registry にも存在しない
	}

	scriptPath := filepath.Join(c.workspaceDir, "tools", toolName+".sh")
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("registered tool %q: script not found at %s", toolName, scriptPath)
	}

	// args を JSON 化して引数として渡す
	argsJSON, err := json.Marshal(args)
	if err != nil {
		log.Printf("[CompositeRunner] WARN: failed to marshal args for %q: %v", toolName, err)
		argsJSON = []byte("{}")
	}

	ctx, cancel := context.WithTimeout(ctx, registeredToolTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", scriptPath, string(argsJSON))
	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return tool.NewError(tool.ErrTimeout, fmt.Sprintf("tool %q timed out", toolName), nil), nil
		}
		return tool.NewError(tool.ErrInternalError, fmt.Sprintf("tool %q failed: %v", toolName, err), nil), nil
	}

	return tool.NewSuccess(string(output)), nil
}
