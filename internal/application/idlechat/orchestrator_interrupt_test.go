package idlechat

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

type blockingInterruptProvider struct {
	started chan struct{}
	done    chan error
}

func (p *blockingInterruptProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	close(p.started)
	<-ctx.Done()
	err := ctx.Err()
	p.done <- err
	return llm.GenerateResponse{}, err
}

func (p *blockingInterruptProvider) Name() string { return "blocking-interrupt" }

func TestIdleChatInterruptResetsStateAndCancelsRunContext(t *testing.T) {
	provider := &blockingInterruptProvider{
		started: make(chan struct{}),
		done:    make(chan error, 1),
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil)

	o.mu.Lock()
	o.manualMode = true
	o.chatActive = true
	o.sessionMode = "idle"
	o.currentTopic = "topic"
	o.sessionContext = "context"
	o.beginIdleRunLocked()
	o.activeSessionID = "idle-test-topic-00"
	o.mu.Unlock()

	go func() {
		_, _ = o.generateIdleLLM(provider, llm.GenerateRequest{Messages: []llm.Message{{Role: "user", Content: "hello"}}})
	}()

	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("provider did not start")
	}

	o.Interrupt("user_input")

	select {
	case err := <-provider.done:
		if err == nil {
			t.Fatal("expected canceled context error")
		}
	case <-time.After(time.Second):
		t.Fatal("interrupt did not cancel running LLM request")
	}

	if o.IsManualMode() {
		t.Fatal("manualMode should be false after interrupt")
	}
	if o.IsChatActive() {
		t.Fatal("chatActive should be false after interrupt")
	}
	if got := o.CurrentMode(); got != "" {
		t.Fatalf("CurrentMode() = %q, want empty", got)
	}
	if got := o.CurrentTopic(); got != "" {
		t.Fatalf("CurrentTopic() = %q, want empty", got)
	}
}

func TestIdleChatInterruptDiscardsStaleTimelineEvent(t *testing.T) {
	o := NewIdleChatOrchestrator(&capturingIdleProvider{response: "ok"}, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil)
	emitted := 0
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		emitted++
		done := make(chan struct{})
		close(done)
		return done
	})

	o.mu.Lock()
	o.chatActive = true
	o.beginIdleRunLocked()
	o.activeSessionID = "idle-stale-topic-00"
	o.mu.Unlock()

	o.Interrupt("user_input")
	done := o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "shiro",
		Content:   "late response",
		SessionID: "idle-stale-topic-00",
	})

	if done != nil {
		t.Fatal("stale idlechat event should not return a TTS wait channel")
	}
	if emitted != 0 {
		t.Fatalf("stale idlechat event emitted %d times, want 0", emitted)
	}
}
