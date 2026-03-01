package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestHeartbeatCollectorModule(t *testing.T) {
	module := NewHeartbeatCollectorModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "worker",
		Alias: "Shiro",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "HeartbeatCollector" {
		t.Errorf("Expected name 'HeartbeatCollector', got '%s'", module.Name())
	}

	// Test ReportHeartbeat
	module.ReportHeartbeat("chat", "Mio", "idle", "")
	module.ReportHeartbeat("worker", "Shiro", "processing", "job_20260301_001")
	module.ReportHeartbeat("order1", "Aka", "idle", "")

	// Test GetReport
	report := module.GetReport()

	if len(report.Agents) != 3 {
		t.Errorf("Expected 3 agents in report, got %d", len(report.Agents))
	}

	// Check that all agents are present
	agentMap := make(map[string]AgentStatus)
	for _, status := range report.Agents {
		agentMap[status.AgentID] = status
	}

	if _, exists := agentMap["chat"]; !exists {
		t.Error("Expected chat agent in report")
	}

	if _, exists := agentMap["worker"]; !exists {
		t.Error("Expected worker agent in report")
	}

	if status, exists := agentMap["worker"]; exists {
		if status.JobID != "job_20260301_001" {
			t.Errorf("Expected worker JobID 'job_20260301_001', got '%s'", status.JobID)
		}
	}

	// Test timeout detection
	module.timeout = 1 * time.Millisecond
	time.Sleep(10 * time.Millisecond)

	report = module.GetReport()
	for _, status := range report.Agents {
		if status.Status != "timeout" {
			t.Errorf("Expected status 'timeout' for %s, got '%s'", status.AgentID, status.Status)
		}
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	// After shutdown, agents map should be empty
	report = module.GetReport()
	if len(report.Agents) != 0 {
		t.Errorf("Expected 0 agents after shutdown, got %d", len(report.Agents))
	}
}
