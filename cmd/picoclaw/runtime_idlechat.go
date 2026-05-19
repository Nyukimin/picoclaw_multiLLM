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
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
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
	if coder2Adapter != nil && cfg.Coder2.Provider == "openai" && cfg.Coder2.APIKey != "" {
		openaiProvider := openai.NewOpenAIProvider(cfg.Coder2.APIKey, cfg.Coder2.Model)
		idleChatOrch.SetForecastProvider(openaiProvider)
		idleChatOrch.InitForecastTopicStock(filepath.Join(cfg.Session.StorageDir, "forecast_topic_stock.json"))
		log.Printf("IdleChat: Forecast provider set to OpenAI (Coder2: %s), topic stock filling", cfg.Coder2.Model)
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
				deps.eventHub.OnEvent(viewerEvent)
			}
			if ev.Type == "idlechat.viewer" {
				return nil
			}
			return emitIdleChatTTSAsync(ttsBridge, ev)
		})
	}
	if deps.eventRelay != nil {
		deps.eventRelay.SetIdleChat(idleChatOrch)
	}
	idleChatOrch.Start()
	deps.idleChatOrch = idleChatOrch
	log.Printf("IdleChat enabled (participants=%v)", cfg.IdleChat.Participants)
}
