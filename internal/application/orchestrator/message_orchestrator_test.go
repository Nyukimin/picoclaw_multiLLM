package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// mockSessionRepository はテスト用のSessionRepository（エラー注入対応）
type mockSessionRepository struct {
	sessions  map[string]*session.Session
	loadErr   error // non-nil ならLoad時にこのエラーを返す
	saveErr   error // non-nil ならSave時にこのエラーを返す
}

func newMockSessionRepository() *mockSessionRepository {
	return &mockSessionRepository{
		sessions: make(map[string]*session.Session),
	}
}

func (m *mockSessionRepository) Save(ctx context.Context, sess *session.Session) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.sessions[sess.ID()] = sess
	return nil
}

func (m *mockSessionRepository) Load(ctx context.Context, id string) (*session.Session, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	sess, exists := m.sessions[id]
	if !exists {
		return nil, session.ErrSessionNotFound
	}
	return sess, nil
}

func (m *mockSessionRepository) Exists(ctx context.Context, id string) (bool, error) {
	_, exists := m.sessions[id]
	return exists, nil
}

func (m *mockSessionRepository) Delete(ctx context.Context, id string) error {
	delete(m.sessions, id)
	return nil
}

// mockMioAgent はテスト用のMioAgent（function pointer でエラー注入可能）
type mockMioAgent struct {
	decision   routing.Decision
	response   string
	decideFunc func(ctx context.Context, t task.Task) (routing.Decision, error)
	chatFunc   func(ctx context.Context, t task.Task) (string, error)
	cmdFunc    func(ctx context.Context, sessionID string, message string) (agent.ChatCommandResult, error)
}

func (m *mockMioAgent) DecideAction(ctx context.Context, t task.Task) (routing.Decision, error) {
	if m.decideFunc != nil {
		return m.decideFunc(ctx, t)
	}
	return m.decision, nil
}

func (m *mockMioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	if m.chatFunc != nil {
		return m.chatFunc(ctx, t)
	}
	return m.response, nil
}

func (m *mockMioAgent) HandleChatCommand(ctx context.Context, sessionID string, message string) (agent.ChatCommandResult, error) {
	if m.cmdFunc != nil {
		return m.cmdFunc(ctx, sessionID, message)
	}
	return agent.ChatCommandResult{Handled: false}, nil
}

// mockShiroAgent はテスト用のShiroAgent
type mockShiroAgent struct {
	response    string
	executeFunc func(ctx context.Context, t task.Task) (string, error)
}

func (m *mockShiroAgent) Execute(ctx context.Context, t task.Task) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, t)
	}
	return m.response, nil
}

// mockCoderAgent はテスト用のCoderAgent
type mockCoderAgent struct {
	response string
}

func (m *mockCoderAgent) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return m.response, nil
}

// mockWorkerExecutionService はテスト用のWorkerExecutionService
type mockWorkerExecutionService struct{}

func (m *mockWorkerExecutionService) ExecuteProposal(ctx context.Context, jobID task.JobID, p interface{}) (interface{}, error) {
	return nil, nil
}

func TestNewMessageOrchestrator(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "test"),
		response: "Hello",
	}
	shiro := &mockShiroAgent{response: "executed"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	if orchestrator == nil {
		t.Fatal("NewMessageOrchestrator should not return nil")
	}
}

func TestMessageOrchestrator_ProcessMessage_NewSession(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat route"),
		response: "こんにちは！",
	}
	shiro := &mockShiroAgent{response: "executed"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "こんにちは",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Response != "こんにちは！" {
		t.Errorf("Expected response 'こんにちは！', got '%s'", resp.Response)
	}

	if resp.Route != routing.RouteCHAT {
		t.Errorf("Expected route CHAT, got '%s'", resp.Route)
	}

	// セッションが保存されているか確認
	exists, _ := repo.Exists(context.Background(), "20260302-line-U123")
	if !exists {
		t.Error("Session should be saved")
	}
}

