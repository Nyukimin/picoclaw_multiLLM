package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// LLMClassifier はLLMベースのタスク分類器
type LLMClassifier struct {
	llmProvider  llm.LLMProvider
	systemPrompt string
}

// NewLLMClassifier は新しいLLMClassifierを作成
func NewLLMClassifier(llmProvider llm.LLMProvider, systemPrompt string) *LLMClassifier {
	return &LLMClassifier{
		llmProvider:  llmProvider,
		systemPrompt: systemPrompt,
	}
}

// Classify はタスクを分類
func (c *LLMClassifier) Classify(ctx context.Context, t task.Task) (routing.Decision, error) {
	userMessage := fmt.Sprintf("ユーザーからのメッセージ: %s", t.UserMessage())

	req := llm.GenerateRequest{
		SystemPrompt: c.systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: userMessage},
		},
		MaxTokens:   100,
		Temperature: 0.3, // 低温度で安定した分類
	}

	resp, err := c.llmProvider.Generate(ctx, req)
	if err != nil {
		return routing.Decision{}, fmt.Errorf("LLM classification failed: %w", err)
	}

	// LLM応答からルートを抽出
	route := c.parseRoute(resp.Content)
	confidence := c.calculateConfidence(route, resp.Content)
	reason := fmt.Sprintf("LLM classified as %s", route)

	return routing.NewDecision(route, confidence, reason), nil
}

// parseRoute はLLM応答からルートを抽出
func (c *LLMClassifier) parseRoute(response string) routing.Route {
	// レスポンスをトリムして大文字化
	trimmed := strings.TrimSpace(response)
	upper := strings.ToUpper(trimmed)

	// 長いルート名から順にチェック（CODE3 を CODE より先に判定）
	validRoutes := []struct {
		key   string
		route routing.Route
	}{
		{"CODE3", routing.RouteCODE3},
		{"CODE2", routing.RouteCODE2},
		{"CODE1", routing.RouteCODE1},
		{"CODE", routing.RouteCODE},
		{"ANALYZE", routing.RouteANALYZE},
		{"RESEARCH", routing.RouteRESEARCH},
		{"PLAN", routing.RoutePLAN},
		{"CHAT", routing.RouteCHAT},
		{"OPS", routing.RouteOPS},
	}

	for _, vr := range validRoutes {
		if strings.Contains(upper, vr.key) {
			return vr.route
		}
	}

	// 無効なルート名の場合はCHATにフォールバック
	return routing.RouteCHAT
}

// calculateConfidence は信頼度を計算
func (c *LLMClassifier) calculateConfidence(route routing.Route, response string) float64 {
	// CHATへのフォールバックは低信頼度
	if route == routing.RouteCHAT && !strings.Contains(strings.ToUpper(response), "CHAT") {
		return 0.4
	}

	// 明確なマッチは高信頼度
	return 0.7
}
