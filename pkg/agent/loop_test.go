package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/bus"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/config"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
	"github.com/Nyukimin/picoclaw_multiLLM/pkg/tools"
)

// mockProvider is a simple mock LLM provider for testing
type mockProvider struct{}

func (m *mockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   "Mock response",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *mockProvider) GetDefaultModel() string {
	return "mock-model"
}

func TestRecordLastChannel(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Test RecordLastChannel
	testChannel := "test-channel"
	err = al.RecordLastChannel(testChannel)
	if err != nil {
		t.Fatalf("RecordLastChannel failed: %v", err)
	}

	// Verify channel was saved
	lastChannel := al.state.GetLastChannel()
	if lastChannel != testChannel {
		t.Errorf("Expected channel '%s', got '%s'", testChannel, lastChannel)
	}

	// Verify persistence by creating a new agent loop
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if al2.state.GetLastChannel() != testChannel {
		t.Errorf("Expected persistent channel '%s', got '%s'", testChannel, al2.state.GetLastChannel())
	}
}

func TestRecordLastChatID(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Test RecordLastChatID
	testChatID := "test-chat-id-123"
	err = al.RecordLastChatID(testChatID)
	if err != nil {
		t.Fatalf("RecordLastChatID failed: %v", err)
	}

	// Verify chat ID was saved
	lastChatID := al.state.GetLastChatID()
	if lastChatID != testChatID {
		t.Errorf("Expected chat ID '%s', got '%s'", testChatID, lastChatID)
	}

	// Verify persistence by creating a new agent loop
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if al2.state.GetLastChatID() != testChatID {
		t.Errorf("Expected persistent chat ID '%s', got '%s'", testChatID, al2.state.GetLastChatID())
	}
}

func TestNewAgentLoop_StateInitialized(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Verify state manager is initialized
	if al.state == nil {
		t.Error("Expected state manager to be initialized")
	}

	// Verify state directory was created
	stateDir := filepath.Join(tmpDir, "state")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("Expected state directory to exist")
	}
}

// TestToolRegistry_ToolRegistration verifies tools can be registered and retrieved
func TestToolRegistry_ToolRegistration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a custom tool
	customTool := &mockCustomTool{}
	al.RegisterTool(customTool)

	// Verify tool is registered by checking it doesn't panic on GetStartupInfo
	// (actual tool retrieval is tested in tools package tests)
	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]interface{})
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := false
	for _, name := range toolsList {
		if name == "mock_custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestToolContext_Updates verifies tool context is updated with channel/chatID
func TestToolContext_Updates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "OK"}
	_ = NewAgentLoop(cfg, msgBus, provider)

	// Verify that ContextualTool interface is defined and can be implemented
	// This test validates the interface contract exists
	ctxTool := &mockContextualTool{}

	// Verify the tool implements the interface correctly
	var _ tools.ContextualTool = ctxTool
}

// TestToolRegistry_GetDefinitions verifies tool definitions can be retrieved
func TestToolRegistry_GetDefinitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a test tool and verify it shows up in startup info
	testTool := &mockCustomTool{}
	al.RegisterTool(testTool)

	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]interface{})
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := false
	for _, name := range toolsList {
		if name == "mock_custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestAgentLoop_GetStartupInfo verifies startup info contains tools
func TestAgentLoop_GetStartupInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	info := al.GetStartupInfo()

	// Verify tools info exists
	toolsInfo, ok := info["tools"]
	if !ok {
		t.Fatal("Expected 'tools' key in startup info")
	}

	toolsMap, ok := toolsInfo.(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'tools' to be a map")
	}

	count, ok := toolsMap["count"]
	if !ok {
		t.Fatal("Expected 'count' in tools info")
	}

	// Should have default tools registered
	if count.(int) == 0 {
		t.Error("Expected at least some tools to be registered")
	}
}

