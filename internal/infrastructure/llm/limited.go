package llm

import (
	"context"
	"fmt"

	domainllm "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// LimitedProvider bounds concurrent requests for an LLMProvider.
// Multiple LimitedProviders can share the same global semaphore while keeping
// their own per-model semaphore.
type LimitedProvider struct {
	inner  domainllm.LLMProvider
	name   string
	global chan struct{}
	model  chan struct{}
}

func NewLimitedProvider(inner domainllm.LLMProvider, name string, global, model chan struct{}) *LimitedProvider {
	return &LimitedProvider{
		inner:  inner,
		name:   name,
		global: global,
		model:  model,
	}
}

func (p *LimitedProvider) Generate(ctx context.Context, req domainllm.GenerateRequest) (domainllm.GenerateResponse, error) {
	release, err := p.acquire(ctx)
	if err != nil {
		return domainllm.GenerateResponse{}, err
	}
	defer release()
	return p.inner.Generate(ctx, req)
}

func (p *LimitedProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return p.inner.Name()
}

func (p *LimitedProvider) Chat(ctx context.Context, req domainllm.ChatRequest) (domainllm.ChatResponse, error) {
	tcp, ok := p.inner.(domainllm.ToolCallingProvider)
	if !ok {
		return domainllm.ChatResponse{}, fmt.Errorf("inner provider does not support Chat")
	}
	release, err := p.acquire(ctx)
	if err != nil {
		return domainllm.ChatResponse{}, err
	}
	defer release()
	return tcp.Chat(ctx, req)
}

func (p *LimitedProvider) acquire(ctx context.Context) (func(), error) {
	acquiredGlobal := false
	acquiredModel := false
	release := func() {
		if acquiredModel && p.model != nil {
			<-p.model
		}
		if acquiredGlobal && p.global != nil {
			<-p.global
		}
	}
	if p.global != nil {
		select {
		case p.global <- struct{}{}:
			acquiredGlobal = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.model != nil {
		select {
		case p.model <- struct{}{}:
			acquiredModel = true
		case <-ctx.Done():
			release()
			return nil, ctx.Err()
		}
	}
	return release, nil
}