func TestMessageOrchestrator_ProcessMessage_ExistingSession(t *testing.T) {
	repo := newMockSessionRepository()

	// 既存セッションを作成
	existingSession := session.NewSession("20260302-line-U123", "line", "U123")
	repo.Save(context.Background(), existingSession)

	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat route"),
		response: "2回目の応答",
	}
	shiro := &mockShiroAgent{response: "executed"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "2回目のメッセージ",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// セッションに履歴が追加されているか確認
	loadedSession, _ := repo.Load(context.Background(), "20260302-line-U123")
	if loadedSession.HistoryCount() != 1 {
		t.Errorf("Expected 1 task in history, got %d", loadedSession.HistoryCount())
	}

	if resp.Response != "2回目の応答" {
		t.Errorf("Expected response '2回目の応答', got '%s'", resp.Response)
	}
}

func TestMessageOrchestrator_ProcessMessage_OPSRoute(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteOPS, 0.9, "OPS route"),
		response: "chat response",
	}
	shiro := &mockShiroAgent{response: "Command executed successfully"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "ls -la",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Route != routing.RouteOPS {
		t.Errorf("Expected route OPS, got '%s'", resp.Route)
	}

	if resp.Response != "Command executed successfully" {
		t.Errorf("Expected Shiro response, got '%s'", resp.Response)
	}
}

func TestMessageOrchestrator_ProcessMessage_CODERoute(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE, 0.85, "CODE route"),
		response: "chat response",
	}
	shiro := &mockShiroAgent{response: "executed"}
	coder := &mockCoderAgent{response: "// Generated code\nfunc main() {}\n"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, coder, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "main関数を実装して",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Route != routing.RouteCODE {
		t.Errorf("Expected route CODE, got '%s'", resp.Route)
	}

	if resp.Response != "// Generated code\nfunc main() {}\n" {
		t.Errorf("Expected Coder response, got '%s'", resp.Response)
	}
}

func TestMessageOrchestrator_ProcessMessage_ExplicitCommand(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "Explicit command"),
		response: "chat response",
	}
	shiro := &mockShiroAgent{response: "executed"}
	coder3 := &mockCoderAgent{response: "High quality code review"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "/code3 このコードをレビューして",
	}

	resp, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Route != routing.RouteCODE3 {
		t.Errorf("Expected route CODE3, got '%s'", resp.Route)
	}

	if resp.Response != "High quality code review" {
		t.Errorf("Expected Coder3 response, got '%s'", resp.Response)
	}
}

func TestMessageOrchestrator_ProcessMessage_TaskAddedToHistory(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat"),
		response: "応答",
	}
	shiro := &mockShiroAgent{response: "executed"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil, nil)

	req := ProcessMessageRequest{
		SessionID:   "20260302-line-U123",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "テスト",
	}

	_, err := orchestrator.ProcessMessage(context.Background(), req)
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	// セッションをロードして履歴確認
	sess, _ := repo.Load(context.Background(), "20260302-line-U123")
	if sess.HistoryCount() != 1 {
		t.Errorf("Expected 1 task in history, got %d", sess.HistoryCount())
	}

	history := sess.GetHistory()
	if history[0].UserMessage() != "テスト" {
		t.Errorf("Expected user message 'テスト', got '%s'", history[0].UserMessage())
	}

	if history[0].Route() != routing.RouteCHAT {
		t.Errorf("Expected task route CHAT, got '%s'", history[0].Route())
	}
}

// === Phase 1D: Error path tests ===

func defaultReq() ProcessMessageRequest {
	return ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "test message",
	}
}

