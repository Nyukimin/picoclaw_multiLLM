package order

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestApprovalFlowModule(t *testing.T) {
	module := NewApprovalFlowModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "order3",
		Alias: "Gin",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "ApprovalFlow" {
		t.Errorf("Expected name 'ApprovalFlow', got '%s'", module.Name())
	}

	// Test StorePendingProposal
	proposal := Proposal{
		JobID:   "job_20260301_001",
		OrderID: "order3",
		Plan:    "Test plan",
		Patch:   "Test patch",
	}

	module.StorePendingProposal(proposal)

	// Test GetPendingProposal
	retrieved, exists := module.GetPendingProposal("job_20260301_001")
	if !exists {
		t.Error("Expected proposal to exist")
	}

	if retrieved.JobID != "job_20260301_001" {
		t.Errorf("Expected JobID 'job_20260301_001', got '%s'", retrieved.JobID)
	}

	// Test non-existent proposal
	_, exists = module.GetPendingProposal("nonexistent")
	if exists {
		t.Error("Expected proposal not to exist")
	}

	// Test ApprovePendingProposal
	approved := module.ApprovePendingProposal("job_20260301_001")
	if !approved {
		t.Error("Expected approval to succeed")
	}

	// After approval, proposal should be removed
	_, exists = module.GetPendingProposal("job_20260301_001")
	if exists {
		t.Error("Expected proposal to be removed after approval")
	}

	// Test DenyPendingProposal
	module.StorePendingProposal(proposal)
	denied := module.DenyPendingProposal("job_20260301_001")
	if !denied {
		t.Error("Expected denial to succeed")
	}

	// After denial, proposal should be removed
	_, exists = module.GetPendingProposal("job_20260301_001")
	if exists {
		t.Error("Expected proposal to be removed after denial")
	}

	// Test approve/deny non-existent
	approved = module.ApprovePendingProposal("nonexistent")
	if approved {
		t.Error("Expected approval of non-existent to fail")
	}

	denied = module.DenyPendingProposal("nonexistent")
	if denied {
		t.Error("Expected denial of non-existent to fail")
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
