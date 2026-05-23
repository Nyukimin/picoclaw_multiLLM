package idlechat

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func NewIdleChatOrchestrator(
	llmProvider llm.LLMProvider,
	memory *session.CentralMemory,
	participants []string,
	intervalMin int,
	maxTurns int,
	temperature float64,
	personalities map[string]string,
	storyDataDir string,
) *IdleChatOrchestrator {
	randSeedOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})
	// LoadStoryData: Complex Story Mode用、現在はアーカイブ済み
	// Simple Story Mode はハードコードされた昔話リストを使用
	_ = storyDataDir // unused
	ctx, cancel := context.WithCancel(context.Background())
	return &IdleChatOrchestrator{
		llmProvider:         llmProvider,
		speakerLLMs:         make(map[string]llm.LLMProvider),
		memory:              memory,
		participants:        participants,
		intervalMin:         intervalMin,
		interval:            time.Duration(intervalMin) * time.Minute,
		maxTurns:            maxTurns,
		temperature:         temperature,
		personalities:       personalities,
		lastActivity:        time.Now(),
		history:             make([]SessionSummary, 0, 32),
		ctx:                 ctx,
		cancel:              cancel,
		runCtx:              ctx,
		interruptedSessions: make(map[string]struct{}),
	}
}

func (o *IdleChatOrchestrator) SetEventEmitter(emit func(TimelineEvent) <-chan struct{}) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitEvent = emit
}

// SetForecastProvider sets a high-capability LLM for forecast topic generation and keyword extraction.

func (o *IdleChatOrchestrator) SetForecastProvider(provider llm.LLMProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.forecastProvider = provider
}

func (o *IdleChatOrchestrator) SetRecentTopicProvider(provider func(context.Context, int) ([]string, error)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.recentTopics = provider
}

// SetTopicStore configures persistent storage for topic summaries.

func (o *IdleChatOrchestrator) SetSpeakerProviders(providers map[string]llm.LLMProvider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.speakerLLMs = make(map[string]llm.LLMProvider, len(providers))
	for name, provider := range providers {
		if provider == nil {
			continue
		}
		o.speakerLLMs[strings.ToLower(strings.TrimSpace(name))] = provider
	}
}

func (o *IdleChatOrchestrator) SetTopicStore(path string) error {
	store, err := NewTopicStore(path)
	if err != nil {
		return err
	}
	o.mu.Lock()
	o.topicStore = store
	o.history = store.GetRecent(200)
	o.mu.Unlock()
	return nil
}

// NewIdleChatOrchestrator は新しいIdleChatOrchestratorを作成

func (o *IdleChatOrchestrator) providerForSpeaker(name string) llm.LLMProvider {
	o.mu.Lock()
	defer o.mu.Unlock()
	if provider, ok := o.speakerLLMs[strings.ToLower(strings.TrimSpace(name))]; ok && provider != nil {
		return provider
	}
	return o.llmProvider
}

// Start はIdleChatの監視ループを開始
