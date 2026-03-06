package agent

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ChatCommandResult はチャットコマンドの処理結果
type ChatCommandResult struct {
	Handled  bool
	Response string
}

// HandleChatCommand はチャットコマンドを処理する
// コマンドでない場合は Handled=false を返す
func (m *MioAgent) HandleChatCommand(ctx context.Context, sessionID string, message string) (ChatCommandResult, error) {
	cmd, _ := parseChatCommand(message)
	if cmd == "" {
		return ChatCommandResult{Handled: false}, nil
	}

	switch cmd {
	case "status":
		return m.cmdStatus(ctx, sessionID)
	case "stop":
		return ChatCommandResult{
			Handled:  true,
			Response: "現在のリクエストを停止しました。",
		}, nil
	case "compact":
		return m.cmdCompact(ctx, sessionID)
	case "context":
		return m.cmdContext(ctx, sessionID, message)
	case "new":
		return m.cmdNew(ctx, sessionID)
	default:
		return ChatCommandResult{Handled: false}, nil
	}
}

// parseChatCommand はメッセージからチャットコマンドを抽出する
// 戻り値: (コマンド名, 残りのテキスト)
func parseChatCommand(message string) (string, string) {
	trimmed := strings.TrimSpace(message)
	if !strings.HasPrefix(trimmed, "/") {
		return "", ""
	}

	// チャットコマンド一覧（ルーティングコマンドと区別）
	chatCommands := []string{"status", "stop", "compact", "context", "new"}

	parts := strings.SplitN(trimmed, " ", 2)
	cmd := strings.TrimPrefix(parts[0], "/")
	rest := ""
	if len(parts) > 1 {
		rest = parts[1]
	}

	for _, c := range chatCommands {
		if cmd == c {
			return c, rest
		}
	}
	return "", ""
}

// cmdStatus はスレッド情報を表示
func (m *MioAgent) cmdStatus(ctx context.Context, sessionID string) (ChatCommandResult, error) {
	if m.conversationEngine == nil {
		return ChatCommandResult{
			Handled:  true,
			Response: "会話エンジンが無効です。",
		}, nil
	}

	status, err := m.conversationEngine.GetStatus(ctx, sessionID)
	if err != nil {
		return ChatCommandResult{}, fmt.Errorf("GetStatus failed: %w", err)
	}

	elapsed := ""
	if !status.ThreadStart.IsZero() {
		elapsed = time.Since(status.ThreadStart).Truncate(time.Second).String()
	}

	resp := fmt.Sprintf("📊 セッション状態\n"+
		"- セッション: %s\n"+
		"- スレッドID: %d\n"+
		"- ドメイン: %s\n"+
		"- ターン数: %d\n"+
		"- 経過時間: %s\n"+
		"- ステータス: %s",
		status.SessionID,
		status.ThreadID,
		status.ThreadDomain,
		status.TurnCount,
		elapsed,
		status.ThreadStatus,
	)

	return ChatCommandResult{Handled: true, Response: resp}, nil
}

// cmdCompact は現在のスレッドを即座にフラッシュ
func (m *MioAgent) cmdCompact(ctx context.Context, sessionID string) (ChatCommandResult, error) {
	if m.conversationEngine == nil {
		return ChatCommandResult{
			Handled:  true,
			Response: "会話エンジンが無効です。",
		}, nil
	}

	if err := m.conversationEngine.FlushCurrentThread(ctx, sessionID); err != nil {
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("スレッドのフラッシュに失敗しました: %v", err),
		}, nil
	}

	return ChatCommandResult{
		Handled:  true,
		Response: "現在のスレッドをフラッシュし、新しいスレッドを開始しました。",
	}, nil
}

// cmdContext は現在のRecallPackの内容を表示
func (m *MioAgent) cmdContext(ctx context.Context, sessionID string, _ string) (ChatCommandResult, error) {
	if m.conversationEngine == nil {
		return ChatCommandResult{
			Handled:  true,
			Response: "会話エンジンが無効です。",
		}, nil
	}

	pack, err := m.conversationEngine.BeginTurn(ctx, sessionID, "")
	if err != nil {
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("RecallPack取得に失敗: %v", err),
		}, nil
	}

	var sb strings.Builder
	sb.WriteString("📋 現在のコンテキスト\n")

	sb.WriteString(fmt.Sprintf("\n【ペルソナ】%s\n", pack.Persona.Name))

	if len(pack.ShortContext) > 0 {
		sb.WriteString(fmt.Sprintf("\n【短期記憶】%d件\n", len(pack.ShortContext)))
		for _, msg := range pack.ShortContext {
			sb.WriteString(fmt.Sprintf("  - [%s] %s\n", msg.Speaker, truncate(msg.Msg, 60)))
		}
	}

	if len(pack.MidSummaries) > 0 {
		sb.WriteString(fmt.Sprintf("\n【中期記憶】%d件\n", len(pack.MidSummaries)))
		for _, s := range pack.MidSummaries {
			sb.WriteString(fmt.Sprintf("  - %s\n", truncate(s.Summary, 80)))
		}
	}

	if len(pack.LongFacts) > 0 {
		sb.WriteString(fmt.Sprintf("\n【長期記憶】%d件\n", len(pack.LongFacts)))
		for _, f := range pack.LongFacts {
			sb.WriteString(fmt.Sprintf("  - %s\n", truncate(f, 80)))
		}
	}

	return ChatCommandResult{Handled: true, Response: sb.String()}, nil
}

// cmdNew はセッションをリセット
func (m *MioAgent) cmdNew(ctx context.Context, sessionID string) (ChatCommandResult, error) {
	if m.conversationEngine == nil {
		return ChatCommandResult{
			Handled:  true,
			Response: "会話エンジンが無効です。",
		}, nil
	}

	if err := m.conversationEngine.ResetSession(ctx, sessionID); err != nil {
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("セッションリセットに失敗しました: %v", err),
		}, nil
	}

	return ChatCommandResult{
		Handled:  true,
		Response: "セッションをリセットしました。新しい会話を始めましょう！",
	}, nil
}

// truncate は文字列を指定文字数で切り詰める
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
