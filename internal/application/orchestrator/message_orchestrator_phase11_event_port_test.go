package orchestrator

import "testing"

type phase11RecordingEventListener struct {
	events []OrchestratorEvent
}

func (l *phase11RecordingEventListener) OnEvent(ev OrchestratorEvent) {
	l.events = append(l.events, ev)
}

func TestPhase11EventPortNilListenerIsNoop(t *testing.T) {
	port := newMessageEventPort(nil)
	port.Emit("agent.start", "mio", "user", "考え中...", "CHAT", "job-1", "sess-1", "line", "U123")
	port.EmitMessageReceived(ProcessMessageRequest{
		SessionID:   "sess-1",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "こんにちは",
	})
}

func TestPhase11EventPortUsesUpdatedListener(t *testing.T) {
	port := newMessageEventPort(nil)
	listener := &phase11RecordingEventListener{}
	port.SetListener(listener)

	port.Emit("routing.decision", "mio", "", "confidence 90%", "CHAT", "job-1", "sess-1", "line", "U123")
	if len(listener.events) != 1 {
		t.Fatalf("expected one event, got %d", len(listener.events))
	}
	ev := listener.events[0]
	if ev.Type != "routing.decision" || ev.From != "mio" || ev.Route != "CHAT" || ev.JobID != "job-1" {
		t.Fatalf("unexpected event: %#v", ev)
	}

	port.EmitMessageReceived(ProcessMessageRequest{
		SessionID:   "sess-2",
		Channel:     "discord",
		ChatID:      "C123",
		UserMessage: "hello",
	})
	if len(listener.events) != 2 {
		t.Fatalf("expected two events, got %d", len(listener.events))
	}
	received := listener.events[1]
	if received.Type != "message.received" || received.From != "user" || received.To != "mio" {
		t.Fatalf("unexpected message received event: %#v", received)
	}
	if received.Route != "" || received.JobID != "" {
		t.Fatalf("message.received should not include route/job before decision: %#v", received)
	}
}
