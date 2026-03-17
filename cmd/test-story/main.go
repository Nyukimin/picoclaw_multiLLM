package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/ollama"
)

func main() {
	cfg, err := config.LoadConfig("./config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	chatProvider := ollama.NewOllamaProviderWithNumCtx(cfg.Ollama.BaseURL, cfg.Ollama.Model, 32768)
	memory := session.NewCentralMemory()
	orch := idlechat.NewIdleChatOrchestrator(
		chatProvider,
		memory,
		cfg.IdleChat.Participants,
		cfg.IdleChat.IntervalMin,
		cfg.IdleChat.MaxTurns,
		cfg.IdleChat.Temperature,
		cfg.Prompts.IdleChatAgents,
	)
	orch.SetSpeakerProviders(map[string]llm.LLMProvider{
		"mio":   chatProvider,
		"shiro": chatProvider,
	})

	orch.RunStorySession()

	history := orch.GetHistory(1)
	if len(history) == 0 {
		fmt.Println(`{"status":"empty"}`)
		return
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(history[0]); err != nil {
		log.Fatalf("encode history: %v", err)
	}
}
