// Package order provides modules for Order agents (Order1/Aka, Order2/Ao, Order3/Gin).
package order

import (
	"context"
	"fmt"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
)

// ProposalRequest contains the information needed to generate a proposal.
type ProposalRequest struct {
	JobID    string
	Route    string
	UserText string
	Context  map[string]interface{}
}

// Proposal represents a generated proposal from an Order agent.
type Proposal struct {
	JobID      string
	OrderID    string
	Plan       string
	Patch      string
	Risk       string
	Confidence float64
	Metadata   map[string]interface{}
}

// ProposalGenerationModule generates implementation proposals.
// This module is the core capability of Order agents - generating plan/patch/risk
// for code changes without directly executing them.
type ProposalGenerationModule struct {
	agent *modules.AgentCore
}

// NewProposalGenerationModule creates a new proposal generation module.
func NewProposalGenerationModule() *ProposalGenerationModule {
	return &ProposalGenerationModule{}
}

// Name returns the module name.
func (m *ProposalGenerationModule) Name() string {
	return "ProposalGeneration"
}

// Initialize initializes the module with the agent core.
func (m *ProposalGenerationModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *ProposalGenerationModule) Shutdown(ctx context.Context) error {
	return nil
}

// GenerateProposal creates a proposal (plan/patch/risk) for the given request.
// This method uses the Order agent's LLM provider to generate the proposal.
func (m *ProposalGenerationModule) GenerateProposal(ctx context.Context, req ProposalRequest) (Proposal, error) {
	logger.InfoCF("proposal", "order.generate", map[string]interface{}{
		"job_id":   req.JobID,
		"order_id": m.agent.ID,
		"route":    req.Route,
	})

	if m.agent.Provider == nil {
		return Proposal{}, fmt.Errorf("LLM provider not configured for %s", m.agent.ID)
	}

	// Build system prompt based on Order agent role
	systemPrompt := m.buildSystemPrompt()

	// Build user prompt requesting plan/patch/risk
	userPrompt := fmt.Sprintf(`以下のタスクについて、実装提案を作成してください：

%s

以下の形式で回答してください：
## PLAN
実装の計画・手順

## PATCH
実装するコードの差分

## RISK
リスク評価・注意点`, req.UserText)

	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	response, err := m.agent.Provider.Chat(ctx, messages, nil, m.agent.Model, nil)
	if err != nil {
		return Proposal{}, fmt.Errorf("proposal generation failed: %w", err)
	}

	// Parse response into plan/patch/risk
	plan, patch, risk := m.parseProposalResponse(response.Content)

	proposal := Proposal{
		JobID:      req.JobID,
		OrderID:    m.agent.ID,
		Plan:       plan,
		Patch:      patch,
		Risk:       risk,
		Confidence: 0.8, // TODO: Calculate confidence from response
		Metadata: map[string]interface{}{
			"tokens": response.Usage.TotalTokens,
			"model":  m.agent.Model,
		},
	}

	logger.InfoCF("proposal", "order.generated", map[string]interface{}{
		"job_id":     req.JobID,
		"order_id":   m.agent.ID,
		"confidence": proposal.Confidence,
		"tokens":     response.Usage.TotalTokens,
	})

	return proposal, nil
}

// buildSystemPrompt returns the system prompt for this Order agent.
func (m *ProposalGenerationModule) buildSystemPrompt() string {
	switch m.agent.ID {
	case "order1":
		return "あなたは仕様設計のスペシャリスト（Aka）です。要件を整理し、設計書と実装計画を作成します。"
	case "order2":
		return "あなたは実装のスペシャリスト（Ao）です。効率的なコードを書き、パフォーマンスを重視します。"
	case "order3":
		return "あなたは高品質コーディングのスペシャリスト（Gin）です。保守性・可読性・安全性を重視した実装を提案します。"
	default:
		return "あなたはコーディングのスペシャリストです。"
	}
}

// parseProposalResponse parses the LLM response into plan/patch/risk sections.
func (m *ProposalGenerationModule) parseProposalResponse(content string) (plan, patch, risk string) {
	// Simple parser: split by ## markers
	lines := strings.Split(content, "\n")
	currentSection := ""
	var planLines, patchLines, riskLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		if strings.HasPrefix(upper, "## PLAN") {
			currentSection = "plan"
			continue
		} else if strings.HasPrefix(upper, "## PATCH") {
			currentSection = "patch"
			continue
		} else if strings.HasPrefix(upper, "## RISK") {
			currentSection = "risk"
			continue
		}

		switch currentSection {
		case "plan":
			planLines = append(planLines, line)
		case "patch":
			patchLines = append(patchLines, line)
		case "risk":
			riskLines = append(riskLines, line)
		}
	}

	plan = strings.TrimSpace(strings.Join(planLines, "\n"))
	patch = strings.TrimSpace(strings.Join(patchLines, "\n"))
	risk = strings.TrimSpace(strings.Join(riskLines, "\n"))

	// Fallback: if parsing failed, use full content as plan
	if plan == "" && patch == "" && risk == "" {
		plan = content
	}

	return plan, patch, risk
}