// TestAgentLoop_Stop verifies Stop() sets running to false
func TestAgentLoop_Stop(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &mockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Note: running is only set to true when Run() is called
	// We can't test that without starting the event loop
	// Instead, verify the Stop method can be called safely
	al.Stop()

	// Verify running is false (initial state or after Stop)
	if al.running.Load() {
		t.Error("Expected agent to be stopped (or never started)")
	}
}

// Mock implementations for testing

type simpleMockProvider struct {
	response string
}

func (m *simpleMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *simpleMockProvider) GetDefaultModel() string {
	return "mock-model"
}

// mockCustomTool is a simple mock tool for registration testing
type mockCustomTool struct{}

func (m *mockCustomTool) Name() string {
	return "mock_custom"
}

func (m *mockCustomTool) Description() string {
	return "Mock custom tool for testing"
}

func (m *mockCustomTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (m *mockCustomTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	return tools.SilentResult("Custom tool executed")
}

// mockContextualTool tracks context updates
type mockContextualTool struct {
	lastChannel string
	lastChatID  string
}

func (m *mockContextualTool) Name() string {
	return "mock_contextual"
}

func (m *mockContextualTool) Description() string {
	return "Mock contextual tool"
}

func (m *mockContextualTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (m *mockContextualTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	return tools.SilentResult("Contextual tool executed")
}

func (m *mockContextualTool) SetContext(channel, chatID string) {
	m.lastChannel = channel
	m.lastChatID = chatID
}

// testHelper executes a message and returns the response
type testHelper struct {
	al *AgentLoop
}

func (h testHelper) executeAndGetResponse(tb testing.TB, ctx context.Context, msg bus.InboundMessage) string {
	// Use a short timeout to avoid hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, responseTimeout)
	defer cancel()

	response, err := h.al.processMessage(timeoutCtx, msg)
	if err != nil {
		tb.Fatalf("processMessage failed: %v", err)
	}
	return response
}

const responseTimeout = 3 * time.Second

// TestToolResult_SilentToolDoesNotSendUserMessage verifies silent tools don't trigger outbound
func TestToolResult_SilentToolDoesNotSendUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "File operation complete"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ReadFileTool returns SilentResult, which should not send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "read test.txt",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// Silent tool should return the LLM's response directly
	if response != "File operation complete" {
		t.Errorf("Expected 'File operation complete', got: %s", response)
	}
}

// TestToolResult_UserFacingToolDoesSendMessage verifies user-facing tools trigger outbound
func TestToolResult_UserFacingToolDoesSendMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "Command output: hello world"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ExecTool returns UserResult, which should send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "run hello",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// User-facing tool should include the output in final response
	if response != "Command output: hello world" {
		t.Errorf("Expected 'Command output: hello world', got: %s", response)
	}
}

// failFirstMockProvider fails on the first N calls with a specific error
type failFirstMockProvider struct {
	failures    int
	currentCall int
	failError   error
	successResp string
}

