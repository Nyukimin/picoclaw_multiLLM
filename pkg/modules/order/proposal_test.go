package order

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestProposalGenerationModule(t *testing.T) {
	module := NewProposalGenerationModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "order1",
		Alias: "Aka",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "ProposalGeneration" {
		t.Errorf("Expected name 'ProposalGeneration', got '%s'", module.Name())
	}

	// Test GenerateProposal with nil Provider
	req := ProposalRequest{
		JobID:    "job_20260301_001",
		Route:    "CODE3",
		UserText: "テストコードを書いて",
	}

	// This should fail because Provider is nil
	_, err = module.GenerateProposal(context.Background(), req)
	if err == nil {
		t.Error("Expected error when Provider is nil")
	}

	expectedErrMsg := "LLM provider not configured for order1"
	if err.Error() != expectedErrMsg {
		t.Errorf("Expected error '%s', got '%s'", expectedErrMsg, err.Error())
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
