package orchestrator

import (
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func shouldTraceShiroDelegation(route routing.Route) bool {
	switch route {
	case routing.RouteOPS:
		return true
	default:
		return false
	}
}

func formatMioToShiroInstruction(t task.Task, route routing.Route) string {
	return fmt.Sprintf("MioからShiroへの指示: route=%s job=%s。ユーザー指示を実行タスクとして扱って。内容: %s",
		route.String(), t.JobID().String(), traceShortText(t.UserMessage(), 600))
}

func formatShiroToWorkerInstruction(req CodeExecutionRequest, p *proposal.Proposal) string {
	patchBytes := 0
	if p != nil {
		patchBytes = len(p.Patch())
	}
	return fmt.Sprintf("ShiroからWorkerへの指示: job=%s route=%s。Coderが出したProposalを検証済みとしてWorkerで実行して。patch_bytes=%d plan=%s",
		req.JobID, req.Route.String(), patchBytes, traceShortText(proposalPlanText(p), 700))
}

func formatWorkerToShiroResult(result *patch.PatchExecutionResult, err error) string {
	if err != nil {
		return "WorkerからShiroへの戻り: 実行失敗。error=" + traceShortText(err.Error(), 700)
	}
	if result == nil {
		return "WorkerからShiroへの戻り: 実行結果なし。"
	}
	return fmt.Sprintf("WorkerからShiroへの戻り: success=%t executed=%d failed=%d summary=%s",
		result.Success, result.ExecutedCmds, result.FailedCmds, traceShortText(result.Summary, 700))
}

func formatShiroToMioReport(route routing.Route, jobID, body string) string {
	return fmt.Sprintf("ShiroからMioへの戻り報告: route=%s job=%s。%s",
		route.String(), strings.TrimSpace(jobID), traceShortText(body, 900))
}

func proposalPlanText(p *proposal.Proposal) string {
	if p == nil {
		return ""
	}
	return p.Plan()
}

func traceShortText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "..."
}
