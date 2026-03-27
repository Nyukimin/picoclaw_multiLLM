package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/capability"
)

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
		{"/code something", "", ""},      // ルーティングコマンドはチャットコマンドではない
		{"/code3 something", "", ""},      // 同上
		{"hello", "", ""},                 // コマンドではない
		{"", "", ""},                      // 空文字列
		{"/unknown", "", ""},              // 未知のコマンド
		{"  /status  ", "status", ""},     // 空白あり
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

// --- /approve-tool テスト ---

type mockToolRegistry struct {
	entries map[string]capability.ToolEntry
}

func (r *mockToolRegistry) Register(_ context.Context, entry capability.ToolEntry) error {
	r.entries[entry.Name] = entry
	return nil
}

func (r *mockToolRegistry) Approve(_ context.Context, name string) error {
	e, ok := r.entries[name]
	if !ok {
		return fmt.Errorf("not found: %s", name)
	}
	e.Trusted = true
	r.entries[name] = e
	return nil
}

func (r *mockToolRegistry) ListForPlatform(_ context.Context, _ string) ([]capability.ToolEntry, error) {
	return nil, nil
}

func (r *mockToolRegistry) Get(_ context.Context, name string) (capability.ToolEntry, error) {
	e, ok := r.entries[name]
	if !ok {
		return capability.ToolEntry{}, fmt.Errorf("not found: %s", name)
	}
	return e, nil
}

func (r *mockToolRegistry) Close() error { return nil }

func TestCmdApproveTool_NoRegistry(t *testing.T) {
	m := &MioAgent{}
	result, err := m.cmdApproveTool(context.Background(), "my_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
	if result.Response != "ToolRegistry is not enabled." {
		t.Errorf("unexpected response: %q", result.Response)
	}
}

func TestCmdApproveTool_EmptyName(t *testing.T) {
	m := &MioAgent{toolRegistry: &mockToolRegistry{entries: map[string]capability.ToolEntry{}}}
	result, err := m.cmdApproveTool(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response != "Usage: /approve-tool <tool_name>" {
		t.Errorf("unexpected response: %q", result.Response)
	}
}

func TestCmdApproveTool_NotFound(t *testing.T) {
	m := &MioAgent{toolRegistry: &mockToolRegistry{entries: map[string]capability.ToolEntry{}}}
	result, err := m.cmdApproveTool(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
	// 「見つかりません」を含むはず
	if !strings.Contains(result.Response, "nonexistent") {
		t.Errorf("expected tool name in response, got %q", result.Response)
	}
}

func TestCmdApproveTool_AlreadyTrusted(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{
		"my_tool": {Name: "my_tool", Trusted: true},
	}}
	m := &MioAgent{toolRegistry: reg}
	result, err := m.cmdApproveTool(context.Background(), "my_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Response, "既に承認済み") {
		t.Errorf("expected 'already approved' message, got %q", result.Response)
	}
}

func TestCmdApproveTool_Success(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{
		"my_tool": {Name: "my_tool", Trusted: false},
	}}
	m := &MioAgent{toolRegistry: reg}
	result, err := m.cmdApproveTool(context.Background(), "my_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled {
		t.Error("expected Handled=true")
	}
	if !strings.Contains(result.Response, "承認しました") {
		t.Errorf("expected success message, got %q", result.Response)
	}
	// registry に反映されているか確認
	entry, _ := reg.Get(context.Background(), "my_tool")
	if !entry.Trusted {
		t.Error("expected Trusted=true after approve")
	}
}

func TestHandleChatCommand_ApproveTool(t *testing.T) {
	reg := &mockToolRegistry{entries: map[string]capability.ToolEntry{
		"my_tool": {Name: "my_tool", Trusted: false},
	}}
	m := &MioAgent{toolRegistry: reg}
	result, err := m.HandleChatCommand(context.Background(), "session1", "/approve-tool my_tool")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Handled {
		t.Error("expected Handled=true")
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
