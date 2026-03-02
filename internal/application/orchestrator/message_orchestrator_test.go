package orchestrator

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// mockSessionRepository はテスト用のSessionRepository
type mockSessionRepository struct {
	sessions map[string]*session.Session
}

func newMockSessionRepository() *mockSessionRepository {
	return &mockSessionRepository{
		sessions: make(map[string]*session.Session),
	}
}

func (m *mockSessionRepository) Save(ctx context.Context, sess *session.Session) error {
	m.sessions[sess.ID()] = sess
	return nil
}

func (m *mockSessionRepository) Load(ctx context.Context, id string) (*session.Session, error) {
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

// mockMioAgent はテスト用のMioAgent
type mockMioAgent struct {
	decision routing.Decision
	response string
}

func (m *mockMioAgent) DecideAction(ctx context.Context, t task.Task) (routing.Decision, error) {
	return m.decision, nil
}

func (m *mockMioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	return m.response, nil
}

// mockShiroAgent はテスト用のShiroAgent
type mockShiroAgent struct {
	response string
}

func (m *mockShiroAgent) Execute(ctx context.Context, t task.Task) (string, error) {
	return m.response, nil
}

// mockCoderAgent はテスト用のCoderAgent
type mockCoderAgent struct {
	response string
}

func (m *mockCoderAgent) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return m.response, nil
}

func TestNewMessageOrchestrator(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 1.0, "test"),
		response: "Hello",
	}
	shiro := &mockShiroAgent{response: "executed"}

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, coder, nil, nil)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, coder3)

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

	orchestrator := NewMessageOrchestrator(repo, mio, shiro, nil, nil, nil)

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
