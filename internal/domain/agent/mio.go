package agent

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/internal/domain/llm"
	"github.com/sipeed/picoclaw/internal/domain/routing"
	"github.com/sipeed/picoclaw/internal/domain/task"
)

// MioAgent は Chat（会話・意思決定）を担当するエンティティ
type MioAgent struct {
	llmProvider    llm.LLMProvider
	classifier     Classifier
	ruleDictionary RuleDictionary
}

// NewMioAgent は新しいMioAgentを作成
func NewMioAgent(
	llmProvider llm.LLMProvider,
	classifier Classifier,
	ruleDictionary RuleDictionary,
) *MioAgent {
	return &MioAgent{
		llmProvider:    llmProvider,
		classifier:     classifier,
		ruleDictionary: ruleDictionary,
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

	// 優先度3: 分類器（LLM）
	decision, err := m.classifier.Classify(ctx, t)
	if err != nil {
		// 優先度4: 安全側フォールバック
		return routing.NewDecision(routing.RouteCHAT, 0.5, "Classifier failed, fallback to CHAT"), nil
	}

	return decision, nil
}

// Chat は会話を実行
func (m *MioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "user", Content: t.UserMessage()},
		},
		MaxTokens:   2048,
		Temperature: 0.7,
	}

	resp, err := m.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// parseExplicitCommand は明示コマンドを解析
func (m *MioAgent) parseExplicitCommand(message string) routing.Route {
	commands := map[string]routing.Route{
		"/chat":     routing.RouteCHAT,
		"/plan":     routing.RoutePLAN,
		"/analyze":  routing.RouteANALYZE,
		"/ops":      routing.RouteOPS,
		"/research": routing.RouteRESEARCH,
		"/code":     routing.RouteCODE,
		"/code1":    routing.RouteCODE1,
		"/code2":    routing.RouteCODE2,
		"/code3":    routing.RouteCODE3,
	}

	trimmed := strings.TrimSpace(message)
	for cmd, route := range commands {
		if strings.HasPrefix(trimmed, cmd) {
			return route
		}
	}

	return ""
}