func TestMessageOrchestrator_ProcessMessage_SessionLoadError(t *testing.T) {
	repo := newMockSessionRepository()
	repo.loadErr = fmt.Errorf("database connection failed")

	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat"),
		response: "hello",
	}

	orch := NewMessageOrchestrator(repo, mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for session load failure")
	}
	if !strings.Contains(err.Error(), "database connection failed") {
		t.Errorf("error should contain root cause, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_RoutingError(t *testing.T) {
	mio := &mockMioAgent{
		decideFunc: func(ctx context.Context, t task.Task) (routing.Decision, error) {
			return routing.Decision{}, fmt.Errorf("LLM classifier timeout")
		},
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for routing failure")
	}
	if !strings.Contains(err.Error(), "routing decision failed") {
		t.Errorf("error should mention routing, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_ChatError(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat"),
		chatFunc: func(ctx context.Context, t task.Task) (string, error) {
			return "", fmt.Errorf("Ollama unavailable")
		},
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for chat failure")
	}
	if !strings.Contains(err.Error(), "Ollama unavailable") {
		t.Errorf("error should contain root cause, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_ShiroError(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteOPS, 0.9, "OPS"),
	}
	shiro := &mockShiroAgent{
		executeFunc: func(ctx context.Context, t task.Task) (string, error) {
			return "", fmt.Errorf("command execution failed")
		},
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, shiro, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for shiro failure")
	}
	if !strings.Contains(err.Error(), "command execution failed") {
		t.Errorf("error should contain root cause, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE1_NoCoder(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE1, 1.0, "CODE1"),
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for CODE1 with no coder")
	}
	if !strings.Contains(err.Error(), "no coder1 available") {
		t.Errorf("error should mention coder unavailability, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE2_NoCoder(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE2, 1.0, "CODE2"),
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for CODE2 with no coder")
	}
	if !strings.Contains(err.Error(), "no coder2 available") {
		t.Errorf("error should mention coder unavailability, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_CODE3_NoCoder(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "CODE3"),
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for CODE3 with no coder")
	}
	if !strings.Contains(err.Error(), "no coder3 available") {
		t.Errorf("error should mention coder unavailability, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_PLAN_FallbackToChat(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RoutePLAN, 0.8, "PLAN"),
		response: "plan response via chat",
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	resp, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if resp.Route != routing.RoutePLAN {
		t.Errorf("route: want PLAN, got %s", resp.Route)
	}
	if resp.Response != "plan response via chat" {
		t.Errorf("response: want 'plan response via chat', got %q", resp.Response)
	}
}

func TestMessageOrchestrator_ProcessMessage_ANALYZE_FallbackToChat(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteANALYZE, 0.8, "ANALYZE"),
		response: "analysis via chat",
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	resp, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if resp.Route != routing.RouteANALYZE {
		t.Errorf("route: want ANALYZE, got %s", resp.Route)
	}
}

func TestMessageOrchestrator_ProcessMessage_RESEARCH_FallbackToChat(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteRESEARCH, 0.8, "RESEARCH"),
		response: "research via chat",
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	resp, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if resp.Route != routing.RouteRESEARCH {
		t.Errorf("route: want RESEARCH, got %s", resp.Route)
	}
}

func TestMessageOrchestrator_ProcessMessage_UnknownRoute(t *testing.T) {
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.Route("UNKNOWN"), 0.5, "unknown"),
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for unknown route")
	}
	if !strings.Contains(err.Error(), "unknown route") {
		t.Errorf("error should mention unknown route, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_ChatCommand_Handled(t *testing.T) {
	mio := &mockMioAgent{
		cmdFunc: func(ctx context.Context, sessionID, message string) (agent.ChatCommandResult, error) {
			return agent.ChatCommandResult{Handled: true, Response: "status output"}, nil
		},
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	resp, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if resp.Response != "status output" {
		t.Errorf("response: want 'status output', got %q", resp.Response)
	}
	if resp.Route != routing.RouteCHAT {
		t.Errorf("route for handled command should be CHAT, got %s", resp.Route)
	}
}

func TestMessageOrchestrator_ProcessMessage_ChatCommand_Error(t *testing.T) {
	mio := &mockMioAgent{
		cmdFunc: func(ctx context.Context, sessionID, message string) (agent.ChatCommandResult, error) {
			return agent.ChatCommandResult{}, fmt.Errorf("command processing failed")
		},
	}

	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for command failure")
	}
	if !strings.Contains(err.Error(), "chat command failed") {
		t.Errorf("error should mention chat command, got: %v", err)
	}
}

func TestMessageOrchestrator_ProcessMessage_SessionSaveError(t *testing.T) {
	repo := newMockSessionRepository()
	repo.saveErr = fmt.Errorf("disk full")

	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "Chat"),
		response: "hello",
	}

	orch := NewMessageOrchestrator(repo, mio, &mockShiroAgent{}, nil, nil, nil, nil)
	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("expected error for save failure")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error should contain root cause, got: %v", err)
	}
}
