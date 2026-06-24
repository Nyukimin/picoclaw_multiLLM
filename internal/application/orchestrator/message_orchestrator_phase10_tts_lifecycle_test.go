package orchestrator

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestPhase10TTSLifecycleUsesUpdatedTTSBridge(t *testing.T) {
	bridge := &mockTTSBridge{}
	lifecycle := newMessageTTSLifecycle(nil, nil, func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {})
	lifecycle.SetTTSBridge(bridge)

	req := ProcessMessageRequest{
		SessionID:   "sess-1",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "実行して",
	}
	jobID := task.NewJobID()
	decision := routing.NewDecision(routing.RouteOPS, 0.9, "ops")

	lifecycle.StartSessionForRoute(context.Background(), req, jobID, decision, "tts-1")
	if len(bridge.startReqs) != 1 {
		t.Fatalf("expected one TTS start request, got %d", len(bridge.startReqs))
	}
	if bridge.startReqs[0].SessionID != "tts-1" {
		t.Fatalf("expected session tts-1, got %s", bridge.startReqs[0].SessionID)
	}

	lifecycle.Push(context.Background(), "tts-1", routing.RouteOPS, "agent.response", "完了しました")
	if len(bridge.pushes) == 0 {
		t.Fatal("expected TTS push after bridge update")
	}

	lifecycle.EndSession(context.Background(), "tts-1")
	if len(bridge.ended) != 1 || bridge.ended[0] != "tts-1" {
		t.Fatalf("expected TTS end for tts-1, got %#v", bridge.ended)
	}
}

func TestPhase10TTSLifecycleStreamHooksPreservePreviousCallbackAndEmitThinking(t *testing.T) {
	var previous []string
	var emitted []string
	lifecycle := newMessageTTSLifecycle(nil, nil, func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
		emitted = append(emitted, eventType+":"+content)
	})

	ctx := llm.ContextWithStreamCallback(context.Background(), func(token string) {
		previous = append(previous, token)
	})
	streamCtx, bundle := lifecycle.WithStreamHooks(ctx, routing.RouteCHAT, "job-1", "sess-1", "line", "U123", "")
	if bundle == nil {
		t.Fatal("expected stream bundle")
	}

	callback := llm.StreamCallbackFromContext(streamCtx)
	if callback == nil {
		t.Fatal("expected stream callback")
	}
	callback("tok")

	if len(previous) != 1 || previous[0] != "tok" {
		t.Fatalf("expected previous callback to receive token, got %#v", previous)
	}
	if len(emitted) != 1 || emitted[0] != "agent.thinking:tok" {
		t.Fatalf("expected thinking event for token, got %#v", emitted)
	}
}
