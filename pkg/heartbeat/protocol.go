// Package heartbeat provides the heartbeat protocol for agent monitoring.
package heartbeat

import (
	"sync"
	"time"
)

// AgentHeartbeat represents a heartbeat from an agent.
type AgentHeartbeat struct {
	AgentID   string
	JobID     string
	Status    string // "idle", "processing", "waiting"
	Timestamp time.Time
	Metadata  map[string]interface{}
}

// HeartbeatBus manages heartbeat messages between agents.
type HeartbeatBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan AgentHeartbeat
	buffer      map[string][]AgentHeartbeat
	bufferSize  int
}

// NewHeartbeatBus creates a new heartbeat bus.
func NewHeartbeatBus() *HeartbeatBus {
	return &HeartbeatBus{
		subscribers: make(map[string][]chan AgentHeartbeat),
		buffer:      make(map[string][]AgentHeartbeat),
		bufferSize:  100,
	}
}

// Report sends a heartbeat to the bus.
func (hb *HeartbeatBus) Report(heartbeat AgentHeartbeat) {
	hb.mu.Lock()
	defer hb.mu.Unlock()

	// Add to buffer
	agentID := heartbeat.AgentID
	hb.buffer[agentID] = append(hb.buffer[agentID], heartbeat)

	// Trim buffer if too large
	if len(hb.buffer[agentID]) > hb.bufferSize {
		hb.buffer[agentID] = hb.buffer[agentID][len(hb.buffer[agentID])-hb.bufferSize:]
	}

	// Send to subscribers
	if subs, exists := hb.subscribers[agentID]; exists {
		for _, ch := range subs {
			select {
			case ch <- heartbeat:
			default:
				// Channel full, skip
			}
		}
	}

	// Send to wildcard subscribers (subscribed to all agents)
	if subs, exists := hb.subscribers["*"]; exists {
		for _, ch := range subs {
			select {
			case ch <- heartbeat:
			default:
				// Channel full, skip
			}
		}
	}
}

// Subscribe subscribes to heartbeats from a specific agent.
// Use "*" as agentID to subscribe to all agents.
func (hb *HeartbeatBus) Subscribe(agentID string) <-chan AgentHeartbeat {
	hb.mu.Lock()
	defer hb.mu.Unlock()

	ch := make(chan AgentHeartbeat, 10)
	hb.subscribers[agentID] = append(hb.subscribers[agentID], ch)

	return ch
}

// Unsubscribe removes a subscription.
func (hb *HeartbeatBus) Unsubscribe(agentID string, ch <-chan AgentHeartbeat) {
	hb.mu.Lock()
	defer hb.mu.Unlock()

	if subs, exists := hb.subscribers[agentID]; exists {
		for i, sub := range subs {
			if sub == ch {
				hb.subscribers[agentID] = append(subs[:i], subs[i+1:]...)
				close(sub)
				break
			}
		}
	}
}

// GetRecentHeartbeats returns recent heartbeats for an agent.
func (hb *HeartbeatBus) GetRecentHeartbeats(agentID string, count int) []AgentHeartbeat {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	buffer := hb.buffer[agentID]
	if len(buffer) == 0 {
		return nil
	}

	if count > len(buffer) {
		count = len(buffer)
	}

	// Return most recent N heartbeats
	start := len(buffer) - count
	result := make([]AgentHeartbeat, count)
	copy(result, buffer[start:])

	return result
}

// GetAllAgents returns a list of all agents that have sent heartbeats.
func (hb *HeartbeatBus) GetAllAgents() []string {
	hb.mu.RLock()
	defer hb.mu.RUnlock()

	agents := make([]string, 0, len(hb.buffer))
	for agentID := range hb.buffer {
		agents = append(agents, agentID)
	}

	return agents
}
