package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// MioAgent は Chat（会話・意思決定）を担当するエンティティ
type MioAgent struct {
	llmProvider        llm.LLMProvider
	classifier         Classifier
	ruleDictionary     RuleDictionary
	toolRunner         ToolRunner
	mcpClient          MCPClient
	conversationEngine conversation.ConversationEngine // v5.1: 会話エンジン（nilを許容）
}

// NewMioAgent は新しいMioAgentを作成
func NewMioAgent(
	llmProvider llm.LLMProvider,
	classifier Classifier,
	ruleDictionary RuleDictionary,
	toolRunner ToolRunner,
	mcpClient MCPClient,
	conversationEngine conversation.ConversationEngine, // v5.1: ConversationEngine（nilを許容）
) *MioAgent {
	return &MioAgent{
		llmProvider:        llmProvider,
		classifier:         classifier,
		ruleDictionary:     ruleDictionary,
		toolRunner:         toolRunner,
		mcpClient:          mcpClient,
		conversationEngine: conversationEngine,
	}
}


// DecideAction はMioによる委譲判断（4段階優先順位）
func (m *MioAgent) DecideAction(ctx context.Context, t task.Task) (routing.Decision, error) {
	// 優先度1: 明示コマンド
	if explicitRoute := m.parseExplicitCommand(t.UserMessage()); explicitRoute != "" {
		return routing.NewDecision(explicitRoute, 1.0, "Explicit command"), nil
	}

	// 優先度2: ルール辞書
	if route, confidence, matched := m.ruleDictionary.Match(t); matched {
		return routing.NewDecision(route, confidence, "Rule dictionary match"), nil
	}

	// 優先度3: 安全側フォールバック（CHAT）
	// 技術的キーワードがルール辞書で捕捉されなかったメッセージは会話として処理
	// LLM分類器は精度向上のためのオプション（レイテンシ優先で現在はスキップ）
	return routing.NewDecision(routing.RouteCHAT, 0.7, "No rule match, default to CHAT"), nil
}

// Chat は会話を実行（v5.1: ConversationEngine + キーワードベース自動Web検索）
func (m *MioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	userMessage := t.UserMessage()

	// === v5.1: ConversationEngine による RecallPack 生成 ===
	var messages []llm.Message
	if m.conversationEngine != nil {
		recallPack, err := m.conversationEngine.BeginTurn(ctx, t.ChatID(), userMessage)
		if err != nil {
			fmt.Printf("WARN: BeginTurn failed: %v\n", err)
		}
		if recallPack != nil {
			// RecallPack からプロンプトメッセージを生成（system prompt + 過去文脈 + 会話履歴）
			messages = recallPack.ToPromptMessages()
		}
	}

	// キーワードベースでWeb検索が必要か判定
	searchKeywords := []string{"教えて", "調べて", "検索", "について", "最新", "ニュース", "とは"}
	needsSearch := false
	for _, keyword := range searchKeywords {
		if strings.Contains(userMessage, keyword) {
			needsSearch = true
			break
		}
	}

	// Web検索を実行してコンテキストに追加
	if needsSearch && m.toolRunner != nil {
		searchResult, err := m.executeWebSearch(ctx, userMessage)
		if err == nil && searchResult != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: "以下はWeb検索の結果です。この情報を参考にして質問に答えてください:\n\n" + searchResult,
			})
		}
	}

	// ユーザーメッセージを最後に追加
	messages = append(messages, llm.Message{Role: "user", Content: userMessage})

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   512,
		Temperature: 0.7,
	}

	resp, err := m.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	response := resp.Content

	// === v5.1: EndTurn（Store） ===
	if m.conversationEngine != nil {
		if err := m.conversationEngine.EndTurn(ctx, t.ChatID(), userMessage, response); err != nil {
			fmt.Printf("WARN: EndTurn failed: %v\n", err)
		}
	}

	return response, nil
}

// executeWebSearch はWeb検索を実行（内部ヘルパー）
func (m *MioAgent) executeWebSearch(ctx context.Context, query string) (string, error) {
	if m.toolRunner == nil {
		return "", fmt.Errorf("toolRunner not available")
	}

	// クエリから検索キーワードを抽出（不要な部分を除去）
	cleanedQuery := cleanSearchQuery(query)

	args := map[string]interface{}{
		"query": cleanedQuery,
	}

	result, err := m.toolRunner.Execute(ctx, "web_search", args)
	if err != nil {
		return "", err
	}

	return result, nil
}

// cleanSearchQuery は検索クエリから不要な部分を除去
func cleanSearchQuery(query string) string {
	// 除去するパターン（質問形式の語尾など）
	removePatterns := []string{
		"について教えて", "を教えて", "教えて",
		"について調べて", "を調べて", "調べて",
		"について検索", "を検索", "検索して",
		"とは", "って何", "ってなに",
	}

	cleaned := query
	for _, pattern := range removePatterns {
		cleaned = strings.Replace(cleaned, pattern, "", -1)
	}

	return strings.TrimSpace(cleaned)
}

// parseExplicitCommand は明示コマンドを解析
func (m *MioAgent) parseExplicitCommand(message string) routing.Route {
	// 長いコマンドから順にチェック（/code3 を /code より先に判定）
	commands := []struct {
		cmd   string
		route routing.Route
	}{
		{"/analyze", routing.RouteANALYZE},
		{"/research", routing.RouteRESEARCH},
		{"/code3", routing.RouteCODE3},
		{"/code2", routing.RouteCODE2},
		{"/code1", routing.RouteCODE1},
		{"/code", routing.RouteCODE},
		{"/plan", routing.RoutePLAN},
		{"/chat", routing.RouteCHAT},
		{"/ops", routing.RouteOPS},
	}

	trimmed := strings.TrimSpace(message)
	for _, c := range commands {
		if strings.HasPrefix(trimmed, c.cmd) {
			return c.route
		}
	}

	return ""
}
