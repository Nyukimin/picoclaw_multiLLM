package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestNewCoderAgent(t *testing.T) {
	llmProvider := &mockLLMProvider{}
	toolRunner := &mockToolRunner{}
	mcpClient := &mockMCPClient{}

	coder := NewCoderAgent(llmProvider, toolRunner, mcpClient)

	if coder == nil {
		t.Fatal("NewCoderAgent should not return nil")
	}

	if coder.llmProvider != llmProvider {
		t.Error("llmProvider not set correctly")
	}
}

func TestCoderAgentGenerateProposal(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			// システムプロンプトにCoder指示が含まれているか確認
			if len(req.Messages) > 0 && req.Messages[0].Role == "system" {
				if req.Messages[0].Content == "" {
					t.Error("System prompt should not be empty")
				}
			}

			// Proposal形式のレスポンスを返す
			response := `## Plan
1. Create main.go file
2. Implement main function
3. Add error handling

## Patch
` + "```go:main.go\npackage main\n\nfunc main() {}\n```" + `

## Risk
Low risk - simple implementation

## CostHint
5 minutes`

			return llm.GenerateResponse{
				Content:      response,
				TokensUsed:   200,
				FinishReason: "stop",
			}, nil
		},
	}

	coder := NewCoderAgent(llmProvider, &mockToolRunner{}, &mockMCPClient{})

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "main.goファイルを作成して", "line", "U123")

	proposal, err := coder.GenerateProposal(context.Background(), testTask)
	if err != nil {
		t.Fatalf("GenerateProposal failed: %v", err)
	}

	if proposal == nil {
		t.Fatal("Proposal should not be nil")
	}

	if proposal.Plan() == "" {
		t.Error("Plan should not be empty")
	}

	if proposal.Patch() == "" {
		t.Error("Patch should not be empty")
	}

	if proposal.Risk() == "" {
		t.Error("Risk should not be empty")
	}

	if proposal.CostHint() == "" {
		t.Error("CostHint should not be empty")
	}
}

func TestCoderAgentGenerateProposal_LLMError(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{}, errors.New("API rate limit exceeded")
		},
	}

	coder := NewCoderAgent(llmProvider, &mockToolRunner{}, &mockMCPClient{})

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "テスト", "line", "U123")

	_, err := coder.GenerateProposal(context.Background(), testTask)
	if err == nil {
		t.Error("Expected error when LLM fails")
	}

	if err.Error() != "API rate limit exceeded" {
		t.Errorf("Expected 'API rate limit exceeded', got '%s'", err.Error())
	}
}

func TestCoderAgentGenerateProposal_InvalidFormat(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			// Plan/Patchセクションが欠けているレスポンス
			return llm.GenerateResponse{
				Content:      "This is not a valid proposal format",
				TokensUsed:   50,
				FinishReason: "stop",
			}, nil
		},
	}

	coder := NewCoderAgent(llmProvider, &mockToolRunner{}, &mockMCPClient{})

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "テスト", "line", "U123")

	proposal, err := coder.GenerateProposal(context.Background(), testTask)
	if err != nil {
		t.Fatalf("GenerateProposal should not error on invalid format: %v", err)
	}

	if proposal != nil {
		t.Error("Proposal should be nil for invalid format")
	}
}

func TestCoderAgentExtractSection(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	content := `## Plan
This is the plan section

## Patch
This is the patch section

## Risk
This is the risk section
`

	plan := coder.extractSection(content, "## Plan", "##")
	if plan != "This is the plan section" {
		t.Errorf("Expected 'This is the plan section', got '%s'", plan)
	}

	patch := coder.extractSection(content, "## Patch", "##")
	if patch != "This is the patch section" {
		t.Errorf("Expected 'This is the patch section', got '%s'", patch)
	}

	risk := coder.extractSection(content, "## Risk", "##")
	if risk != "This is the risk section" {
		t.Errorf("Expected 'This is the risk section', got '%s'", risk)
	}
}

func TestCoderAgentExtractSection_NotFound(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	content := "No sections here"

	result := coder.extractSection(content, "## Plan", "##")
	if result != "" {
		t.Errorf("Expected empty string for non-existent section, got '%s'", result)
	}
}

func TestCoderAgentExtractSection_LastSection(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	// 最後のセクション（次のセクションマーカーがない）
	content := `## Plan
First section

## CostHint
This is the last section with no next marker`

	costHint := coder.extractSection(content, "## CostHint", "##")
	if costHint != "This is the last section with no next marker" {
		t.Errorf("Expected full last section, got '%s'", costHint)
	}
}

func TestCoderAgentExtractProposal_CompleteProposal(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	content := `## Plan
Step 1: Create file
Step 2: Test

## Patch
` + "```go:main.go\npackage main\n```" + `

## Risk
Low risk

## CostHint
10 minutes`

	proposal := coder.extractProposal(content)

	if proposal == nil {
		t.Fatal("Proposal should not be nil")
	}

	if !proposal.IsValid() {
		t.Error("Proposal should be valid")
	}

	if proposal.Plan() == "" || proposal.Patch() == "" {
		t.Error("Plan and Patch should not be empty")
	}
}

func TestCoderAgentExtractProposal_MissingPlan(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	content := `## Patch
` + "```go:main.go\npackage main\n```"

	proposal := coder.extractProposal(content)

	if proposal != nil {
		t.Error("Proposal should be nil when Plan is missing")
	}
}

func TestCoderAgentExtractProposal_MissingPatch(t *testing.T) {
	coder := NewCoderAgent(&mockLLMProvider{}, &mockToolRunner{}, &mockMCPClient{})

	content := `## Plan
Step 1: Create file`

	proposal := coder.extractProposal(content)

	if proposal != nil {
		t.Error("Proposal should be nil when Patch is missing")
	}
}
