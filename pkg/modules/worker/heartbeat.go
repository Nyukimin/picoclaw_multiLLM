package worker

import (
	"context"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/logger"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

// AgentStatus represents the current status of an agent.
type AgentStatus struct {
	AgentID  string
	Alias    string
	Status   string // "idle", "processing", "timeout"
	LastSeen time.Time
	JobID    string
}

// HeartbeatReport contains the status of all agents.
type HeartbeatReport struct {
	Timestamp time.Time
	Agents    []AgentStatus
}

// HeartbeatCollectorModule collects and monitors heartbeats from all agents.
// This module tracks agent liveness and detects timeouts.
type HeartbeatCollectorModule struct {
	agent       *modules.AgentCore
	agents      map[string]*AgentStatus
	mu          sync.RWMutex
	timeout     time.Duration
	lastCleanup time.Time
}

// NewHeartbeatCollectorModule creates a new heartbeat collector.
func NewHeartbeatCollectorModule() *HeartbeatCollectorModule {
	return &HeartbeatCollectorModule{
		agents:  make(map[string]*AgentStatus),
		timeout: 60 * time.Second,
	}
}

// Name returns the module name.
func (m *HeartbeatCollectorModule) Name() string {
	return "HeartbeatCollector"
}

// Initialize initializes the module with the agent core.
func (m *HeartbeatCollectorModule) Initialize(ctx context.Context, agent *modules.AgentCore) error {
	m.agent = agent
	m.lastCleanup = time.Now()

	logger.InfoCF("heartbeat", "collector.initialized", map[string]interface{}{
		"timeout_seconds": m.timeout.Seconds(),
	})

	return nil
}

// Shutdown cleans up module resources.
func (m *HeartbeatCollectorModule) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.agents = make(map[string]*AgentStatus)
	return nil
}

// ReportHeartbeat records a heartbeat from an agent.
func (m *HeartbeatCollectorModule) ReportHeartbeat(agentID, alias, status, jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	m.agents[agentID] = &AgentStatus{
		AgentID:  agentID,
		Alias:    alias,
		Status:   status,
		LastSeen: now,
		JobID:    jobID,
	}

	// Periodic cleanup of old entries
	if time.Since(m.lastCleanup) > 5*time.Minute {
		m.cleanupOldEntries(now)
		m.lastCleanup = now
	}
}

// GetReport generates a heartbeat report for all known agents.
func (m *HeartbeatCollectorModule) GetReport() HeartbeatReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	report := HeartbeatReport{
		Timestamp: now,
		Agents:    make([]AgentStatus, 0, len(m.agents)),
	}

	for _, status := range m.agents {
		// Check for timeout
		currentStatus := status.Status
		if time.Since(status.LastSeen) > m.timeout {
			currentStatus = "timeout"
		}

		report.Agents = append(report.Agents, AgentStatus{
			AgentID:  status.AgentID,
			Alias:    status.Alias,
			Status:   currentStatus,
			LastSeen: status.LastSeen,
			JobID:    status.JobID,
		})
	}

	return report
}

// cleanupOldEntries removes agents that haven't been seen in a long time.
// Must be called with lock held.
func (m *HeartbeatCollectorModule) cleanupOldEntries(now time.Time) {
	threshold := 10 * time.Minute
	for agentID, status := range m.agents {
		if time.Since(status.LastSeen) > threshold {
			logger.InfoCF("heartbeat", "collector.cleanup", map[string]interface{}{
				"agent_id":  agentID,
				"last_seen": status.LastSeen.Format(time.RFC3339),
			})
			delete(m.agents, agentID)
		}
	}
}
