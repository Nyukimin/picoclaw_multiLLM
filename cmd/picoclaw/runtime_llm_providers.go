package main

import (
	"log"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

type llmRuntimeProviders struct {
	Primary            primaryLLMProviders
	Chat               llm.LLMProvider
	Worker             llm.LLMProvider
	Heavy              llm.LLMProvider
	Wild               llm.LLMProvider
	WorkerToolProvider llm.ToolCallingProvider
	Coder1             *coderAdapter
	Coder2             *coderAdapter
	Coder3             *coderAdapter
	Coder4             *coderAdapter
}

func buildLLMRuntimeProviders(cfg *config.Config) llmRuntimeProviders {
	primaryProviders := buildPrimaryLLMProviders(cfg)
	workerToolProvider, ok := primaryProviders.Worker.(llm.ToolCallingProvider)
	if !ok {
		log.Fatalf("worker provider %s does not support tool calling", primaryProviders.Worker.Name())
	}
	coder1Adapter, coder2Adapter, coder3Adapter, coder4Adapter := setupCoders(cfg)
	return llmRuntimeProviders{
		Primary:            primaryProviders,
		Chat:               primaryProviders.Chat,
		Worker:             primaryProviders.Worker,
		Heavy:              primaryProviders.Heavy,
		Wild:               primaryProviders.Wild,
		WorkerToolProvider: workerToolProvider,
		Coder1:             coder1Adapter,
		Coder2:             coder2Adapter,
		Coder3:             coder3Adapter,
		Coder4:             coder4Adapter,
	}
}
