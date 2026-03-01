// Package chat provides modules for the Chat agent (Mio).
package chat

import (
	"context"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

// Task represents a work unit received by Chat and delegated to Worker.
type Task struct {
	JobID       string
	UserText    string
	Source      bus.InboundMessage
	ReceivedAt  time.Time
	Metadata    map[string]interface{}
}

// LightweightReceptionModule handles receiving user messages and creating tasks.
// In the new architecture, Chat only performs lightweight reception and delegates
// routing decisions to Worker.
type LightweightReceptionModule struct {
	agent *modules.AgentCore
}

// NewLightweightReceptionModule creates a new lightweight reception module.
func NewLightweightReceptionModule() *LightweightReceptionModule {
	return &LightweightReceptionModule{}
}

// Name returns the module name.
func (m *LightweightReceptionModule) Name() string {
	return "LightweightReception"
}

// Initialize initializes the module with the agent core.
func (m *LightweightReceptionModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	return nil
}

// Shutdown cleans up module resources.
func (m *LightweightReceptionModule) Shutdown(ctx context.Context) error {
	return nil
}

// ReceiveTask creates a Task from an inbound message.
// This is a lightweight operation that only packages the message with a JobID.
func (m *LightweightReceptionModule) ReceiveTask(msg bus.InboundMessage, jobID string) Task {
	return Task{
		JobID:      jobID,
		UserText:   msg.Content,
		Source:     msg,
		ReceivedAt: time.Now(),
		Metadata:   make(map[string]interface{}),
	}
}
