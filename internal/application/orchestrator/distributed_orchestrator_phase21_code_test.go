package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func TestPhase21DistributedCodeExecutionCoordinatorAddsCoderConfigAndFinishesWithoutProposal(t *testing.T) {
	var coderMsg domaintransport.Message
	coordinator := newDistributedCodeExecutionCoordinator(
		session.NewCentralMemory(),
		func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(route routing.Route, userMessage string) string { return "coder3" },
		func() map[string]interface{} { return map[string]interface{}{"coder3": "cfg"} },
		func() int { return 0 },
		func(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
			coderMsg = msg
			return domaintransport.Message{From: targetAgent, To: "shiro", Content: "coder result", Type: domaintransport.MessageTypeResult}, nil
		},
		func(ctx context.Context, targetAgent string, msg domaintransport.Message) (domaintransport.Message, error) {
			if targetAgent != "shiro" {
				t.Fatalf("expected shiro target, got %s", targetAgent)
			}
			return domaintransport.Message{From: "shiro", To: "mio", Content: "final result", Type: domaintransport.MessageTypeResult}, nil
		},
	)

	resp, err := coordinator.Execute(context.Background(), task.NewTask(task.NewJobID(), "code please", "line", "U123"), routing.RouteCODE3, "sess-1", "job-1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if resp != "final result" {
		t.Fatalf("expected final result, got %q", resp)
	}
	if coderMsg.Context["route"] != "CODE3" || coderMsg.Context["retry_attempt"] != 0 || coderMsg.Context["channel"] != "line" || coderMsg.Context["chat_id"] != "U123" {
		t.Fatalf("unexpected coder context: %#v", coderMsg.Context)
	}
	if coderMsg.Context["coder_config"] != "cfg" {
		t.Fatalf("expected coder_config, got %#v", coderMsg.Context)
	}
}

func TestPhase21DistributedCodeExecutionCoordinatorReturnsNoCoderMapped(t *testing.T) {
	coordinator := newDistributedCodeExecutionCoordinator(
		session.NewCentralMemory(),
		func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(route routing.Route, userMessage string) string { return "" },
		func() map[string]interface{} { return nil },
		func() int { return 0 },
		nil,
		nil,
	)

	_, err := coordinator.Execute(context.Background(), task.NewTask(task.NewJobID(), "code please", "line", "U123"), routing.RouteCODE, "sess-1", "job-1")
	if err == nil || err.Error() != "no coder mapped for route CODE" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPhase21DistributedCodeExecutionCoordinatorRetriesCoderMailboxFailure(t *testing.T) {
	var attempts []string
	coordinator := newDistributedCodeExecutionCoordinator(
		session.NewCentralMemory(),
		func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(route routing.Route, userMessage string) string { return "coder3" },
		func() map[string]interface{} { return nil },
		func() int { return 1 },
		func(ctx context.Context, targetAgent string, msg domaintransport.Message, receiveOnAgent string) (domaintransport.Message, error) {
			attempts = append(attempts, msg.Content)
			if len(attempts) == 1 {
				return domaintransport.Message{}, errors.New("command not found")
			}
			return domaintransport.Message{From: targetAgent, To: "shiro", Content: "coder result", Type: domaintransport.MessageTypeResult}, nil
		},
		func(ctx context.Context, targetAgent string, msg domaintransport.Message) (domaintransport.Message, error) {
			return domaintransport.Message{From: "shiro", To: "mio", Content: "final result", Type: domaintransport.MessageTypeResult}, nil
		},
	)

	resp, err := coordinator.Execute(context.Background(), task.NewTask(task.NewJobID(), "code please", "line", "U123"), routing.RouteCODE3, "sess-1", "job-1")
	if err != nil {
		t.Fatalf("Execute failed after retry: %v", err)
	}
	if resp != "final result" {
		t.Fatalf("expected final result, got %q", resp)
	}
	if len(attempts) != 2 {
		t.Fatalf("expected two coder attempts, got %#v", attempts)
	}
	if attempts[0] == attempts[1] {
		t.Fatalf("expected retry instruction to change request text")
	}
}
