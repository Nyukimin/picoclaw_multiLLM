package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	domainskill "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/skillgovernance"
)

func shouldUseProposalPath(route routing.Route, target codeTarget) bool {
	switch route {
	case routing.RouteCODE1, routing.RouteCODE2, routing.RouteCODE3, routing.RouteCODE4:
		return true
	}
	return false
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
		e.recordCoderProposalEvidence(ctx, req, target, p, nil, "", err)
		return CodeExecutionResponse{}, true, err
	}

	formatted := formatExecutionResult(p, result)
	e.recordCoderProposalEvidence(ctx, req, target, p, result, formatted, nil)
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
	e.emit("worker.request", "shiro", "worker", formatShiroToWorkerInstruction(req, p), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)

	result, err := e.workerExecution.ExecuteProposal(ctx, req.Task.JobID(), p)
	if err != nil {
		e.emit("worker.result", "worker", "shiro", formatWorkerToShiroResult(nil, err), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		e.emit("agent.response", "shiro", "mio", "実行失敗: "+err.Error(), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		e.emit("agent.report", "shiro", "mio", formatShiroToMioReport(req.Route, req.JobID, "実行失敗: "+err.Error()), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
		return nil, fmt.Errorf("worker execution failed: %w", err)
	}
	e.emit("worker.result", "worker", "shiro", formatWorkerToShiroResult(result, nil), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	return result, nil
}

func (e *DefaultCodeExecutor) emitProposalExecutionResult(req CodeExecutionRequest, formatted string) {
	e.emit("agent.response", "shiro", "mio", formatted, req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
	e.emit("agent.report", "shiro", "mio", formatShiroToMioReport(req.Route, req.JobID, formatted), req.Route.String(), req.JobID, req.SessionID, req.Channel, req.ChatID)
}

func (e *DefaultCodeExecutor) recordCoderProposalEvidence(
	ctx context.Context,
	req CodeExecutionRequest,
	target codeTarget,
	p *proposal.Proposal,
	result *patch.PatchExecutionResult,
	formatted string,
	runErr error,
) {
	if e.proposalEvidence == nil || p == nil {
		return
	}
	evidence := domainskill.CoderProposalEvidence{
		JobID:           req.JobID,
		SessionID:       req.SessionID,
		Route:           req.Route.String(),
		Agent:           target.name,
		TaskText:        req.Task.UserMessage(),
		Plan:            p.Plan(),
		Patch:           p.Patch(),
		Risk:            p.Risk(),
		CostHint:        p.CostHint(),
		FormattedResult: formatted,
		Success:         runErr == nil,
	}
	if result != nil {
		evidence.ExecutionSummary = result.Summary
		evidence.Success = result.Success
	}
	if runErr != nil {
		evidence.ExecutionError = runErr.Error()
	}
	paths, err := e.proposalEvidence.SaveCoderProposalEvidence(ctx, evidence)
	if err != nil {
		log.Printf("WARN: failed to save coder proposal evidence job=%s route=%s: %v", req.JobID, req.Route, err)
		return
	}
	if paths.SkillDiffPath != "" || paths.AgentTranscriptPath != "" {
		log.Printf("Coder proposal evidence saved job=%s skill_diff=%s agent_transcript=%s", req.JobID, paths.SkillDiffPath, paths.AgentTranscriptPath)
	}
}
