package chat

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/modules"
)

func TestLightweightReceptionModule(t *testing.T) {
	module := NewLightweightReceptionModule()

	// Test initialization
	agent := &modules.AgentCore{
		ID:    "chat",
		Alias: "Mio",
	}

	err := module.Initialize(context.Background(), agent)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test Name
	if module.Name() != "LightweightReception" {
		t.Errorf("Expected name 'LightweightReception', got '%s'", module.Name())
	}

	// Test ReceiveTask
	msg := bus.InboundMessage{
		Content: "テストメッセージ",
		Channel: "test",
	}

	task := module.ReceiveTask(msg, "job_20260301_001")

	if task.JobID != "job_20260301_001" {
		t.Errorf("Expected JobID 'job_20260301_001', got '%s'", task.JobID)
	}

	if task.UserText != "テストメッセージ" {
		t.Errorf("Expected UserText 'テストメッセージ', got '%s'", task.UserText)
	}

	if task.Metadata == nil {
		t.Error("Expected Metadata to be initialized")
	}

	// Test Shutdown
	err = module.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}
