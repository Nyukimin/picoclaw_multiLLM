package middleware

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainllm "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
)

type limitedTestProvider struct {
	name string
	fn   func(context.Context, domainllm.GenerateRequest) (domainllm.GenerateResponse, error)
}

func (p limitedTestProvider) Generate(ctx context.Context, req domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
	if p.fn != nil {
		return p.fn(ctx, req)
	}
	return domainllm.GenerateResponse{Content: "ok", FinishReason: "stop"}, nil
}

func (p limitedTestProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "limited-test"
}

func TestLimitedProviderQueueTimeoutDoesNotCallInnerProvider(t *testing.T) {
	var calls atomic.Int32
	model := make(chan struct{}, 1)
	model <- struct{}{}
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(context.Context, domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			calls.Add(1)
			return domainllm.GenerateResponse{Content: "unexpected"}, nil
		},
	}, "local-Chat-Chat", nil, model, LimitedProviderOptions{
		QueueTimeout:      10 * time.Millisecond,
		GenerationTimeout: time.Second,
		QueuePolicy:       LLMQueuePolicyWait,
	})

	_, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16})
	if err == nil {
		t.Fatal("Generate() error = nil, want queue timeout")
	}
	var phaseErr LLMPhaseError
	if !errors.As(err, &phaseErr) || phaseErr.Phase != LLMTimeoutPhaseQueue {
		t.Fatalf("Generate() error = %v, want phase=queue", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("inner provider calls = %d, want 0", calls.Load())
	}
}

func TestLimitedProviderGenerationTimeoutIsSeparateFromQueue(t *testing.T) {
	var calls atomic.Int32
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(ctx context.Context, _ domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			calls.Add(1)
			<-ctx.Done()
			return domainllm.GenerateResponse{}, ctx.Err()
		},
	}, "local-Chat-Chat", nil, make(chan struct{}, 1), LimitedProviderOptions{
		QueueTimeout:      time.Second,
		GenerationTimeout: 10 * time.Millisecond,
		QueuePolicy:       LLMQueuePolicyWait,
	})

	_, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16})
	if err == nil {
		t.Fatal("Generate() error = nil, want generation timeout")
	}
	var phaseErr LLMPhaseError
	if !errors.As(err, &phaseErr) || phaseErr.Phase != LLMTimeoutPhaseGenerate {
		t.Fatalf("Generate() error = %v, want phase=generate", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("inner provider calls = %d, want 1", calls.Load())
	}
}

func TestLimitedProviderWorkerGenerationTimeoutIsClassified(t *testing.T) {
	var events atomic.Int32
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(ctx context.Context, _ domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			<-ctx.Done()
			return domainllm.GenerateResponse{}, ctx.Err()
		},
	}, "local-Worker-Worker", nil, make(chan struct{}, 1), LimitedProviderOptions{
		Alias:             "Worker",
		QueueTimeout:      time.Second,
		GenerationTimeout: 10 * time.Millisecond,
		QueuePolicy:       LLMQueuePolicyWait,
		OnBackendTimeout: func(ev BackendTimeoutEvent) {
			if ev.Alias != "Worker" {
				t.Fatalf("event alias=%q", ev.Alias)
			}
			events.Add(1)
		},
	})

	_, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16})
	if err == nil {
		t.Fatal("Generate() error = nil, want worker backend timeout")
	}
	var phaseErr LLMPhaseError
	if !errors.As(err, &phaseErr) || phaseErr.Phase != LLMTimeoutPhaseGenerate {
		t.Fatalf("Generate() error = %v, want phase=generate", err)
	}
	var backendErr WorkerBackendTimeoutError
	if !errors.As(err, &backendErr) {
		t.Fatalf("Generate() error = %v, want WorkerBackendTimeoutError", err)
	}
	if !strings.Contains(backendErr.Error(), "Worker backend timeout") {
		t.Fatalf("backend error=%q", backendErr.Error())
	}
	if events.Load() != 1 {
		t.Fatalf("backend timeout events=%d, want 1", events.Load())
	}
}

func TestLimitedProviderWorkerBackendBusy429IsClassifiedWithoutRecovery(t *testing.T) {
	var events atomic.Int32
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(context.Context, domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			return domainllm.GenerateResponse{}, openai.CompatibleAPIStatusError{
				Operation:  "openai API",
				StatusCode: 429,
				Body:       `{"error":{"code":"backend_busy"}}`,
			}
		},
	}, "local-ChatWorker-ChatWorker", nil, make(chan struct{}, 1), LimitedProviderOptions{
		Alias:             "ChatWorker",
		QueueTimeout:      time.Second,
		GenerationTimeout: time.Second,
		QueuePolicy:       LLMQueuePolicyWait,
		OnBackendTimeout: func(BackendTimeoutEvent) {
			events.Add(1)
		},
	})

	_, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16})
	if err == nil {
		t.Fatal("Generate() error = nil, want backend busy")
	}
	var busyErr WorkerBackendBusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("Generate() error = %v, want WorkerBackendBusyError", err)
	}
	if events.Load() != 0 {
		t.Fatalf("backend timeout events=%d, want 0", events.Load())
	}
}

func TestLimitedProviderWorkerBackendTimeout504IsClassified(t *testing.T) {
	var events atomic.Int32
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(context.Context, domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			return domainllm.GenerateResponse{}, openai.CompatibleAPIStatusError{
				Operation:  "openai API",
				StatusCode: 504,
				Body:       `{"error":{"code":"backend_timeout"}}`,
			}
		},
	}, "local-Worker-Worker", nil, make(chan struct{}, 1), LimitedProviderOptions{
		Alias:             "Worker",
		QueueTimeout:      time.Second,
		GenerationTimeout: time.Second,
		QueuePolicy:       LLMQueuePolicyWait,
		OnBackendTimeout: func(BackendTimeoutEvent) {
			events.Add(1)
		},
	})

	_, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16})
	if err == nil {
		t.Fatal("Generate() error = nil, want backend timeout")
	}
	var backendErr WorkerBackendTimeoutError
	if !errors.As(err, &backendErr) {
		t.Fatalf("Generate() error = %v, want WorkerBackendTimeoutError", err)
	}
	if events.Load() != 1 {
		t.Fatalf("backend timeout events=%d, want 1", events.Load())
	}
}

func TestLimitedProviderModelConcurrencySerializesSameAlias(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32
	provider := NewLimitedProviderWithOptions(limitedTestProvider{
		fn: func(context.Context, domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
			cur := active.Add(1)
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return domainllm.GenerateResponse{Content: "ok", FinishReason: "stop"}, nil
		},
	}, "local-Chat-Chat", nil, make(chan struct{}, 1), LimitedProviderOptions{
		QueueTimeout:      time.Second,
		GenerationTimeout: time.Second,
		QueuePolicy:       LLMQueuePolicyWait,
	})

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := provider.Generate(context.Background(), domainllm.GenerateRequest{MaxTokens: 16}); err != nil {
				t.Errorf("Generate() error = %v", err)
			}
		}()
	}
	wg.Wait()
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("max active calls = %d, want 1", got)
	}
}