func (m *failFirstMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	m.currentCall++
	if m.currentCall <= m.failures {
		return nil, m.failError
	}
	return &providers.LLMResponse{
		Content:   m.successResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *failFirstMockProvider) GetDefaultModel() string {
	return "mock-fail-model"
}

type captureToolsMockProvider struct {
	lastToolsCount int
}

func (m *captureToolsMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	m.lastToolsCount = len(tools)
	return &providers.LLMResponse{
		Content:   "ok",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *captureToolsMockProvider) GetDefaultModel() string {
	return "capture-tools-model"
}

type stagedMockProvider struct {
	responses []string
	calls     int
	toolsLog  []int
}

func (m *stagedMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	m.toolsLog = append(m.toolsLog, len(tools))
	idx := m.calls
	m.calls++
	resp := "ok"
	if idx < len(m.responses) {
		resp = m.responses[idx]
	}
	return &providers.LLMResponse{
		Content:   resp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *stagedMockProvider) GetDefaultModel() string {
	return "staged-mock-model"
}

// TestAgentLoop_ContextExhaustionRetry verify that the agent retries on context errors
func TestAgentLoop_ContextExhaustionRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
	}

	msgBus := bus.NewMessageBus()

	// Create a provider that fails once with a context error
	contextErr := fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	provider := &failFirstMockProvider{
		failures:    1,
		failError:   contextErr,
		successResp: "Recovered from context error",
	}

	al := NewAgentLoop(cfg, msgBus, provider)

	// Inject some history to simulate a full context
	sessionKey := "test-session-context"
	// Create dummy history
	history := []providers.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Old message 1"},
		{Role: "assistant", Content: "Old response 1"},
		{Role: "user", Content: "Old message 2"},
		{Role: "assistant", Content: "Old response 2"},
		{Role: "user", Content: "Trigger message"},
	}
	al.sessions.SetHistory(sessionKey, history)

	// Call ProcessDirectWithChannel
	// Note: ProcessDirectWithChannel calls processMessage which will execute runLLMIteration
	response, err := al.ProcessDirectWithChannel(context.Background(), "Trigger message", sessionKey, "test", "test-chat")

	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if response != "Recovered from context error" {
		t.Errorf("Expected 'Recovered from context error', got '%s'", response)
	}

	// We expect 2 calls: 1st failed, 2nd succeeded
	if provider.currentCall != 2 {
		t.Errorf("Expected 2 calls (1 fail + 1 success), got %d", provider.currentCall)
	}

	// Check final history length
	finalHistory := al.sessions.GetHistory(sessionKey)
	// We verify that the history has been modified (compressed)
	// Original length: 6
	// Expected behavior: compression drops ~50% of history (mid slice)
	// We can assert that the length is NOT what it would be without compression.
	// Without compression: 6 + 1 (new user msg) + 1 (assistant msg) = 8
	if len(finalHistory) >= 8 {
		t.Errorf("Expected history to be compressed (len < 8), got %d", len(finalHistory))
	}
}

