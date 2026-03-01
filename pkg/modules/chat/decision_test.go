package chat

import (
	"context"
	"errors"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestFinalDecisionModule(t *testing.T) {
	module := NewFinalDecisionModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "chat",
		Alias: "Mio",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "FinalDecision" {
		t.Errorf("Expected name 'FinalDecision', got '%s'", module.Name())
	}

	// Test successful result
	result := TaskResult{
		JobID:   "job_20260301_001",
		Success: true,
		Output:  "Task completed successfully",
	}

	response := module.MakeFinalDecision(context.Background(), result)
	if response != "Task completed successfully" {
		t.Errorf("Expected 'Task completed successfully', got '%s'", response)
	}

	// Test failed result with error
	result = TaskResult{
		JobID:   "job_20260301_002",
		Success: false,
		Error:   errors.New("test error"),
	}

	response = module.MakeFinalDecision(context.Background(), result)
	if response != "エラーが発生しました: test error" {
		t.Errorf("Unexpected error message: %s", response)
	}

	// Test failed result without error
	result = TaskResult{
		JobID:   "job_20260301_003",
		Success: false,
	}

	response = module.MakeFinalDecision(context.Background(), result)
	if response != "タスクの実行に失敗しました。" {
		t.Errorf("Unexpected failure message: %s", response)
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
