package agent

import (
	"context"
	"log"
	"regexp"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// CoderAgent は Coder（設計・実装）を担当するエンティティ
type CoderAgent struct {
	llmProvider    llm.LLMProvider
	toolRunner     ToolRunner
	mcpClient      MCPClient
	proposalPrompt string
}

// NewCoderAgent は新しいCoderAgentを作成
func NewCoderAgent(
	llmProvider llm.LLMProvider,
	toolRunner ToolRunner,
	mcpClient MCPClient,
	proposalPrompt string,
) *CoderAgent {
	return &CoderAgent{
		llmProvider:    llmProvider,
		toolRunner:     toolRunner,
		mcpClient:      mcpClient,
		proposalPrompt: proposalPrompt,
	}
}

// GenerateProposal はplan/patchを生成
func (c *CoderAgent) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
	log.Printf("[CoderAgent] GenerateProposal start: msg_len=%d", len(t.UserMessage()))
	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: c.proposalPrompt,
			},
			{
				Role:    "user",
				Content: t.UserMessage(),
			},
		},
		MaxTokens:   8192,
		Temperature: 0.5,
	}

	resp, err := c.llmProvider.Generate(ctx, req)
	if err != nil {
		log.Printf("[CoderAgent] GenerateProposal LLM error: %v", err)
		return nil, err
	}
	log.Printf("[CoderAgent] GenerateProposal raw response: len=%d tokens=%d finish=%s",
		len(resp.Content), resp.TokensUsed, resp.FinishReason)

	// レスポンスからProposalを抽出
	p := c.extractProposal(resp.Content)
	if p == nil {
		log.Printf("[CoderAgent] GenerateProposal parse failed: preview=%q", preview(resp.Content, 240))
		return nil, nil
	}
	log.Printf("[CoderAgent] GenerateProposal success: plan_len=%d patch_len=%d risk_len=%d cost_len=%d patch_preview=%q",
		len(p.Plan()), len(p.Patch()), len(p.Risk()), len(p.CostHint()), preview(p.Patch(), 240))
	return p, nil
}

// GenerateWithPrompt は指定されたシステムプロンプトでLLM応答を生成
func (c *CoderAgent) GenerateWithPrompt(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: t.UserMessage()},
		},
		MaxTokens:   8192,
		Temperature: 0.5,
	}

	resp, err := c.llmProvider.Generate(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// extractProposal はLLM応答からProposalを抽出
func (c *CoderAgent) extractProposal(content string) *proposal.Proposal {
	plan := c.extractSection(content, "## Plan", "##")
	patch := c.extractSection(content, "## Patch", "##")
	risk := c.extractSection(content, "## Risk", "##")
	costHint := c.extractSection(content, "## CostHint", "##")

	if patch == "" {
		patch = extractJSONPatch(content)
	}
	if plan == "" && looksLikeJSONPatch(patch) {
		plan = "Auto-generated plan: execute extracted patch commands."
	}

	if plan == "" || patch == "" {
		return nil
	}

	return proposal.NewProposal(plan, patch, risk, costHint)
}

// extractSection はコンテンツからセクションを抽出
func (c *CoderAgent) extractSection(content, startMarker, endMarker string) string {
	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return ""
	}

	// セクション開始位置（マーカーの次の行）
	startIdx += len(startMarker)
	if startIdx >= len(content) {
		return ""
	}

	// 次のセクションマーカーを探す
	remaining := content[startIdx:]
	endIdx := strings.Index(remaining, endMarker)
	if endIdx == -1 {
		// 次のセクションがない場合は末尾まで
		return strings.TrimSpace(remaining)
	}

	return strings.TrimSpace(remaining[:endIdx])
}

func extractJSONPatch(content string) string {
	reFence := regexp.MustCompile("(?s)```json\\s*(\\[.*?\\])\\s*```")
	if m := reFence.FindStringSubmatch(content); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	reArray := regexp.MustCompile("(?s)(\\[\\s*\\{.*\\}\\s*\\])")
	if m := reArray.FindStringSubmatch(content); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func preview(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

func looksLikeJSONPatch(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{")
}
