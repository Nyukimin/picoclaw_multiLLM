package service

import (
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
)

func (w *workerExecutionService) classifyExecutionFailure(result *patch.PatchExecutionResult) {
	if result == nil || result.Success || len(result.Results) == 0 {
		return
	}
	for idx, cr := range result.Results {
		if cr.Success {
			continue
		}
		kind, reason, retryable := classifyFailure(cr.Error, cr.Output)
		result.FailedIndex = idx
		result.WithFailureMetadata(kind, reason, retryable)
		return
	}
	result.WithFailureMetadata("unknown", "execution failed", false)
}

func classifyFailure(errText, output string) (kind, reason string, retryable bool) {
	text := strings.ToLower(strings.TrimSpace(errText + "\n" + output))
	switch {
	case strings.Contains(text, "patch parse error"):
		return "patch_parse_failed", strings.TrimSpace(errText), true
	case strings.Contains(text, "security error"), strings.Contains(text, "protected file"):
		return "unsafe_operation", strings.TrimSpace(errText), false
	case strings.Contains(text, "command not found"), strings.Contains(text, "not found"), strings.Contains(text, "exit status 127"):
		return "missing_command", strings.TrimSpace(errText), true
	case strings.Contains(text, "no module named"), strings.Contains(text, "module not found"), strings.Contains(text, "cannot find package"), strings.Contains(text, "missing dependency"):
		return "missing_dependency", strings.TrimSpace(errText), true
	case strings.Contains(text, "verification failed"), strings.Contains(text, "test failed"), strings.Contains(text, "assert"):
		return "verification_failed", strings.TrimSpace(errText), true
	case strings.Contains(text, "spec missing"), strings.Contains(text, "missing required"), strings.Contains(text, "insufficient"):
		return "spec_missing", strings.TrimSpace(errText), true
	default:
		reason = strings.TrimSpace(errText)
		if reason == "" {
			reason = strings.TrimSpace(output)
		}
		if reason == "" {
			reason = "execution failed"
		}
		return "unknown", reason, false
	}
}
