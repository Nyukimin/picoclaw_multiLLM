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

// mockCoderAgentWithProposal はProposal生成をサポートするCoderAgent
type mockCoderAgentWithProposal struct {
	response string
	proposal *proposal.Proposal
}

func (m *mockCoderAgentWithProposal) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return m.response, nil
}

func (m *mockCoderAgentWithProposal) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
	return m.proposal, nil
}

func TestMessageOrchestrator_ProcessMessage_CODE3_WithProposal_JSONPatch(t *testing.T) {
	// テスト用ワークスペース作成
	tmpDir := t.TempDir()

	// WorkerExecutionService初期化
	workerConfig := config.WorkerConfig{
		AutoCommit:        false,
		StopOnError:       false,
		Workspace:         tmpDir,
		ProtectedPatterns: []string{".env*"},
		ActionOnProtected: "error",
		CommandTimeout:    10,
		GitTimeout:        10,
	}
	workerService := service.NewWorkerExecutionService(workerConfig)

	// Proposal生成（JSON形式のPatch）
	jsonPatch := `[
		{
			"type": "file_edit",
			"action": "create",
			"target": "` + tmpDir + `/test.txt",
			"content": "Hello, CODE3!"
		}
	]`

	testProposal := proposal.NewProposal(
		"Test plan: Create test.txt file",
		jsonPatch,
		"Low risk",
		"Low cost",
	)

	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "Explicit CODE3 command"),
		response: "chat response",
	}
	shiro := &mockShiroAgent{response: "executed"}
	coder3 := &mockCoderAgentWithProposal{
		response: "Proposal generated",
		proposal: testProposal,
	}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, workerService)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 test.txtを作成して",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Route != routing.RouteCODE3 {
		t.Errorf("Expected route CODE3, got '%s'", resp.Route)
	}

	// レスポンスにPlan、Execution Result、Riskが含まれているか確認
	if resp.Response == "" {
		t.Error("Response should not be empty")
	}

	// レスポンスフォーマット検証
	expected := []string{"## Plan", "## Execution Result", "## Risk", "Status"}
	for _, keyword := range expected {
		if !contains(resp.Response, keyword) {
			t.Errorf("Response should contain '%s', got: %s", keyword, resp.Response)
		}
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE3_WithProposal_MarkdownPatch(t *testing.T) {
	tmpDir := t.TempDir()

	workerConfig := config.WorkerConfig{
		AutoCommit:  false,
		StopOnError: false,
		Workspace:   tmpDir,
		CommandTimeout: 10,
		GitTimeout: 10,
	}
	workerService := service.NewWorkerExecutionService(workerConfig)

	// Markdown形式のPatch
	markdownPatch := "```go:" + tmpDir + "/hello.go\npackage main\n\nfunc Hello() string {\n\treturn \"Hello\"\n}\n```"

	testProposal := proposal.NewProposal(
		"Test plan: Create hello.go",
		markdownPatch,
		"Low risk",
		"Low cost",
	)

	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3"),
		response: "chat response",
	}
	shiro := &mockShiroAgent{response: "executed"}
	coder3 := &mockCoderAgentWithProposal{
		proposal: testProposal,
	}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, workerService)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 hello.goを実装して",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// レスポンス検証
	if !contains(resp.Response, "## Plan") {
		t.Error("Response should contain Plan section")
	}

	if !contains(resp.Response, "✅") && !contains(resp.Response, "⚠️") {
		t.Error("Response should contain status emoji")
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE3_InvalidProposal(t *testing.T) {
	tmpDir := t.TempDir()

	workerConfig := config.WorkerConfig{
		Workspace: tmpDir,
		CommandTimeout: 10,
		GitTimeout: 10,
	}
	workerService := service.NewWorkerExecutionService(workerConfig)

	// 無効なProposal（Patchが空）
	invalidProposal := proposal.NewProposal("", "", "", "")

	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3"),
	}
	shiro := &mockShiroAgent{}
	coder3 := &mockCoderAgentWithProposal{
		proposal: invalidProposal,
	}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, workerService)

	req := ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 テスト",
	}

	_, err := orchestrator.ProcessMessage(context.Background(), req)
	if err == nil {
		t.Error("Expected error for invalid proposal, but got nil")
	}

	if !contains(err.Error(), "invalid proposal") {
		t.Errorf("Expected 'invalid proposal' error, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE3_NoCoder3Available(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3"),
	}
	shiro := &mockShiroAgent{}

	// coder3 = nil
	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 テスト",
	}

	_, err := orchestrator.ProcessMessage(context.Background(), req)
	if err == nil {
		t.Error("Expected error when coder3 is not available")
	}

	if !contains(err.Error(), "no coder3 available") {
		t.Errorf("Expected 'no coder3 available' error, got: %v", err)
	}
}

func TestFormatExecutionResult_SuccessWithGitCommit(t *testing.T) {
	tmpDir := t.TempDir()

	workerConfig := config.WorkerConfig{
		AutoCommit:           false, // Git repo not initialized in test
		CommitMessagePrefix:  "[Test]",
		Workspace:            tmpDir,
		CommandTimeout:       10,
		GitTimeout:           10,
	}
	workerService := service.NewWorkerExecutionService(workerConfig)

	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3"),
	}
	shiro := &mockShiroAgent{}

	jsonPatch := `[{"type": "file_edit", "action": "create", "target": "` + tmpDir + `/test.txt", "content": "Test"}]`
	testProposal := proposal.NewProposal("Test plan", jsonPatch, "Low", "Low")

	coder3 := &mockCoderAgentWithProposal{proposal: testProposal}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, workerService)

	req := ProcessMessageRequest{
		SessionID:   "test",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 test",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// Git Commit行が含まれているか（auto-commitがfalseなのでno-changesまたは含まれない）
	// ここでは実行結果のフォーマットを検証
	if !contains(resp.Response, "Success Rate") {
		t.Error("Response should contain 'Success Rate'")
	}

	if !contains(resp.Response, "Executed") {
		t.Error("Response should contain 'Executed' count")
	}
}

func TestFormatExecutionResult_PartialFailure(t *testing.T) {
	tmpDir := t.TempDir()

	workerConfig := config.WorkerConfig{
		StopOnError: false, // 継続モード
		Workspace:   tmpDir,
		CommandTimeout: 10,
		GitTimeout: 10,
	}
	workerService := service.NewWorkerExecutionService(workerConfig)

	// 最初は成功、2番目は失敗するPatch
	jsonPatch := `[
		{"type": "file_edit", "action": "create", "target": "` + tmpDir + `/ok.txt", "content": "OK"},
		{"type": "file_edit", "action": "delete", "target": "/nonexistent/file.txt"}
	]`

	testProposal := proposal.NewProposal("Test plan", jsonPatch, "Medium", "Low")

	repo := newMockSessionRepository()
	mio := &mockMioAgent{decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3")}
	shiro := &mockShiroAgent{}
	coder3 := &mockCoderAgentWithProposal{proposal: testProposal}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, workerService)

	req := ProcessMessageRequest{
		SessionID:   "test",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 test",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// 部分的失敗を示す警告絵文字が含まれているはず
	if !contains(resp.Response, "⚠️") && !contains(resp.Response, "❌") {
		t.Error("Response should contain warning or error emoji for partial failure")
	}

	// Failed countが記録されているはず
	if !contains(resp.Response, "Failed") {
		t.Error("Response should contain 'Failed' count")
	}
}

// contains はヘルパー関数
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(findInString(s, substr) >= 0))
}

func findInString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
