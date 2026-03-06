package tools

import (
	"context"
	"fmt"
)

// SubagentFunc はサブエージェント呼び出し関数の型
type SubagentFunc func(ctx context.Context, message string) (string, error)

// executeSubagent はサブエージェントを呼び出すツール
func (r *ToolRunner) executeSubagent(ctx context.Context, args map[string]interface{}) (string, error) {
	agentName, ok := args["agent"].(string)
	if !ok || agentName == "" {
		return "", fmt.Errorf("'agent' argument is required and must be a string")
	}

	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("'message' argument is required and must be a string")
	}

	fn, exists := r.config.Subagents[agentName]
	if !exists {
		available := make([]string, 0, len(r.config.Subagents))
		for k := range r.config.Subagents {
			available = append(available, k)
		}
		return "", fmt.Errorf("unknown agent: %s (available: %v)", agentName, available)
	}

	return fn(ctx, message)
}
