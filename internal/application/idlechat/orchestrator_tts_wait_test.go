package idlechat

import (
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func TestEmitTopicToTimelineWaitsForTTSCompletion(t *testing.T) {
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
		t.Fatal("emitTopicToTimeline returned before TTS completion")
	case <-time.After(50 * time.Millisecond):
	}

	close(ttsDone)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("emitTopicToTimeline did not return after TTS completion")
	}
}
