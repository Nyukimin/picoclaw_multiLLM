package idlechat

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

type blockingStoryProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingStoryProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	started := false
	p.once.Do(func() {
		close(p.started)
		started = true
	})
	if !started {
		return llm.GenerateResponse{Content: "QUALITY: pass\nISSUES:\n- 大きな損耗は検出されませんでした。\nPROMPT_FIX: ", FinishReason: "stop"}, nil
	}
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

func TestStartModesExposeNonEmptyCurrentTopicImmediately(t *testing.T) {
	tests := []struct {
		name  string
		start func(*IdleChatOrchestrator) error
	}{
		{
			name:  "forecast",
			start: func(o *IdleChatOrchestrator) error { return o.StartForecastMode() },
		},
		{
			name:  "story",
			start: func(o *IdleChatOrchestrator) error { return o.StartStoryMode() },
		},
		{
			name:  "story-simple",
			start: func(o *IdleChatOrchestrator) error { return o.StartSimpleStoryMode() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil, "")
			if err := tt.start(o); err != nil {
				t.Fatalf("start failed: %v", err)
			}
			if got := o.CurrentTopic(); got == "" {
				t.Fatal("current topic is empty immediately after start")
			}
		})
	}
}
