package idlechat

import (
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func TestEmitTopicToTimelineDoesNotWaitForTTSCompletion(t *testing.T) {
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil, "")
	ttsDone := make(chan struct{})
	eventSeen := make(chan struct{}, 1)
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		if ev.Type != "idlechat.message" {
			t.Fatalf("unexpected event type: %s", ev.Type)
		}
		eventSeen <- struct{}{}
		return ttsDone
	})

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		o.emitTopicToTimeline("idle-wait", "記憶と風景の関係", StrategyExternalStimulus)
	}()

	select {
	case <-eventSeen:
	case <-time.After(time.Second):
		t.Fatal("topic event was not emitted")
	}
	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("emitTopicToTimeline waited for TTS completion")
	}
	close(ttsDone)
}

func TestWaitForTTSDoneTimesOut(t *testing.T) {
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil, "")
	old := idleChatTTSWaitTimeout
	idleChatTTSWaitTimeout = 10 * time.Millisecond
	defer func() { idleChatTTSWaitTimeout = old }()

	blocked := make(chan struct{})
	start := time.Now()
	o.waitForTTSDone(blocked)

	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("waitForTTSDone did not time out promptly: %s", elapsed)
	}
}
