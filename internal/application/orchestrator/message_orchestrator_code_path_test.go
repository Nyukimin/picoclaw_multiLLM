package orchestrator

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

type recordingEventListener struct {
	events []OrchestratorEvent
}

func (r *recordingEventListener) OnEvent(ev OrchestratorEvent) {
	r.events = append(r.events, ev)
}

func indexOfEvent(events []OrchestratorEvent, typ, from, to, route string) int {
	for i, ev := range events {
		if ev.Type == typ && ev.From == from && ev.To == to && ev.Route == route {
			return i
		}
	}
	return -1
}

func TestMessageOrchestrator_CodeRoute_AlwaysViaShiro_CODE1(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE1, 1.0, "explicit code1"),
	}
	shiro := &mockShiroAgent{response: "unused"}
	coder1 := &mockCoderAgent{response: "spec ready"}
	orch := NewMessageOrchestrator(repo, mio, shiro, coder1, nil, nil, nil)
	rec := &recordingEventListener{}
	orch.SetEventListener(rec)

	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "s1",
		Channel:     "line",
		ChatID:      "u1",
		UserMessage: "design this",
	})
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	i1 := indexOfEvent(rec.events, "agent.start", "mio", "shiro", "CODE1")
	i2 := indexOfEvent(rec.events, "agent.start", "shiro", "coder1", "CODE1")
	i3 := indexOfEvent(rec.events, "agent.response", "coder1", "shiro", "CODE1")
	i4 := indexOfEvent(rec.events, "agent.response", "shiro", "mio", "CODE1")

	if i1 < 0 || i2 < 0 || i3 < 0 || i4 < 0 {
		t.Fatalf("missing expected shiro relay events for CODE1: %#v", rec.events)
	}
	if !(i1 < i2 && i2 < i3 && i3 < i4) {
		t.Fatalf("unexpected CODE1 event order: i1=%d i2=%d i3=%d i4=%d", i1, i2, i3, i4)
	}
}

func TestMessageOrchestrator_CodeRoute_AlwaysViaShiro_CODE2(t *testing.T) {
	repo := newMockSessionRepository()
	mio := &mockMioAgent{
		decision: routing.NewDecision(routing.RouteCODE2, 1.0, "explicit code2"),
	}
	shiro := &mockShiroAgent{response: "unused"}
	coder2 := &mockCoderAgent{response: "impl ready"}
	orch := NewMessageOrchestrator(repo, mio, shiro, nil, coder2, nil, nil)
	rec := &recordingEventListener{}
	orch.SetEventListener(rec)

	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "s2",
		Channel:     "line",
		ChatID:      "u2",
		UserMessage: "implement this",
	})
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	i1 := indexOfEvent(rec.events, "agent.start", "mio", "shiro", "CODE2")
	i2 := indexOfEvent(rec.events, "agent.start", "shiro", "coder2", "CODE2")
	i3 := indexOfEvent(rec.events, "agent.response", "coder2", "shiro", "CODE2")
	i4 := indexOfEvent(rec.events, "agent.response", "shiro", "mio", "CODE2")

	if i1 < 0 || i2 < 0 || i3 < 0 || i4 < 0 {
		t.Fatalf("missing expected shiro relay events for CODE2: %#v", rec.events)
	}
	if !(i1 < i2 && i2 < i3 && i3 < i4) {
		t.Fatalf("unexpected CODE2 event order: i1=%d i2=%d i3=%d i4=%d", i1, i2, i3, i4)
	}
}