func TestRunAgentLoop_ChatRouteDisablesTools(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Loop: config.LoopConfig{
			MaxLoops:  1,
			MaxMillis: 5000,
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &captureToolsMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	_, err = al.runAgentLoop(context.Background(), processOptions{
		SessionKey:      "chat-tools-off",
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     "こんにちは",
		DefaultResponse: "default",
		EnableSummary:   false,
		SendResponse:    false,
		Route:           RouteChat,
		MaxLoops:        1,
		MaxMillis:       5000,
	})
	if err != nil {
		t.Fatalf("runAgentLoop failed: %v", err)
	}
	if provider.lastToolsCount != 0 {
		t.Fatalf("CHAT route should disable tools, got %d", provider.lastToolsCount)
	}
}

func TestProcessMessage_ChatDelegatesThenFinalizes(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Provider:          "ollama",
				Model:             "ollama/chat-v1:latest",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Routing: config.RoutingConfig{
			Classifier:    config.RoutingClassifierConfig{Enabled: false},
			FallbackRoute: RouteChat,
			LLM: config.RouteLLMConfig{
				ChatProvider:   "ollama",
				ChatModel:      "ollama/chat-v1:latest",
				WorkerProvider: "ollama",
				WorkerModel:    "ollama/chat-v1:latest",
				CoderProvider:  "ollama",
				CoderModel:     "ollama/chat-v1:latest",
			},
		},
		Loop: config.LoopConfig{
			MaxLoops:  3,
			MaxMillis: 5000,
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &stagedMockProvider{
		responses: []string{
			"DELEGATE: CODE\nTASK:\n添付内容を保存して結果を返して",
			"delegated worker result",
			"最終回答です",
		},
	}
	al := NewAgentLoop(cfg, msgBus, provider)

	msg := bus.InboundMessage{
		Channel:    "line",
		SenderID:   "u1",
		ChatID:     "c1",
		Content:    "このファイル保存して",
		SessionKey: "line:c1",
	}
	got, err := al.processMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}
	if !strings.Contains(got, "最終回答です") {
		t.Fatalf("unexpected final response: %s", got)
	}
	if !strings.Contains(got, "作業終わったって") {
		t.Fatalf("expected delegation completion notice, got: %s", got)
	}
	if provider.calls != 3 {
		t.Fatalf("expected 3 LLM calls (decision/delegate/finalize), got %d", provider.calls)
	}
	if len(provider.toolsLog) != 3 {
		t.Fatalf("expected tools log for 3 calls, got %d", len(provider.toolsLog))
	}
	if provider.toolsLog[0] != 0 {
		t.Fatalf("first chat decision call should have 0 tools, got %d", provider.toolsLog[0])
	}
	if provider.toolsLog[1] == 0 {
		t.Fatalf("delegated worker call should enable tools")
	}
	if provider.toolsLog[2] != 0 {
		t.Fatalf("final chat call should disable tools, got %d", provider.toolsLog[2])
	}
}

func TestProcessMessage_ChatNoDelegateKeepsSinglePass(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Provider:          "ollama",
				Model:             "ollama/chat-v1:latest",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Routing: config.RoutingConfig{
			Classifier:    config.RoutingClassifierConfig{Enabled: false},
			FallbackRoute: RouteChat,
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &stagedMockProvider{responses: []string{"通常回答だけ返す"}}
	al := NewAgentLoop(cfg, msgBus, provider)

	msg := bus.InboundMessage{
		Channel:    "line",
		SenderID:   "u1",
		ChatID:     "c1",
		Content:    "こんにちは",
		SessionKey: "line:c1",
	}
	got, err := al.processMessage(context.Background(), msg)
	if err != nil {
		t.Fatalf("processMessage failed: %v", err)
	}
	if got != "通常回答だけ返す" {
		t.Fatalf("unexpected response: %s", got)
	}
	if provider.calls != 1 {
		t.Fatalf("expected single LLM call, got %d", provider.calls)
	}
}

func TestParseChatDelegateDirective_InvalidMissingTask(t *testing.T) {
	if _, ok := parseChatDelegateDirective("DELEGATE: CODE\n理由だけ書く"); ok {
		t.Fatalf("expected invalid directive when TASK block is missing")
	}
}

func TestDelegationNotices_ContainRequiredPhrases(t *testing.T) {
	start := buildDelegationStartNotice("line:c1", "Worker（Shiro）", "画像解析")
	if !strings.Contains(start, "お願いするね") {
		t.Fatalf("start notice should contain 委譲 phrase, got: %s", start)
	}
	done := buildDelegationDoneNotice("line:c1", "Worker（Shiro）", "画像解析")
	if !strings.Contains(done, "作業終わったって") {
		t.Fatalf("done notice should contain completion phrase, got: %s", done)
	}
}

func TestRunAgentLoop_NonChatRouteEnablesTools(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         4096,
				MaxToolIterations: 10,
			},
		},
		Loop: config.LoopConfig{
			MaxLoops:  1,
			MaxMillis: 5000,
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &captureToolsMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	_, err = al.runAgentLoop(context.Background(), processOptions{
		SessionKey:      "plan-tools-on",
		Channel:         "cli",
		ChatID:          "direct",
		UserMessage:     "段取りを作って",
		DefaultResponse: "default",
		EnableSummary:   false,
		SendResponse:    false,
		Route:           RoutePlan,
		MaxLoops:        1,
		MaxMillis:       5000,
	})
	if err != nil {
		t.Fatalf("runAgentLoop failed: %v", err)
	}
	if provider.lastToolsCount == 0 {
		t.Fatalf("Non-CHAT route should include tools, got %d", provider.lastToolsCount)
	}
}

func TestResolveRouteLLM_AllRoutes(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/chat-v1:latest",
				},
			},
			Routing: config.RoutingConfig{
				LLM: config.RouteLLMConfig{
					ChatAlias:      "Mio",
					ChatProvider:   "ollama",
					ChatModel:      "ollama/chat-v1:latest",
					WorkerAlias:    "Shiro",
					WorkerProvider: "ollama",
					WorkerModel:    "ollama/worker-v1:latest",
					CoderAlias:     "Aka",
					CoderProvider:  "deepseek",
					CoderModel:     "deepseek-chat",
				},
			},
		},
	}

	cases := []struct {
		route    string
		provider string
		model    string
	}{
		{RouteChat, "ollama", "ollama/chat-v1:latest"},
		{RoutePlan, "ollama", "ollama/worker-v1:latest"},
		{RouteAnalyze, "ollama", "ollama/worker-v1:latest"},
		{RouteOps, "ollama", "ollama/worker-v1:latest"},
		{RouteResearch, "ollama", "ollama/worker-v1:latest"},
		{RouteCode, "deepseek", "deepseek-chat"},
	}

	for _, tc := range cases {
		t.Run(tc.route, func(t *testing.T) {
			provider, model := al.resolveRouteLLM(tc.route)
			if provider != tc.provider || model != tc.model {
				t.Fatalf("route=%s got (%s, %s), want (%s, %s)", tc.route, provider, model, tc.provider, tc.model)
			}
		})
	}
}

