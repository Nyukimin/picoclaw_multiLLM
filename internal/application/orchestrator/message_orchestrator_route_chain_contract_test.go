package orchestrator

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestMessageOrchestrator_RouteChainContract_RoutingDecisionBeforeDispatch(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCHAT, 0.91, "chat"),
		response: "chat response",
	}
	orch := NewMessageOrchestrator(repo, mio, &mockShiroAgent{}, nil, nil, nil, nil, nil)
	rec := &recordingEventListener{}
	orch.SetEventListener(rec)

	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	messageIdx := indexOfEvent(rec.events, "message.received", "user", "mio", "")
	decisionIdx := indexOfEvent(rec.events, "routing.decision", "mio", "", "CHAT")
	startIdx := indexOfEvent(rec.events, "agent.start", "mio", "user", "CHAT")
	responseIdx := indexOfEvent(rec.events, "agent.response", "mio", "user", "CHAT")
	if messageIdx < 0 || decisionIdx < 0 || startIdx < 0 || responseIdx < 0 {
		t.Fatalf("missing route chain events: %#v", rec.events)
	}
	if !(messageIdx < decisionIdx && decisionIdx < startIdx && startIdx < responseIdx) {
		t.Fatalf("unexpected route chain event order: message=%d decision=%d start=%d response=%d", messageIdx, decisionIdx, startIdx, responseIdx)
	}
}

func TestMessageOrchestrator_RouteChainContract_ChatCommandBypassesRouteDecision(t *testing.T) {
	decideCalled := false
	mio := &mockMioAgent{
		decideFunc: func(ctx context.Context, t task.Task) (routing.Decision, error) {
			decideCalled = true
			return routing.NewDecision(routing.RouteOPS, 0.9, "should not run"), nil
		},
		cmdFunc: func(ctx context.Context, sessionID, message string) (agent.ChatCommandResult, error) {
			return agent.ChatCommandResult{Handled: true, Response: "command response"}, nil
		},
	}
	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, nil, nil, nil)
	rec := &recordingEventListener{}
	orch.SetEventListener(rec)

	resp, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if decideCalled {
		t.Fatal("chat command should be handled before route decision")
	}
	if resp.Route != routing.RouteCHAT {
		t.Fatalf("chat command route = %s, want CHAT", resp.Route)
	}
	if indexOfEvent(rec.events, "routing.decision", "mio", "", "OPS") >= 0 {
		t.Fatalf("routing.decision should not be emitted for handled chat command: %#v", rec.events)
	}
	if indexOfEvent(rec.events, "agent.response", "mio", "user", "CHAT") < 0 {
		t.Fatalf("chat command response event missing: %#v", rec.events)
	}
}

func TestMessageOrchestrator_RouteChainContract_InvalidProposalDoesNotReachWorker(t *testing.T) {
	worker := &recordingWorkerExecutionService{}
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE3, 1.0, "code3"),
	}
	coder3 := &mockCoderAgentWithProposal{
		proposal: proposal.NewProposal("", "", "", ""),
	}
	orch := NewMessageOrchestrator(newMockSessionRepository(), mio, &mockShiroAgent{}, nil, nil, coder3, nil, worker)

	_, err := orch.ProcessMessage(context.Background(), defaultReq())
	if err == nil {
		t.Fatal("invalid proposal should return error")
	}
	if worker.calls != 0 {
		t.Fatalf("invalid proposal reached WorkerExecutionService: calls=%d", worker.calls)
	}
}

type recordingWorkerExecutionService struct {
	calls int
}

func (w *recordingWorkerExecutionService) ExecuteProposal(ctx context.Context, jobID task.JobID, p *proposal.Proposal) (*patch.PatchExecutionResult, error) {
	w.calls++
	return patch.NewPatchExecutionResult(), nil
}
