package agent

import (
	"context"
	"strings"
	"testing"

	domainmemory "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/memory"
)

type mockUserMemoryManager struct {
	createInput  domainmemory.CreateUserMemoryInput
	listItems    []domainmemory.UserMemory
	forgetID     string
	forgetReason string
}

func (m *mockUserMemoryManager) CreateUserMemory(_ context.Context, input domainmemory.CreateUserMemoryInput) (*domainmemory.UserMemory, error) {
	m.createInput = input
	return &domainmemory.UserMemory{
		ID:               "mem-1",
		Namespace:        "user:" + input.UserID,
		UserID:           input.UserID,
		Type:             input.Type,
		Statement:        strings.TrimSpace(input.Statement),
		EvidenceEventIDs: input.EvidenceEventIDs,
		Confidence:       input.Confidence,
		Sensitivity:      input.Sensitivity,
		State:            input.State,
		Active:           true,
	}, nil
}

func (m *mockUserMemoryManager) ListUserMemories(_ context.Context, _ string, _ string, _ bool, _ int) ([]domainmemory.UserMemory, error) {
	return append([]domainmemory.UserMemory(nil), m.listItems...), nil
}

func (m *mockUserMemoryManager) UpdateUserMemoryState(context.Context, string, string, string) (*domainmemory.UserMemory, error) {
	return nil, nil
}

func (m *mockUserMemoryManager) ForgetUserMemory(_ context.Context, id string, reason string) (*domainmemory.UserMemory, error) {
	m.forgetID = id
	m.forgetReason = reason
	return &domainmemory.UserMemory{ID: id, Namespace: "user:ren", UserID: "ren", Statement: "短く答える", Active: false}, nil
}

func TestParseChatCommand(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantArg string
	}{
		{"/status", "status", ""},
		{"/stop", "stop", ""},
		{"/compact", "compact", ""},
		{"/context", "context", ""},
		{"/new", "new", ""},
		{"/status extra", "status", "extra"},
		{"/code something", "", ""},   // ルーティングコマンドはチャットコマンドではない
		{"/code3 something", "", ""},  // 同上
		{"hello", "", ""},             // コマンドではない
		{"", "", ""},                  // 空文字列
		{"/unknown", "", ""},          // 未知のコマンド
		{"  /status  ", "status", ""}, // 空白あり
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cmd, arg := parseChatCommand(tt.input)
			if cmd != tt.wantCmd {
				t.Errorf("parseChatCommand(%q) cmd = %q, want %q", tt.input, cmd, tt.wantCmd)
			}
			if arg != tt.wantArg {
				t.Errorf("parseChatCommand(%q) arg = %q, want %q", tt.input, arg, tt.wantArg)
			}
		})
	}
}

func TestHandleChatCommand_NoEngine(t *testing.T) {
	// conversationEngine が nil の場合
	m := &MioAgent{}

	tests := []string{"/status", "/compact", "/context", "/new"}
	for _, cmd := range tests {
		t.Run(cmd, func(t *testing.T) {
			result, err := m.HandleChatCommand(nil, "session1", cmd)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Handled {
				t.Error("expected Handled=true")
			}
			if result.Response != "会話エンジンが無効です。" {
				t.Errorf("unexpected response: %s", result.Response)
			}
		})
	}
}

func TestHandleChatCommand_Stop(t *testing.T) {
	m := &MioAgent{}
	result, err := m.HandleChatCommand(nil, "session1", "/stop")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
	if result.Response == "" {
		t.Error("expected non-empty response for /stop")
	}
}

func TestHandleChatCommand_NotCommand(t *testing.T) {
	m := &MioAgent{}
	result, err := m.HandleChatCommand(nil, "session1", "こんにちは")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handled {
		t.Error("expected Handled=false for normal message")
	}
}

func TestHandleChatCommand_RoutingCommand(t *testing.T) {
	m := &MioAgent{}
	result, err := m.HandleChatCommand(nil, "session1", "/code fix bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handled {
		t.Error("expected Handled=false for routing command /code")
	}
}

func TestHandleChatCommand_UserMemoryRemember(t *testing.T) {
	mem := &mockUserMemoryManager{}
	m := (&MioAgent{}).WithUserMemoryManager(mem)

	result, err := m.HandleChatCommand(context.Background(), "session1", "覚えて: 短く論理的な説明を好む")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled || !strings.Contains(result.Response, "覚える候補") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if mem.createInput.UserID != "ren" ||
		mem.createInput.State != domainmemory.MemoryStateCandidate ||
		mem.createInput.Type != domainmemory.UserMemoryTypePreference ||
		mem.createInput.Statement != "短く論理的な説明を好む" ||
		len(mem.createInput.EvidenceEventIDs) != 1 {
		t.Fatalf("unexpected create input: %+v", mem.createInput)
	}
}

func TestHandleChatCommand_UserMemoryPrioritize(t *testing.T) {
	mem := &mockUserMemoryManager{}
	m := (&MioAgent{}).WithUserMemoryManager(mem)

	result, err := m.HandleChatCommand(context.Background(), "session1", "これを優先して 日本語で答える")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled || !strings.Contains(result.Response, "固定") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if mem.createInput.State != domainmemory.MemoryStatePinned ||
		mem.createInput.Type != domainmemory.UserMemoryTypeConstraint ||
		mem.createInput.Source != "user_explicit_priority" {
		t.Fatalf("unexpected priority input: %+v", mem.createInput)
	}
}

func TestHandleChatCommand_UserMemoryForget(t *testing.T) {
	mem := &mockUserMemoryManager{
		listItems: []domainmemory.UserMemory{{
			ID:        "mem-1",
			Namespace: "user:ren",
			UserID:    "ren",
			Statement: "短く答える",
			State:     domainmemory.MemoryStateConfirmed,
			Active:    true,
		}},
	}
	m := (&MioAgent{}).WithUserMemoryManager(mem)

	result, err := m.HandleChatCommand(context.Background(), "session1", "忘れて 短く答える")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled || !strings.Contains(result.Response, "無効化") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if mem.forgetID != "mem-1" || mem.forgetReason != "forget" {
		t.Fatalf("unexpected forget args: %+v", mem)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 3, "hel..."},
		{"こんにちは世界", 4, "こんにち..."},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}
