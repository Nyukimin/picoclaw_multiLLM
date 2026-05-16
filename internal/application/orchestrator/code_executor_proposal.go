package orchestrator

import (
	"context"
	"fmt"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

func shouldUseProposalPath(route routing.Route, target codeTarget) bool {
	switch route {
	case routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return true
	}
	return target.degradedRoute == routing.RouteCODE3
}

// tryExecuteProposalPath はProposal生成→Worker実行パスを試行
func (e *DefaultCodeExecutor) tryExecuteProposalPath(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
) (CodeExecutionResponse, bool, error) {
	coderWithProposal, ok := proposalCoderForTarget(target)
	if !ok {
		return CodeExecutionResponse{}, false, nil
	}

	p, err := e.generateProposalForTarget(ctx, req, target, coderWithProposal)
	if err != nil {
		return CodeExecutionResponse{}, true, err
	}

	if err := e.validateGeneratedProposal(req, target, p); err != nil {
		return CodeExecutionResponse{}, true, err
	}

	e.emitProposalPlan(req, target, p)
	result, err := e.executeProposalWithWorker(ctx, req, p)
	if err != nil {
		return CodeExecutionResponse{}, true, err
	}

	formatted := formatExecutionResult(p, result)
	e.emitProposalExecutionResult(req, formatted)

	return buildProposalHandledResponse(formatted), true, nil
}

func proposalCoderForTarget(target codeTarget) (CoderAgentWithProposal, bool) {
	coderWithProposal, ok := target.coder.(CoderAgentWithProposal)
	return coderWithProposal, ok
}

func (e *DefaultCodeExecutor) generateProposalForTarget(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
	coderWithProposal CoderAgentWithProposal,
) (*proposal.Proposal, error) {
	p, err := coderWithProposal.GenerateProposal(ctx, req.Task)
	if err != nil {
		e.emit("agent.response", target.name, "shiro", "エラー: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return nil, fmt.Errorf("%s proposal generation failed: %w", target.name, err)
	}
	return p, nil
}

func (e *DefaultCodeExecutor) validateGeneratedProposal(
	req CodeExecutionRequest,
	target codeTarget,
	p *proposal.Proposal,
) error {
	if p != nil && p.IsValid() {
		return nil
	}
	e.emit("agent.response", target.name, "shiro", "無効な Proposal が返されました", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	return fmt.Errorf("%s proposal generation failed: invalid proposal", target.name)
}

func (e *DefaultCodeExecutor) emitProposalPlan(req CodeExecutionRequest, target codeTarget, p *proposal.Proposal) {
	e.emit("agent.response", target.name, "shiro", "## Plan\n"+p.Plan(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
}

func (e *DefaultCodeExecutor) executeProposalWithWorker(
	ctx context.Context,
	req CodeExecutionRequest,
	p *proposal.Proposal,
) (*patch.PatchExecutionResult, error) {
	e.emit("agent.start", "shiro", "mio", "Patch を実行中...", req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	result, err := e.workerExecution.ExecuteProposal(ctx, req.Task.JobID(), p)
	if err != nil {
		e.emit("agent.response", "shiro", "mio", "実行失敗: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return nil, fmt.Errorf("worker execution failed: %w", err)
	}
	return result, nil
}

func (e *DefaultCodeExecutor) emitProposalExecutionResult(req CodeExecutionRequest, formatted string) {
	e.emit("agent.response", "shiro", "mio", formatted, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
}