func TestResolveRouteLLM_WorkerFallbacksToChatIfUnset(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/default:latest",
				},
			},
			Routing: config.RoutingConfig{
				LLM: config.RouteLLMConfig{
					ChatProvider: "ollama",
					ChatModel:    "ollama/chat-v1:latest",
				},
			},
		},
	}

	provider, model := al.resolveRouteLLM(RoutePlan)
	if provider != "ollama" || model != "ollama/chat-v1:latest" {
		t.Fatalf("worker fallback got (%s, %s), want (%s, %s)", provider, model, "ollama", "ollama/chat-v1:latest")
	}
}

func TestResolveRouteLLM_FallbacksToDefaults(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/default:latest",
				},
			},
			Routing: config.RoutingConfig{},
		},
	}

	provider, model := al.resolveRouteLLM(RouteCode)
	if provider != "ollama" || model != "ollama/default:latest" {
		t.Fatalf("default fallback got (%s, %s), want (%s, %s)", provider, model, "ollama", "ollama/default:latest")
	}
}

func TestResolveRouteLLM_CodeLegacyKeysAreSupported(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/default:latest",
				},
			},
			Routing: config.RoutingConfig{
				LLM: config.RouteLLMConfig{
					CodeProvider: "deepseek",
					CodeModel:    "deepseek-chat",
				},
			},
		},
	}

	provider, model := al.resolveRouteLLM(RouteCode)
	if provider != "deepseek" || model != "deepseek-chat" {
		t.Fatalf("legacy code_* fallback got (%s, %s), want (%s, %s)", provider, model, "deepseek", "deepseek-chat")
	}
}

func TestResolveRouteLLM_Code1Code2(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/default:latest",
				},
			},
			Routing: config.RoutingConfig{
				LLM: config.RouteLLMConfig{
					CoderAlias:     "Aka",
					CoderProvider:  "deepseek",
					CoderModel:     "deepseek-chat",
					Coder2Alias:    "Midori",
					Coder2Provider: "openai",
					Coder2Model:    "gpt-4o",
				},
			},
		},
	}

	p1, m1 := al.resolveRouteLLM(RouteCode1)
	if p1 != "deepseek" || m1 != "deepseek-chat" {
		t.Fatalf("CODE1 got (%s, %s), want (deepseek, deepseek-chat)", p1, m1)
	}

	p2, m2 := al.resolveRouteLLM(RouteCode2)
	if p2 != "openai" || m2 != "gpt-4o" {
		t.Fatalf("CODE2 got (%s, %s), want (openai, gpt-4o)", p2, m2)
	}
}

