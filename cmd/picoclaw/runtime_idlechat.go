package main

import (
	"context"
	"log"
	"path/filepath"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	domainsession "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	llmfactory "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/factory"
)

func buildIdleChatRuntime(
	cfg *config.Config,
	deps *Dependencies,
	chatProvider llm.LLMProvider,
	workerProvider llm.LLMProvider,
	heavyProvider llm.LLMProvider,
	wildProvider llm.LLMProvider,
	centralMemory *domainsession.CentralMemory,
	coder2Adapter *coderAdapter,
	recentGlossaryTopics func(context.Context, int) ([]string, error),
	ttsBridge orchestrator.TTSBridge,
) {
	if !cfg.IdleChat.Enabled {
		return
	}
	idleChatOrch := idlechat.NewIdleChatOrchestrator(
		chatProvider,
		centralMemory,
		cfg.IdleChat.Participants,
		cfg.IdleChat.IntervalMin,
		cfg.IdleChat.MaxTurns,
		cfg.IdleChat.Temperature,
		config.BuildIdleChatAgentPrompts(cfg.Prompts),
		cfg.IdleChat.StoryDataDir,
	)
	idleChatOrch.SetIntervalSeconds(cfg.IdleChat.IntervalSec)
	idleChatOrch.SetSpeakerProviders(map[string]llm.LLMProvider{
		"mio":   chatProvider,
		"shiro": workerProvider,
		"kuro":  heavyProvider,
		"wild":  wildProvider,
	})
	idleChatOrch.SetSpeakerProviderOptions(idleChatProviderOptionsFromConfig(cfg.IdleChat.SpeakerLLMOptions))
	if forecastProvider, label, externalProvider, externalLabel := selectForecastProviders(cfg); forecastProvider != nil || externalProvider != nil {
		idleChatOrch.SetForecastProviderWithLabel(forecastProvider, label)
		idleChatOrch.SetForecastExternalProviderWithLabel(externalProvider, externalLabel)
		idleChatOrch.InitForecastTopicStock(filepath.Join(cfg.Session.StorageDir, "forecast_topic_stock.json"))
		log.Printf("IdleChat: Forecast provider set to primary=%s external=%s, topic stock filling", forecastProviderLogLabel(label), forecastProviderLogLabel(externalLabel))
	}
	if recentGlossaryTopics != nil {
		idleChatOrch.SetRecentTopicProvider(recentGlossaryTopics)
		log.Printf("IdleChat: Glossary topics injected")
	}
	if deps.personaRuntimeStore != nil {
		idleChatOrch.SetPersonaRuntimeRecorder(deps.personaRuntimeStore, deps.personaTriggerDefinitions)
		idleChatOrch.SetPersonaCanonicalResponses(deps.personaCanonicalResponses)
		log.Printf("Persona runtime recorder integrated with IdleChat (%d trigger definitions, %d canonical responses)", len(deps.personaTriggerDefinitions), len(deps.personaCanonicalResponses))
	}
	topicStorePath := filepath.Join(cfg.Session.StorageDir, "idlechat_topics.jsonl")
	if err := idleChatOrch.SetTopicStore(topicStorePath); err != nil {
		log.Printf("WARN: idleChat topic store disabled: %v", err)
	} else {
		log.Printf("IdleChat topic store enabled: %s", topicStorePath)
	}
	if deps.eventHub != nil {
		idleChatOrch.SetEventEmitter(func(ev idlechat.TimelineEvent) <-chan struct{} {
			if ev.Type != "idlechat.tts" {
				viewerType := ev.Type
				if viewerType == "idlechat.viewer" {
					viewerType = "idlechat.message"
				}
				chatID := strings.TrimSpace(ev.SessionID)
				if chatID == "" {
					chatID = "idlechat"
				}
				viewerEvent := orchestrator.NewEvent(
					viewerType,
					ev.From,
					ev.To,
					ev.Content,
					"IDLECHAT",
					"",
					ev.SessionID,
					"idlechat",
					chatID,
				)
				viewerEvent.RawContent = ev.RawContent
				viewerEvent.MessageID = ev.MessageID
				viewerEvent.TurnIndex = ev.TurnIndex
				deps.eventHub.OnEvent(viewerEvent)
			}
			if ev.Type == "idlechat.viewer" {
				return nil
			}
			return emitIdleChatTTSAsync(ttsBridge, ev)
		})
	}
	idleChatOrch.SetTTSTimeoutReporter(func(ev idlechat.TTSTimeoutEvent) {
		markIdleChatTTSTimeout(ev)
	})
	if deps.eventRelay != nil {
		deps.eventRelay.SetIdleChat(idleChatOrch)
	}
	idleChatOrch.Start()
	deps.idleChatOrch = idleChatOrch
	log.Printf("IdleChat enabled (participants=%v)", cfg.IdleChat.Participants)
}

func selectForecastProvider(cfg *config.Config, chatProvider, workerProvider, wildProvider llm.LLMProvider) (llm.LLMProvider, string) {
	primary, primaryLabel, external, externalLabel := selectForecastProviders(cfg)
	if primary != nil {
		return primary, primaryLabel
	}
	if external != nil {
		return external, externalLabel
	}
	return nil, ""
}

func selectForecastProviders(cfg *config.Config) (llm.LLMProvider, string, llm.LLMProvider, string) {
	if cfg == nil {
		return nil, "", nil, ""
	}
	primary, primaryLabel := createForecastProvider("Coder1", cfg.Coder1)
	if primary == nil {
		log.Printf("WARN: IdleChat forecast primary unavailable: Coder1 provider=%s model=%s", cfg.Coder1.Provider, cfg.Coder1.Model)
	}
	for _, candidate := range []struct {
		label string
		cfg   config.CoderConfig
	}{
		{"Coder2", cfg.Coder2},
		{"Coder3", cfg.Coder3},
		{"Coder4", cfg.Coder4},
	} {
		external, externalLabel := createForecastProvider(candidate.label, candidate.cfg)
		if external != nil {
			return primary, primaryLabel, external, externalLabel
		}
	}
	return primary, primaryLabel, nil, ""
}

func createForecastProvider(label string, cc config.CoderConfig) (llm.LLMProvider, string) {
	if !cc.Enabled {
		return nil, ""
	}
	provider, err := llmfactory.CreateProvider(cc)
	if err != nil {
		log.Printf("WARN: IdleChat forecast provider skipped: %s provider=%s model=%s: %v", label, cc.Provider, cc.Model, err)
		return nil, ""
	}
	if provider == nil {
		return nil, ""
	}
	return provider, label + " " + cc.Provider + " (" + forecastProviderModelLabel(cc.Model) + ")"
}

func forecastProviderLogLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return "unavailable"
	}
	return label
}

func forecastProviderModelLabel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "configured provider"
	}
	return model
}

func idleChatProviderOptionsFromConfig(options map[string]config.IdleChatLLMOptions) map[string]map[string]any {
	out := make(map[string]map[string]any, len(options))
	for name, opts := range options {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" || opts.Think == nil {
			continue
		}
		out[key] = map[string]any{"think": *opts.Think}
	}
	return out
}
