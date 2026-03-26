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

	// コマンド結果詳細
	commandDetails := ""
	for i, cmdResult := range result.Results {
		status := "✅"
		if !cmdResult.Success {
			status = "❌"
		}
		commandDetails += fmt.Sprintf("\n%d. %s `%s` %s",
			i+1, status, cmdResult.Command.Action, cmdResult.Command.Target)
		if cmdResult.Error != "" {
			commandDetails += fmt.Sprintf("\n   Error: %s", cmdResult.Error)
		}
	}

	return fmt.Sprintf(`## Plan
%s

## Execution Result
- **Status**: %s
- **Executed**: %d commands
- **Failed**: %d commands
- **Success Rate**: %.1f%%%s

### Command Results%s

## Risk
%s
`,
		p.Plan(),
		statusEmoji,
		result.ExecutedCmds,
		result.FailedCmds,
		result.SuccessRate()*100,
		gitCommitLine,
		commandDetails,
		p.Risk(),
	)
}
