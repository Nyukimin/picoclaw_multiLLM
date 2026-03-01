package order

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

// ApprovalFlowModule manages the approval workflow for proposals.
// This module handles storing pending proposals, checking approval status,
// and managing auto-approve rules.
type ApprovalFlowModule struct {
	agent            *modules.AgentCore
	pendingProposals map[string]Proposal
}

// NewApprovalFlowModule creates a new approval flow module.
func NewApprovalFlowModule() *ApprovalFlowModule {
	return &ApprovalFlowModule{
		pendingProposals: make(map[string]Proposal),
	}
}

// Name returns the module name.
func (m *ApprovalFlowModule) Name() string {
	return "ApprovalFlow"
}

// Initialize initializes the module with the agent core.
func (m *ApprovalFlowModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *ApprovalFlowModule) Shutdown(ctx context.Context) error {
	return nil
}

// StorePendingProposal stores a proposal pending approval.
func (m *ApprovalFlowModule) StorePendingProposal(proposal Proposal) {
	m.pendingProposals[proposal.JobID] = proposal

	logger.InfoCF("approval", "proposal.pending", map[string]interface{}{
		"job_id":   proposal.JobID,
		"order_id": proposal.OrderID,
	})
}

// GetPendingProposal retrieves a pending proposal by JobID.
func (m *ApprovalFlowModule) GetPendingProposal(jobID string) (Proposal, bool) {
	proposal, exists := m.pendingProposals[jobID]
	return proposal, exists
}

// ApprovePendingProposal marks a proposal as approved.
func (m *ApprovalFlowModule) ApprovePendingProposal(jobID string) bool {
	proposal, exists := m.pendingProposals[jobID]
	if !exists {
		return false
	}

	logger.InfoCF("approval", "proposal.approved", map[string]interface{}{
		"job_id":   jobID,
		"order_id": proposal.OrderID,
	})

	// TODO: Trigger execution of approved proposal
	delete(m.pendingProposals, jobID)
	return true
}

// DenyPendingProposal marks a proposal as denied.
func (m *ApprovalFlowModule) DenyPendingProposal(jobID string) bool {
	proposal, exists := m.pendingProposals[jobID]
	if !exists {
		return false
	}

	logger.InfoCF("approval", "proposal.denied", map[string]interface{}{
		"job_id":   jobID,
		"order_id": proposal.OrderID,
	})

	delete(m.pendingProposals, jobID)
	return true
}
