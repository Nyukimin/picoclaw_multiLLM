package agent

import (
	"context"
	"log"
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
	log.Printf("[CoderAgent] proposal generate start provider=%s job=%s prompt_len=%d", c.llmProvider.Name(), t.JobID().String(), len(t.UserMessage()))
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
		log.Printf("[CoderAgent] proposal generate error provider=%s job=%s err=%v", c.llmProvider.Name(), t.JobID().String(), err)
		return nil, err
	}
	log.Printf("[CoderAgent] proposal generate response provider=%s job=%s content_len=%d finish=%s", c.llmProvider.Name(), t.JobID().String(), len(resp.Content), resp.FinishReason)

	// レスポンスからProposalを抽出
	p := c.extractProposal(resp.Content)
	if p == nil {
		log.Printf("[CoderAgent] proposal extract empty provider=%s job=%s", c.llmProvider.Name(), t.JobID().String())
	} else {
		log.Printf("[CoderAgent] proposal extract complete provider=%s job=%s plan_len=%d patch_len=%d", c.llmProvider.Name(), t.JobID().String(), len(p.Plan()), len(p.Patch()))
	}
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
	patch := normalizeProposalPatch(c.extractSection(content, "## Patch", "##"))
	risk := c.extractSection(content, "## Risk", "##")
	costHint := c.extractSection(content, "## CostHint", "##")

	if plan == "" || patch == "" {
		return nil
	}

	return proposal.NewProposal(plan, patch, risk, costHint)
}

func normalizeProposalPatch(patch string) string {
	trimmed := strings.TrimSpace(patch)
	if trimmed == "" {
		return ""
	}

	if unwrapped, ok := unwrapSingleFence(trimmed); ok {
		trimmed = strings.TrimSpace(unwrapped)
	}

	return trimmed
}

func unwrapSingleFence(content string) (string, bool) {
	if !strings.HasPrefix(content, "```") || !strings.HasSuffix(content, "```") {
		return "", false
	}

	firstNL := strings.IndexByte(content, '\n')
	if firstNL == -1 {
		return "", false
	}

	header := strings.TrimSpace(content[3:firstNL])
	body := strings.TrimSpace(content[firstNL+1 : len(content)-3])
	if body == "" {
		return "", false
	}

	switch header {
	case "json", "markdown", "md", "":
		return body, true
	default:
		return "", false
	}
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
