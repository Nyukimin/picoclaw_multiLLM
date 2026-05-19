package aiworkflow

import "testing"

func TestValidateAIWorkflowRecords(t *testing.T) {
	if err := ValidateWorkflowEvent(WorkflowEvent{EventID: "evt_1", EventType: "project_init_started", Status: "completed"}); err != nil {
		t.Fatalf("ValidateWorkflowEvent() error = %v", err)
	}
	if err := ValidateProjectMemoryIndex(ProjectMemoryIndex{ID: "mem_1", Repo: "repo", FilePath: ".ai/PROJECT_MEMORY.md", MemoryType: "project"}); err != nil {
		t.Fatalf("ValidateProjectMemoryIndex() error = %v", err)
	}
	if err := ValidateWorktreeRegistry(WorktreeRegistry{WorktreeID: "wt_1", Repo: "repo", Path: "../worktrees/repo-feature", Branch: "feature/a", Status: "active"}); err != nil {
		t.Fatalf("ValidateWorktreeRegistry() error = %v", err)
	}
	if err := ValidateCommandRegistry(CommandRegistry{CommandName: "/review-architecture", FilePath: "commands/review-architecture.md"}); err != nil {
		t.Fatalf("ValidateCommandRegistry() error = %v", err)
	}
	if err := ValidateContextUsage(ContextUsage{EventID: "ctx_1", Agent: "Coder", InputTokens: 1}); err != nil {
		t.Fatalf("ValidateContextUsage() error = %v", err)
	}
}

func TestValidateContextUsageRejectsNegativeCounts(t *testing.T) {
	err := ValidateContextUsage(ContextUsage{EventID: "ctx_1", Agent: "Coder", InputTokens: -1})
	if err == nil {
		t.Fatal("expected negative counts to fail")
	}
}
