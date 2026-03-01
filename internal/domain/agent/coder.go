package agent

import (
	"context"
	"strings"

	"github.com/sipeed/picoclaw/internal/domain/llm"
	"github.com/sipeed/picoclaw/internal/domain/proposal"
	"github.com/sipeed/picoclaw/internal/domain/task"
)

// CoderAgent は Coder（設計・実装）を担当するエンティティ
type CoderAgent struct {
	llmProvider llm.LLMProvider
	toolRunner  ToolRunner
	mcpClient   MCPClient
}

// NewCoderAgent は新しいCoderAgentを作成
func NewCoderAgent(
	llmProvider llm.LLMProvider,
	toolRunner ToolRunner,
	mcpClient MCPClient,
) *CoderAgent {
	return &CoderAgent{
		llmProvider: llmProvider,
		toolRunner:  toolRunner,
		mcpClient:   mcpClient,
	}
}

// GenerateProposal はplan/patchを生成
func (c *CoderAgent) GenerateProposal(ctx context.Context, t task.Task) (*proposal.Proposal, error) {
	// Coderシステムプロンプト
	systemPrompt := `You are a professional coder agent. Generate implementation proposals in the following format:

## Plan
[Implementation plan in bullet points]

## Patch
[Patch in JSON or Markdown format]

## Risk
[Risk assessment]

## CostHint
[Estimated cost/effort]
`

	req := llm.GenerateRequest{
		Messages: []llm.Message{
			{
				Role:    "system",
				Content: systemPrompt,
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
		return nil, err
	}

	// レスポンスからProposalを抽出
	return c.extractProposal(resp.Content), nil
}

// extractProposal はLLM応答からProposalを抽出
func (c *CoderAgent) extractProposal(content string) *proposal.Proposal {
	plan := c.extractSection(content, "## Plan", "##")
	patch := c.extractSection(content, "## Patch", "##")
	risk := c.extractSection(content, "## Risk", "##")
	costHint := c.extractSection(content, "## CostHint", "##")

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
