package orchestrator

import (
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

const (
	sharedRoleContextLimit      = 8
	sharedRoleContextEntryLimit = 360
	sharedRoleContextTotalLimit = 2800
	sharedRoleContextHeader     = "共有コンテキスト（Worker/Heavy/Wild共通）"
)

func withSharedRoleContext(t task.Task, memory *session.CentralMemory) task.Task {
	content := withSharedRoleContextText(t.UserMessage(), memory)
	if content == t.UserMessage() {
		return t
	}
	return t.WithUserMessage(content)
}

func withSharedRoleContextText(message string, memory *session.CentralMemory) string {
	message = strings.TrimSpace(message)
	if message == "" || strings.Contains(message, sharedRoleContextHeader) {
		return message
	}
	contextBlock := buildSharedRoleContext(memory, sharedRoleContextLimit)
	if contextBlock == "" {
		return message
	}
	return contextBlock + "\n\n現在の依頼:\n" + message
}

func buildSharedRoleContext(memory *session.CentralMemory, limit int) string {
	if memory == nil {
		return ""
	}
	entries := memory.GetUnifiedView(limit)
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(sharedRoleContextHeader)
	b.WriteString(":\n")
	for _, entry := range entries {
		msg := entry.Message
		content := compactSharedRoleContextLine(msg.Content)
		if content == "" {
			continue
		}
		line := fmt.Sprintf("- %s -> %s: %s\n", strings.TrimSpace(msg.From), strings.TrimSpace(msg.To), content)
		if b.Len()+len(line) > sharedRoleContextTotalLimit {
			break
		}
		b.WriteString(line)
	}
	if strings.TrimSpace(b.String()) == sharedRoleContextHeader+":" {
		return ""
	}
	return strings.TrimRight(b.String(), "\n")
}

func compactSharedRoleContextLine(content string) string {
	content = strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if content == "" {
		return ""
	}
	if len(content) <= sharedRoleContextEntryLimit {
		return content
	}
	return content[:sharedRoleContextEntryLimit] + "..."
}
