package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainmemory "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/memory"
)

// ChatCommandResult はチャットコマンドの処理結果
type ChatCommandResult struct {
	Handled  bool
	Response string
}

// HandleChatCommand はチャットコマンドを処理する
// コマンドでない場合は Handled=false を返す
func (m *MioAgent) HandleChatCommand(ctx context.Context, sessionID string, message string) (ChatCommandResult, error) {
	if result, handled, err := m.handleUserMemoryCommand(ctx, sessionID, message); handled || err != nil {
		return result, err
	}

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

func (m *MioAgent) handleUserMemoryCommand(ctx context.Context, sessionID string, message string) (ChatCommandResult, bool, error) {
	if m.userMemoryManager == nil {
		return ChatCommandResult{}, false, nil
	}
	action, body := parseUserMemoryCommand(message)
	if action == "" {
		return ChatCommandResult{}, false, nil
	}
	if body == "" {
		return ChatCommandResult{
			Handled:  true,
			Response: "覚える内容または対象をもう少し具体的に書いてください。",
		}, true, nil
	}

	evidenceID := "chat_memory_command:" + strings.TrimSpace(sessionID)
	if strings.TrimSpace(sessionID) == "" {
		evidenceID = "chat_memory_command:unknown_session"
	}

	switch action {
	case "remember":
		item, err := m.userMemoryManager.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
			UserID:           "ren",
			Type:             domainmemory.UserMemoryTypePreference,
			Statement:        body,
			State:            domainmemory.MemoryStateCandidate,
			EvidenceEventIDs: []string{evidenceID},
			Confidence:       0.7,
			Sensitivity:      "normal",
			Scope:            "global",
			Source:           "user_memory_command",
		})
		if err != nil {
			return ChatCommandResult{}, true, fmt.Errorf("user memory create failed: %w", err)
		}
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("覚える候補に入れました。\n- id: %s\n- 内容: %s", item.ID, item.Statement),
		}, true, nil
	case "prioritize":
		item, err := m.userMemoryManager.CreateUserMemory(ctx, domainmemory.CreateUserMemoryInput{
			UserID:           "ren",
			Type:             domainmemory.UserMemoryTypeConstraint,
			Statement:        body,
			State:            domainmemory.MemoryStatePinned,
			EvidenceEventIDs: []string{evidenceID},
			Confidence:       1.0,
			Sensitivity:      "normal",
			Scope:            "global",
			Source:           "user_explicit_priority",
		})
		if err != nil {
			return ChatCommandResult{}, true, fmt.Errorf("user memory pin failed: %w", err)
		}
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("優先する記憶として固定しました。\n- id: %s\n- 内容: %s", item.ID, item.Statement),
		}, true, nil
	case "forget", "correct":
		item, err := m.findUserMemoryByText(ctx, body)
		if err != nil {
			return ChatCommandResult{}, true, err
		}
		if item == nil {
			return ChatCommandResult{
				Handled:  true,
				Response: "該当する記憶を見つけられませんでした。忘れる対象の文か memory id を指定してください。",
			}, true, nil
		}
		updated, err := m.userMemoryManager.ForgetUserMemory(ctx, item.ID, action)
		if err != nil {
			return ChatCommandResult{}, true, fmt.Errorf("user memory forget failed: %w", err)
		}
		return ChatCommandResult{
			Handled:  true,
			Response: fmt.Sprintf("記憶を無効化しました。\n- id: %s\n- 内容: %s", updated.ID, updated.Statement),
		}, true, nil
	default:
		return ChatCommandResult{}, false, nil
	}
}

func parseUserMemoryCommand(message string) (string, string) {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || strings.HasPrefix(trimmed, "/") {
		return "", ""
	}
	prefixes := []struct {
		prefix string
		action string
	}{
		{"これを優先して", "prioritize"},
		{"優先して", "prioritize"},
		{"覚えて", "remember"},
		{"忘れて", "forget"},
		{"これは違う", "correct"},
	}
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p.prefix) {
			return p.action, cleanupUserMemoryCommandBody(strings.TrimPrefix(trimmed, p.prefix))
		}
	}
	if strings.HasSuffix(trimmed, "を覚えて") {
		return "remember", cleanupUserMemoryCommandBody(strings.TrimSuffix(trimmed, "を覚えて"))
	}
	if strings.HasSuffix(trimmed, "は忘れて") {
		return "forget", cleanupUserMemoryCommandBody(strings.TrimSuffix(trimmed, "は忘れて"))
	}
	return "", ""
}

func cleanupUserMemoryCommandBody(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimLeft(s, " 　:：,，。")
	s = strings.TrimRight(s, " 　。")
	return strings.TrimSpace(s)
}

func (m *MioAgent) findUserMemoryByText(ctx context.Context, query string) (*domainmemory.UserMemory, error) {
	items, err := m.userMemoryManager.ListUserMemories(ctx, "ren", "", false, 50)
	if err != nil {
		return nil, fmt.Errorf("user memory list failed: %w", err)
	}
	query = strings.TrimSpace(query)
	for _, item := range items {
		if item.ID == query {
			found := item
			return &found, nil
		}
		if strings.Contains(item.Statement, query) || strings.Contains(query, item.Statement) {
			found := item
			return &found, nil
		}
	}
	return nil, nil
}

func (m *MioAgent) userMemoryPrompt(ctx context.Context) (string, error) {
	if m.userMemoryManager == nil {
		return "", nil
	}
	items, err := m.userMemoryManager.ListUserMemories(ctx, "ren", "", false, 12)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, item := range items {
		if !item.Active || item.Sensitivity == "sensitive" {
			continue
		}
		if item.State != domainmemory.MemoryStateConfirmed && item.State != domainmemory.MemoryStatePinned {
			continue
		}
		statement := strings.TrimSpace(item.Statement)
		if statement == "" {
			continue
		}
		prefix := "- "
		if item.State == domainmemory.MemoryStatePinned {
			prefix = "- [優先] "
		}
		lines = append(lines, prefix+statement)
	}
	if len(lines) == 0 {
		return "", nil
	}
	return "思い出したこと:\n" + strings.Join(lines, "\n") + "\n注意: user:ren の confirmed/pinned 記憶だけを補助文脈として扱い、Knowledge DB や raw log と混ぜない。", nil
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
