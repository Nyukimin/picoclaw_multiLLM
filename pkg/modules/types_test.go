package modules

import (
	"context"
	"testing"
)

// mockModule is a test implementation of Module interface
type mockModule struct {
	name            string
	initCalled      bool
	shutdownCalled  bool
	initError       error
	shutdownError   error
}

func (m *mockModule) Name() string {
	return m.name
}

func (m *mockModule) Initialize(ctx context.Context, agent *AgentCore) error {
	m.initCalled = true
	return m.initError
}

func (m *mockModule) Shutdown(ctx context.Context) error {
	m.shutdownCalled = true
	return m.shutdownError
}

func TestNewAgentCore(t *testing.T) {
	core := NewAgentCore("test-agent", "TestBot", nil, "test-model", nil, nil, nil)

	if core.ID != "test-agent" {
		t.Errorf("Expected ID 'test-agent', got '%s'", core.ID)
	}

	if core.Alias != "TestBot" {
		t.Errorf("Expected Alias 'TestBot', got '%s'", core.Alias)
	}

	if core.Model != "test-model" {
		t.Errorf("Expected Model 'test-model', got '%s'", core.Model)
	}

	if core.Modules == nil {
		t.Error("Modules should be initialized to empty slice, got nil")
	}

	if len(core.Modules) != 0 {
		t.Errorf("Modules should be empty initially, got %d modules", len(core.Modules))
	}
}

func TestAgentCore_AttachModule(t *testing.T) {
	core := NewAgentCore("test-agent", "TestBot", nil, "test-model", nil, nil, nil)

	module := &mockModule{name: "test-module"}

	err := core.AttachModule(context.Background(), module)
	if err != nil {
		t.Errorf("AttachModule should succeed, got error: %v", err)
	}

	if !module.initCalled {
		t.Error("Module Initialize should have been called")
	}

	if len(core.Modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(core.Modules))
	}

	if core.Modules[0] != module {
		t.Error("Attached module should be the same instance")
	}
}

func TestAgentCore_AttachModule_Error(t *testing.T) {
	core := NewAgentCore("test-agent", "TestBot", nil, "test-model", nil, nil, nil)

	module := &mockModule{
		name:      "failing-module",
		initError: context.Canceled,
	}

	err := core.AttachModule(context.Background(), module)
	if err == nil {
		t.Error("AttachModule should fail when Initialize returns error")
	}

	if len(core.Modules) != 0 {
		t.Errorf("Module should not be attached on error, got %d modules", len(core.Modules))
	}
}

func TestAgentCore_ShutdownAll(t *testing.T) {
	core := NewAgentCore("test-agent", "TestBot", nil, "test-model", nil, nil, nil)

	module1 := &mockModule{name: "module1"}
	module2 := &mockModule{name: "module2"}
	module3 := &mockModule{name: "module3"}

	core.AttachModule(context.Background(), module1)
	core.AttachModule(context.Background(), module2)
	core.AttachModule(context.Background(), module3)

	err := core.ShutdownAll(context.Background())
	if err != nil {
		t.Errorf("ShutdownAll should succeed, got error: %v", err)
	}

	if !module1.shutdownCalled {
		t.Error("Module1 Shutdown should have been called")
	}

	if !module2.shutdownCalled {
		t.Error("Module2 Shutdown should have been called")
	}

	if !module3.shutdownCalled {
		t.Error("Module3 Shutdown should have been called")
	}
}

func TestAgentCore_ShutdownAll_Error(t *testing.T) {
	core := NewAgentCore("test-agent", "TestBot", nil, "test-model", nil, nil, nil)

	module1 := &mockModule{name: "module1"}
	module2 := &mockModule{
		name:          "module2",
		shutdownError: context.Canceled,
	}

	core.AttachModule(context.Background(), module1)
	core.AttachModule(context.Background(), module2)

	err := core.ShutdownAll(context.Background())
	if err == nil {
		t.Error("ShutdownAll should fail when any module returns error")
	}

	// First module should still be called
	if !module1.shutdownCalled {
		t.Error("Module1 Shutdown should have been called even if module2 fails")
	}
}
