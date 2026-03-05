package heartbeat

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// mockChatAgent はテスト用のChatAgentモック
type mockChatAgent struct {
	response string
	err      error
	called   bool
	lastMsg  string
}

func (m *mockChatAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	m.called = true
	m.lastMsg = t.UserMessage()
	return m.response, m.err
}

// mockSender はテスト用のNotificationSenderモック
type mockSender struct {
	messages []string
	err      error
}

func (m *mockSender) SendNotification(ctx context.Context, message string) error {
	m.messages = append(m.messages, message)
	return m.err
}

func TestNewHeartbeatService(t *testing.T) {
	t.Run("minimum interval is 5 minutes", func(t *testing.T) {
		svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, "/tmp", 1)
		if svc.interval != 5*time.Minute {
			t.Errorf("expected 5m, got %v", svc.interval)
		}
	})

	t.Run("normal interval", func(t *testing.T) {
		svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, "/tmp", 30)
		if svc.interval != 30*time.Minute {
			t.Errorf("expected 30m, got %v", svc.interval)
		}
	})
}

func TestTick_HeartbeatOK(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !agent.called {
		t.Error("expected agent.Chat to be called")
	}
	if len(sender.messages) != 0 {
		t.Errorf("expected no notification, got %d", len(sender.messages))
	}

	logData, _ := os.ReadFile(filepath.Join(dir, "heartbeat.log"))
	if !strings.Contains(string(logData), "[OK]") {
		t.Error("expected [OK] in heartbeat.log")
	}
}

func TestTick_Notification(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check alerts"), 0644)

	agent := &mockChatAgent{response: "Disk usage is 95%"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sender.messages) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(sender.messages))
	}
	if sender.messages[0] != "Disk usage is 95%" {
		t.Errorf("expected 'Disk usage is 95%%', got %q", sender.messages[0])
	}
}

func TestTick_NoFile(t *testing.T) {
	dir := t.TempDir()

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.called {
		t.Error("expected agent.Chat NOT to be called when file is missing")
	}
}

func TestTick_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("   \n  "), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if agent.called {
		t.Error("expected agent.Chat NOT to be called for empty file")
	}
}

func TestTick_ChatError(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check"), 0644)

	agent := &mockChatAgent{err: context.DeadlineExceeded}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, dir, 30)

	err := svc.tick(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "chat failed") {
		t.Errorf("expected 'chat failed' error, got: %v", err)
	}
}

func TestTick_NilSender(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check"), 0644)

	agent := &mockChatAgent{response: "Alert: something is wrong"}
	svc := NewHeartbeatService(agent, nil, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartStop(t *testing.T) {
	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	sender := &mockSender{}
	svc := NewHeartbeatService(agent, sender, t.TempDir(), 5)

	svc.Start()
	svc.Start() // 二重起動しないこと

	time.Sleep(50 * time.Millisecond)
	svc.Stop()
	svc.Stop() // 二重停止しないこと
}

func TestContextBuilder_WithWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()

	// workspace ファイル群を作成
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Agent rules here"), 0644)
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("Soul values here"), 0644)
	os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("Identity info"), 0644)
	os.WriteFile(filepath.Join(dir, "USER.md"), []byte("User prefs"), 0644)
	os.WriteFile(filepath.Join(dir, "CHAT_PERSONA.md"), []byte("Mio persona"), 0644)

	// skills
	os.MkdirAll(filepath.Join(dir, "skills", "weather"), 0755)
	os.WriteFile(filepath.Join(dir, "skills", "weather", "SKILL.md"), []byte("# Weather lookup"), 0644)

	svc := NewHeartbeatService(&mockChatAgent{}, &mockSender{}, dir, 30)

	// tick 経由で ContextBuilder が使われることを確認
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)
	agent := svc.chatAgent.(*mockChatAgent)
	agent.response = "HEARTBEAT_OK"

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := agent.lastMsg

	// workspace コンテキストが含まれること（ContextBuilder は CHAT ルートで ChatOnly も含む）
	if !strings.Contains(msg, "# AGENT\nAgent rules here") {
		t.Error("expected AGENT.md content")
	}
	if !strings.Contains(msg, "# SOUL\nSoul values here") {
		t.Error("expected SOUL.md content")
	}
	if !strings.Contains(msg, "# IDENTITY\nIdentity info") {
		t.Error("expected IDENTITY.md content")
	}
	if !strings.Contains(msg, "weather: Weather lookup") {
		t.Error("expected skills summary")
	}

	// HEARTBEAT タスクが末尾にあること
	if !strings.Contains(msg, "# HEARTBEAT TASKS\nCheck system status") {
		t.Error("expected HEARTBEAT TASKS section")
	}

	// コンテキストとタスクの区切り
	if !strings.Contains(msg, "===") {
		t.Error("expected separator between context and tasks")
	}
}

func TestContextBuilder_NoWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check system status"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// workspace ファイルがなければ HEARTBEAT タスクのみ
	if agent.lastMsg != "Check system status" {
		t.Errorf("expected plain heartbeat content, got: %q", agent.lastMsg)
	}
}

func TestTick_WithWorkspaceContext(t *testing.T) {
	dir := t.TempDir()

	// workspace + HEARTBEAT.md
	os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Be concise"), 0644)
	os.WriteFile(filepath.Join(dir, "HEARTBEAT.md"), []byte("Check alerts"), 0644)

	agent := &mockChatAgent{response: "HEARTBEAT_OK"}
	svc := NewHeartbeatService(agent, &mockSender{}, dir, 30)

	err := svc.tick(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// MioAgentに送信されたメッセージにworkspaceコンテキストが含まれること
	if !strings.Contains(agent.lastMsg, "# AGENT\nBe concise") {
		t.Error("expected workspace context in message sent to agent")
	}
	if !strings.Contains(agent.lastMsg, "# HEARTBEAT TASKS\nCheck alerts") {
		t.Error("expected heartbeat tasks in message sent to agent")
	}
}
