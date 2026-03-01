package chat

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

// TaskResult represents the result of task execution by Worker/Order agents.
type TaskResult struct {
	JobID      string
	Success    bool
	Output     string
	Error      error
	Metadata   map[string]interface{}
}

// FinalDecisionModule handles final decision-making and response formatting.
// In the new architecture, Chat receives aggregated results from Worker
// and makes the final decision on what to send to the user.
type FinalDecisionModule struct {
	agent *modules.AgentCore
}

// NewFinalDecisionModule creates a new final decision module.
func NewFinalDecisionModule() *FinalDecisionModule {
	return &FinalDecisionModule{}
}

// Name returns the module name.
func (m *FinalDecisionModule) Name() string {
	return "FinalDecision"
}

// Initialize initializes the module with the agent core.
func (m *FinalDecisionModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *FinalDecisionModule) Shutdown(ctx context.Context) error {
	return nil
}

// MakeFinalDecision processes the task result and produces the final response.
// This is where Chat applies its judgment on whether to approve, reject,
// or request more information.
func (m *FinalDecisionModule) MakeFinalDecision(ctx context.Context, result TaskResult) string {
	// For now, just return the output
	// Future: Add approval flow, quality checks, formatting
	if !result.Success {
		if result.Error != nil {
			return "エラーが発生しました: " + result.Error.Error()
		}
		return "タスクの実行に失敗しました。"
	}

	return result.Output
}
