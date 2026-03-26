package orchestrator

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// TestCodeExecutor_CODE1Route はCODE1明示ルートのテスト（RED）
func TestCodeExecutor_CODE1Route(t *testing.T) {
	coder1 := &mockCoderAgent{response: "CODE1 response"}
	executor := NewDefaultCodeExecutor(coder1, nil, nil, nil, nil, nil, noopEventEmitter)

	jobID := task.NewJobID()
	req := CodeExecutionRequest{
		Task:      task.NewTask(jobID, "user message", "test", "chat-1"),
		Route:     routing.RouteCODE1,
		SessionID: "sess-1",
		Channel:   "test",
		ChatID:    "chat-1",
		JobID:     jobID.String(),
	}

	resp, err := executor.ExecuteCode(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.Response != "CODE1 response" {
		t.Errorf("Expected 'CODE1 response', got '%s'", resp.Response)
	}

	if resp.Handled {
		t.Error("CODE1 should not use Proposal path")
	}
}

// TestCodeExecutor_CODE3_WithProposal はCODE3 Proposal実行のテスト（RED）
func TestCodeExecutor_CODE3_WithProposal(t *testing.T) {
	tmpDir := t.TempDir()
	testProposal := proposal.NewProposal(
		"Test plan",
		`[{"type": "file_edit", "action": "create", "target": "`+tmpDir+`/test.txt", "content": "test"}]`,
		"Low risk",
		"Low cost",
	)

	coder3 := &mockCoderAgentWithProposal{
		response: "Proposal generated",
		proposal: testProposal,
	}

	workerService := service.NewWorkerExecutionService(workerConfigForTest(tmpDir))
	executor := NewDefaultCodeExecutor(nil, nil, coder3, nil, workerService, nil, noopEventEmitter)

	jobID := task.NewJobID()
	req := CodeExecutionRequest{
		Task:      task.NewTask(jobID, "user message", "test", "chat-1"),
		Route:     routing.RouteCODE3,
		SessionID: "sess-1",
		Channel:   "test",
		ChatID:    "chat-1",
		JobID:     jobID.String(),
	}

	resp, err := executor.ExecuteCode(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !resp.Handled {
		t.Error("CODE3 with Proposal should be handled via Proposal path")
	}

	// レスポンスにPlanが含まれることを確認
	if resp.Response == "" {
		t.Error("Expected non-empty response")
	}
}

// TestCodeExecutor_CODE_GenericRoute_Fallback はCODE汎用ルートのフォールバックテスト
func TestCodeExecutor_CODE_GenericRoute_Fallback(t *testing.T) {
	// coder1がnilの場合にcoder2にフォールバック
	coder2 := &mockCoderAgent{response: "CODE2 fallback response"}

	executor := NewDefaultCodeExecutor(nil, coder2, nil, nil, nil, nil, noopEventEmitter)

	jobID := task.NewJobID()
	req := CodeExecutionRequest{
		Task:      task.NewTask(jobID, "user message", "test", "chat-1"),
		Route:     routing.RouteCODE,
		SessionID: "sess-1",
		Channel:   "test",
		ChatID:    "chat-1",
		JobID:     jobID.String(),
	}

	resp, err := executor.ExecuteCode(context.Background(), req)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// coder1がnilならcoder2にフォールバック
	if resp.Response != "CODE2 fallback response" {
		t.Errorf("Expected fallback to coder2, got '%s'", resp.Response)
	}
}

// ヘルパー関数
func noopEventEmitter(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	// テスト用の空実装
}

func workerConfigForTest(workspace string) config.WorkerConfig {
	return config.WorkerConfig{
		AutoCommit:        false,
		StopOnError:       false,
		Workspace:         workspace,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "error",
		CommandTimeout:    10,
		GitTimeout:        10,
	}
}
