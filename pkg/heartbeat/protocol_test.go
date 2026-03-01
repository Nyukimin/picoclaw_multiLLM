package heartbeat

import (
	"testing"
	"time"
)

func TestHeartbeatBus(t *testing.T) {
	bus := NewHeartbeatBus()

	// Test Report and GetRecentHeartbeats
	hb1 := AgentHeartbeat{
		AgentID:   "chat",
		JobID:     "job_001",
		Status:    "idle",
		Timestamp: time.Now(),
	}

	hb2 := AgentHeartbeat{
		AgentID:   "worker",
		JobID:     "job_002",
		Status:    "processing",
		Timestamp: time.Now(),
	}

	bus.Report(hb1)
	bus.Report(hb2)

	// Get recent heartbeats
	recent := bus.GetRecentHeartbeats("chat", 10)
	if len(recent) != 1 {
		t.Errorf("Expected 1 heartbeat, got %d", len(recent))
	}

	if recent[0].AgentID != "chat" {
		t.Errorf("Expected agentID 'chat', got '%s'", recent[0].AgentID)
	}

	// Test GetAllAgents
	agents := bus.GetAllAgents()
	if len(agents) != 2 {
		t.Errorf("Expected 2 agents, got %d", len(agents))
	}
}

func TestHeartbeatBus_Subscribe(t *testing.T) {
	bus := NewHeartbeatBus()

	// Subscribe to chat agent
	ch := bus.Subscribe("chat")

	// Report heartbeat
	hb := AgentHeartbeat{
		AgentID:   "chat",
		Status:    "idle",
		Timestamp: time.Now(),
	}

	bus.Report(hb)

	// Receive heartbeat
	select {
	case received := <-ch:
		if received.AgentID != "chat" {
			t.Errorf("Expected agentID 'chat', got '%s'", received.AgentID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive heartbeat")
	}
}

func TestHeartbeatBus_WildcardSubscribe(t *testing.T) {
	bus := NewHeartbeatBus()

	// Subscribe to all agents
	ch := bus.Subscribe("*")

	// Report heartbeats from different agents
	hb1 := AgentHeartbeat{AgentID: "chat", Status: "idle", Timestamp: time.Now()}
	hb2 := AgentHeartbeat{AgentID: "worker", Status: "processing", Timestamp: time.Now()}

	bus.Report(hb1)
	bus.Report(hb2)

	// Should receive both
	count := 0
	for i := 0; i < 2; i++ {
		select {
		case <-ch:
			count++
		case <-time.After(100 * time.Millisecond):
			t.Error("Expected to receive heartbeat")
		}
	}

	if count != 2 {
		t.Errorf("Expected 2 heartbeats, got %d", count)
	}
}

func TestHeartbeatBus_BufferLimit(t *testing.T) {
	bus := NewHeartbeatBus()
	bus.bufferSize = 5

	// Report more heartbeats than buffer size
	for i := 0; i < 10; i++ {
		hb := AgentHeartbeat{
			AgentID:   "chat",
			JobID:     "job_" + string(rune('0'+i)),
			Timestamp: time.Now(),
		}
		bus.Report(hb)
	}

	// Should only keep last 5
	recent := bus.GetRecentHeartbeats("chat", 100)
	if len(recent) != 5 {
		t.Errorf("Expected 5 heartbeats in buffer, got %d", len(recent))
	}
}
