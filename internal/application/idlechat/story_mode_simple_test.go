package idlechat

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

type blockingStoryProvider struct {
	started chan struct{}
	release chan struct{}
}

func (p *blockingStoryProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	close(p.started)
	select {
	case <-p.release:
	case <-ctx.Done():
		return llm.GenerateResponse{}, ctx.Err()
	}
	return llm.GenerateResponse{Content: "【テスト物語】\n最初の段落です。次の段落です。", FinishReason: "stop"}, nil
}

func (p *blockingStoryProvider) Name() string {
	return "blocking-story"
}

func TestRunSimpleStorySessionEmitsIntroBeforeGenerationCompletes(t *testing.T) {
	provider := &blockingStoryProvider{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil, "")

	events := make(chan TimelineEvent, 32)
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		events <- ev
		return nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		o.RunSimpleStorySession()
	}()

	select {
	case ev := <-events:
		if ev.Type != "idlechat.viewer" {
			t.Fatalf("first event type = %q, want idlechat.viewer", ev.Type)
		}
		if ev.Content == "" {
			t.Fatal("intro event content is empty")
		}
	case <-time.After(time.Second):
		t.Fatal("no viewer intro emitted before generation completed")
	}

	if got := o.CurrentTopic(); got == "" {
		t.Fatal("current topic is empty while story generation is active")
	}

	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("story generation did not start after intro")
	}

	close(provider.release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("story session did not finish")
	}
}
