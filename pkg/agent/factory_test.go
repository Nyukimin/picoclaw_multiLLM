package agent

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/session"
)

func TestNewAgentWithModules(t *testing.T) {
	cfg := config.DefaultConfig()
	msgBus := bus.NewMessageBus()
	sessions := session.NewSessionManager("/tmp/test_sessions")
	router := NewRouter(cfg.Routing, nil) // nil classifier for test

	testCases := []struct {
		name           string
		agentID        string
		expectedAlias  string
		expectedModuleCount int
	}{
		{
			name:           "Chat agent",
			agentID:        "chat",
			expectedAlias:  "Mio",
			expectedModuleCount: 2, // Reception + Decision
		},
		{
			name:           "Worker agent",
			agentID:        "worker",
			expectedAlias:  "Shiro",
			expectedModuleCount: 3, // Routing + Execution + Aggregation
		},
		{
			name:           "Order1 agent",
			agentID:        "order1",
			expectedAlias:  "Aka",
			expectedModuleCount: 2, // Proposal + Analysis
		},
		{
			name:           "Order2 agent",
			agentID:        "order2",
			expectedAlias:  "Ao",
			expectedModuleCount: 2, // Proposal + Analysis
		},
		{
			name:           "Order3 agent",
			agentID:        "order3",
			expectedAlias:  "Gin",
			expectedModuleCount: 3, // Proposal + Analysis + Approval
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			agent, err := NewAgentWithModules(context.Background(), tc.agentID, cfg, msgBus, sessions, router)
			if err != nil {
				t.Fatalf("Failed to create agent: %v", err)
			}

			if agent.ID != tc.agentID {
				t.Errorf("Expected ID %s, got %s", tc.agentID, agent.ID)
			}

			if agent.Alias != tc.expectedAlias {
				t.Errorf("Expected alias %s, got %s", tc.expectedAlias, agent.Alias)
			}

			if len(agent.Modules) != tc.expectedModuleCount {
				t.Errorf("Expected %d modules, got %d", tc.expectedModuleCount, len(agent.Modules))
			}
		})
	}
}

func TestResolveAlias(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		id       string
		expected string
	}{
		{"chat", "Mio"},
		{"worker", "Shiro"},
		{"order1", "Aka"},
		{"order2", "Ao"},
		{"order3", "Gin"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			alias := resolveAlias(tt.id, cfg)
			if alias != tt.expected {
				t.Errorf("resolveAlias(%s) = %s; want %s", tt.id, alias, tt.expected)
			}
		})
	}
}

func TestResolveModel(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		id       string
		expected string
	}{
		{"chat", cfg.Routing.LLM.ChatModel},
		{"worker", cfg.Routing.LLM.WorkerModel},
		{"order1", cfg.Routing.LLM.CoderModel},
		{"order2", cfg.Routing.LLM.Coder2Model},
		{"order3", cfg.Routing.LLM.Coder3Model},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			model := resolveModel(tt.id, cfg)
			if model != tt.expected {
				t.Errorf("resolveModel(%s) = %s; want %s", tt.id, model, tt.expected)
			}
		})
	}
}
