package agent

import (
	"testing"
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
