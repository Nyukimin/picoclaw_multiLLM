// Package modules provides the module interface for the multi-agent architecture.
// Modules define the behavior and capabilities of individual agents.
package modules

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/session"
)

// Module defines the interface that all agent modules must implement.
// Modules are pluggable components that provide specific functionality to agents.
type Module interface {
	// Name returns the unique identifier for this module.
	Name() string

	// Initialize is called when the module is attached to an agent.
	// It receives the agent core for accessing shared resources.
	Initialize(ctx context.Context, agent *AgentCore) error

	// Shutdown is called when the agent is being terminated.
	// Modules should clean up any resources they hold.
	Shutdown(ctx context.Context) error
}

// AgentCore represents the shared core functionality available to all agents.
// Five agents (Chat, Worker, Order1, Order2, Order3) share this common structure,
// with their behavior differentiated by the attached modules.
type AgentCore struct {
	// ID is the unique identifier for this agent ("chat", "worker", "order1", "order2", "order3")
	ID string

	// Alias is the friendly name for this agent (e.g., "Mio", "Shiro", "Aka", "Ao", "Gin")
	Alias string

	// Provider is the LLM provider used by this agent
	Provider providers.LLMProvider

	// Model is the specific model to use (e.g., "chat-v1:latest", "claude-sonnet-4-5")
	Model string

	// Modules are the functional components attached to this agent
	Modules []Module

	// Config provides access to the global configuration
	Config *config.Config

	// Bus is the message bus for inter-agent communication
	Bus *bus.MessageBus

	// Sessions manages conversation sessions
	Sessions *session.SessionManager
}

// NewAgentCore creates a new agent core with the specified parameters.
func NewAgentCore(id, alias string, provider providers.LLMProvider, model string,
	cfg *config.Config, bus *bus.MessageBus, sessions *session.SessionManager) *AgentCore {
	return &AgentCore{
		ID:       id,
		Alias:    alias,
		Provider: provider,
		Model:    model,
		Modules:  make([]Module, 0),
		Config:   cfg,
		Bus:      bus,
		Sessions: sessions,
	}
}

// AttachModule adds a module to this agent and initializes it.
func (ac *AgentCore) AttachModule(ctx context.Context, module Module) error {
	if err := module.Initialize(ctx, ac); err != nil {
		return err
	}
	ac.Modules = append(ac.Modules, module)
	return nil
}

// ShutdownAll calls Shutdown on all attached modules.
func (ac *AgentCore) ShutdownAll(ctx context.Context) error {
	for _, module := range ac.Modules {
		if err := module.Shutdown(ctx); err != nil {
			return err
		}
	}
	return nil
}
