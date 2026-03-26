package orchestrator

import (
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
)

// truncate はビュワー表示用に長いテキストを切り詰める
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// 行単位で切り詰め
	lines := strings.SplitN(s, "\n", -1)
	var b strings.Builder
	for _, line := range lines {
		if b.Len()+len(line)+1 > maxLen {
			b.WriteString("\n... (truncated)")
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// formatExecutionResult はProposalとPatchExecutionResultを整形
func formatExecutionResult(
	p *proposal.Proposal,
	result *patch.PatchExecutionResult,
) string {
	// 成功/失敗の絵文字
	statusEmoji := "✅"
	if !result.Success {
		statusEmoji = "⚠️"
	}

	// Gitコミット行
	gitCommitLine := ""
	if result.GitCommit != "" && result.GitCommit != "no-changes" {
		shortHash := result.GitCommit
		if len(shortHash) > 8 {
			shortHash = shortHash[:8]
		}
		gitCommitLine = fmt.Sprintf("\n- **Git Commit**: `%s`", shortHash)
	}

	// エラー詳細
	errorDetails := ""
	if !result.Success && len(result.Results) > 0 {
		errorDetails = "\n\n### Errors\n"
		for _, r := range result.Results {
			if !r.Success {
				errorDetails += fmt.Sprintf("- %s: %s\n", r.Command.Type, r.Error)
			}
		}
	}

	successCount := result.ExecutedCmds - result.FailedCmds

	return fmt.Sprintf(`%s **Execution Result**

## Plan
%s

## Result
- **Total Steps**: %d
- **Success Steps**: %d
- **Failed Steps**: %d%s%s`,
		statusEmoji,
		p.Plan(),
		result.ExecutedCmds,
		successCount,
		result.FailedCmds,
		gitCommitLine,
		errorDetails,
	)
}
