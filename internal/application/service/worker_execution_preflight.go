package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
)

var blockedSelfLifecycleCommandPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{
		name: "picoclaw.service lifecycle change",
		re:   regexp.MustCompile(`(?i)\b(systemctl|service)\b[^\n;&|]*(restart|start|stop|reload|enable|disable)[^\n;&|]*\bpicoclaw(?:\.service)?\b`),
	},
	{
		name: "picoclaw.service lifecycle change",
		re:   regexp.MustCompile(`(?i)\bservice\b[^\n;&|]*\bpicoclaw(?:\.service)?\b[^\n;&|]*(restart|start|stop|reload|enable|disable)\b`),
	},
	{
		name: "picoclaw process kill",
		re:   regexp.MustCompile(`(?i)\b(pkill|killall)\b[^\n;&|]*\bpicoclaw\b`),
	},
	{
		name: "RenCrow live binary install",
		re:   regexp.MustCompile(`(?i)(\bmake\s+install\b|\.local/bin/picoclaw|~/.local/bin/picoclaw)`),
	},
}

func (w *workerExecutionService) validateCommandsBeforeExecution(commands []patch.PatchCommand) error {
	for index, cmd := range commands {
		if cmd.Type != patch.TypeShellCommand {
			continue
		}
		if reason := blockedSelfLifecycleCommandReason(cmd.Target); reason != "" {
			return fmt.Errorf("approval required: command %d modifies RenCrow runtime lifecycle or live binary (%s): %s", index+1, reason, cmd.Target)
		}
	}
	return nil
}

func blockedSelfLifecycleCommandReason(command string) string {
	normalized := strings.TrimSpace(command)
	if normalized == "" {
		return ""
	}
	for _, pattern := range blockedSelfLifecycleCommandPatterns {
		if pattern.re.MatchString(normalized) {
			return pattern.name
		}
	}
	return ""
}
