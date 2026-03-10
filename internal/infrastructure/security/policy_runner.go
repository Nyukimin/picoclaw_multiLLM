package security

import (
	"context"
	"fmt"
	"time"

	executionapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/execution"
	domainexecution "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/tool"
)

// PolicyRunner は RunnerV2 をポリシー適用付きでラップする
type PolicyRunner struct {
	inner        tool.RunnerV2
	execService  *executionapp.Service
	toolMetaByID map[string]tool.ToolMetadata
	requestedBy  string
}

func NewPolicyRunner(inner tool.RunnerV2, engine *PolicyEngine, repo domainexecution.Repository, requestedBy string, approvalTTL time.Duration) (*PolicyRunner, error) {
	if inner == nil {
		return nil, fmt.Errorf("inner runner is required")
	}
	if engine == nil {
		return nil, fmt.Errorf("policy engine is required")
	}
	metas, err := inner.ListTools(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}
	metaMap := make(map[string]tool.ToolMetadata, len(metas))
	for _, m := range metas {
		metaMap[m.ToolID] = m
	}
	svc := executionapp.NewService(engine, inner, repo, approvalTTL)
	return &PolicyRunner{
		inner:        inner,
		execService:  svc,
		toolMetaByID: metaMap,
		requestedBy:  requestedBy,
	}, nil
}

func (r *PolicyRunner) ExecuteV2(ctx context.Context, toolName string, args map[string]any) (*tool.ToolResponse, error) {
	meta, exists := r.toolMetaByID[toolName]
	if !exists {
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}

	action := domainexecution.Action{
		JobID:            fmt.Sprintf("job-%d", time.Now().UnixNano()),
		ActionID:         fmt.Sprintf("act-%d", time.Now().UnixNano()),
		Tool:             toolName,
		Arguments:        args,
		RequestedBy:      r.requestedBy,
		RequiresApproval: meta.RequiresApproval,
		RequestedAt:      time.Now().UTC(),
	}
	result, err := r.execService.RequestToolExecution(ctx, action)
	if err != nil {
		return nil, err
	}

	switch result.Record.Status {
	case domainexecution.StatusWaitingApproval:
		return tool.NewError(tool.ErrPermissionDenied, "approval required: use CLI approve command", map[string]any{"job_id": action.JobID, "action_id": action.ActionID}), nil
	case domainexecution.StatusDenied:
		return tool.NewError(tool.ErrPermissionDenied, result.Record.Reason, map[string]any{"rule": "policy_deny"}), nil
	default:
		if result.Response != nil {
			return result.Response, nil
		}
		return tool.NewError(tool.ErrInternalError, "empty tool response", nil), nil
	}
}

func (r *PolicyRunner) ListTools(ctx context.Context) ([]tool.ToolMetadata, error) {
	return r.inner.ListTools(ctx)
}