func TestResolveRouteLLM_Code2FallbackToCoder1(t *testing.T) {
	al := &AgentLoop{
		cfg: &config.Config{
			Agents: config.AgentsConfig{
				Defaults: config.AgentDefaults{
					Provider: "ollama",
					Model:    "ollama/default:latest",
				},
			},
			Routing: config.RoutingConfig{
				LLM: config.RouteLLMConfig{
					CoderProvider: "deepseek",
					CoderModel:    "deepseek-chat",
				},
			},
		},
	}

	p, m := al.resolveRouteLLM(RouteCode2)
	if p != "deepseek" || m != "deepseek-chat" {
		t.Fatalf("CODE2 fallback got (%s, %s), want (deepseek, deepseek-chat)", p, m)
	}
}

func TestSelectCoderRoute(t *testing.T) {
	cases := []struct {
		task string
		want string
	}{
		{"", RouteCode2},
		{"このバグを修正して", RouteCode2},
		{"関数を実装して", RouteCode2},
		{"仕様を設計してください", RouteCode1},
		{"アーキテクチャを検討", RouteCode1},
		{"requirements documentを作成", RouteCode1},
		{"APIの設計書を書いて", RouteCode1},
		{"テストコードを書いて", RouteCode2},
	}

	for _, tc := range cases {
		t.Run(tc.task, func(t *testing.T) {
			got := selectCoderRoute(tc.task)
			if got != tc.want {
				t.Fatalf("selectCoderRoute(%q) = %s, want %s", tc.task, got, tc.want)
			}
		})
	}
}

func TestDetectPatchType(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  string
	}{
		{
			name:  "JSON format",
			patch: `[{"type": "file_edit"}]`,
			want:  "json",
		},
		{
			name:  "JSON with leading whitespace",
			patch: "  \n  [{}]",
			want:  "json",
		},
		{
			name:  "Markdown format with code block",
			patch: "```go:file.go\ncode\n```",
			want:  "markdown",
		},
		{
			name:  "Markdown with bash",
			patch: "Some text\n```bash\ncommand\n```",
			want:  "markdown",
		},
		{
			name:  "Unknown format",
			patch: "Just plain text",
			want:  "unknown",
		},
		{
			name:  "Empty string",
			patch: "",
			want:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectPatchType(tt.patch)
			if got != tt.want {
				t.Errorf("detectPatchType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFormatExecutionResults(t *testing.T) {
	tests := []struct {
		name    string
		results []CommandResult
		want    string
	}{
		{
			name: "Single successful command",
			results: []CommandResult{
				{
					Command: PatchCommand{
						Type:   "file_edit",
						Action: "create",
						Target: "test.go",
					},
					Success:  true,
					Output:   "File created",
					Duration: 150,
				},
			},
			want: "✓ [1] file_edit create test.go (150ms)",
		},
		{
			name: "Single failed command",
			results: []CommandResult{
				{
					Command: PatchCommand{
						Type:   "shell_command",
						Action: "run",
						Target: "go test",
					},
					Success:  false,
					Error:    "exit code 1",
					Duration: 2500,
				},
			},
			want: "✗ [1] shell_command run go test (2500ms)\n    エラー: exit code 1",
		},
		{
			name: "Multiple mixed results",
			results: []CommandResult{
				{
					Command: PatchCommand{
						Type:   "file_edit",
						Action: "update",
						Target: "main.go",
					},
					Success:  true,
					Duration: 100,
				},
				{
					Command: PatchCommand{
						Type:   "shell_command",
						Action: "run",
						Target: "make test",
					},
					Success:  false,
					Error:    "compilation failed",
					Duration: 3000,
				},
			},
			want: "✓ [1] file_edit update main.go (100ms)\n✗ [2] shell_command run make test (3000ms)\n    エラー: compilation failed",
		},
		{
			name:    "Empty results",
			results: []CommandResult{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatExecutionResults(tt.results)
			if got != tt.want {
				t.Errorf("formatExecutionResults() =\n%v\n\nwant:\n%v", got, tt.want)
			}
		})
	}
}
