package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/subagent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persona"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
)

type agentRuntime struct {
	Mio   *agent.MioAgent
	Shiro *agent.ShiroAgent
	Heavy *agent.HeavyAgent
	Wild  *agent.WildAgent
}

func buildAgentRuntime(
	cfg *config.Config,
	chatProvider llm.LLMProvider,
	workerProvider llm.LLMProvider,
	heavyProvider llm.LLMProvider,
	wildProvider llm.LLMProvider,
	classifier *routing.LLMClassifier,
	ruleDictionary *routing.RuleDictionary,
	chatToolRunner agent.ToolRunner,
	workerToolRunner agent.ToolRunner,
	mcpClient *mcp.MCPClient,
	convEngine conversation.ConversationEngine,
	recentGlossaryContext func(context.Context, int) (string, error),
	realMgr *conversationpersistence.RealConversationManager,
	l1Store *conversationpersistence.L1SQLiteStore,
	subagentMgr *subagent.Manager,
) agentRuntime {
	mioAgent := agent.NewMioAgent(chatProvider, classifier, ruleDictionary, chatToolRunner, mcpClient, convEngine).
		WithSystemPrompt(cfg.Prompts.MioPersona)
	if recentGlossaryContext != nil {
		mioAgent = mioAgent.WithRecentContextProvider(recentGlossaryContext)
		log.Printf("Mio: Glossary context injected")
	}
	if realMgr != nil {
		mioAgent = mioAgent.WithKBManager(realMgr)
		log.Printf("Mio: KBManager injected (KB autosave enabled)")
	}
	if l1Store != nil {
		mioAgent = mioAgent.WithUserMemoryManager(l1Store)
		log.Printf("Mio: UserMemoryManager injected")
	}
	mioPersonaFile := filepath.Join(cfg.WorkspaceDir, "persona", "mio.md")
	if cfg.MioPersonaFile != "" {
		mioPersonaFile = filepath.Join(cfg.WorkspaceDir, cfg.MioPersonaFile)
	}
	personaEditor := persona.NewFilePersonaEditor(mioPersonaFile)
	mioAgent = mioAgent.WithPersonaEditor(personaEditor)
	log.Printf("Mio: PersonaEditor injected (file: %s)", mioPersonaFile)

	var shiroSubagentManager agent.SubagentManager
	if subagentMgr != nil {
		shiroSubagentManager = subagentMgr
	}
	shiroAgent := agent.NewShiroAgent(workerProvider, workerToolRunner, mcpClient, cfg.Prompts.Worker, shiroSubagentManager)
	heavyAgent := agent.NewHeavyAgent(heavyProvider, cfg.Prompts.Heavy)
	wildAgent := agent.NewWildAgent(wildProvider, cfg.Prompts.Wild)
	if convEngine != nil {
		shiroAgent.WithConversationEngine(convEngine)
		heavyAgent.WithConversationEngine(convEngine)
		wildAgent.WithConversationEngine(convEngine)
	}
	if cfg.Worker.PersonaFile != "" {
		if content, ok := config.LoadPersonaFile(cfg.WorkspaceDir, cfg.Worker.PersonaFile); ok {
			shiroPersona := agent.AgentPersona{
				Name:        "Shiro",
				Personality: content,
				Tone:        cfg.Worker.Tone,
			}
			shiroAgent.WithPersona(shiroPersona)
			log.Printf("Shiro: persona loaded from %s", cfg.Worker.PersonaFile)
		}
	}
	return agentRuntime{Mio: mioAgent, Shiro: shiroAgent, Heavy: heavyAgent, Wild: wildAgent}
}
