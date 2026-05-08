//go:build e2e

package autonomousverification

import (
	"context"
	"os"
	"reflect"
	"sync"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/service"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/execution"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/deepseek"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/ollama"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/llm/providers/openai"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/mcp"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/session"
	infrarouting "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/tools"
)

// ========================================
// Config Helper
// ========================================

// getConfig は本番と同じ経路で config を読み込む
// .env → 環境変数 → config.yaml ${ENV_VAR} 展開 → Config struct
func getConfig(t *testing.T) *config.Config {
	t.Helper()
	configPath := os.Getenv("PICOCLAW_CONFIG")
	if configPath == "" {
		configPath = "../../config/config.yaml"
	}
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	return cfg
}

// ========================================
// Stage Recorder (autonomous executor stage遷移記録)
// ========================================

// StageRecorder は autonomous executor の stage 遷移を記録する
type StageRecorder struct {
	mu     sync.Mutex
	stages []string // "received", "contract_ready", "planning", "applying", "verifying", "repairing", "completed", "failed"
}

// NewStageRecorder creates a new StageRecorder
func NewStageRecorder() *StageRecorder {
	return &StageRecorder{
		stages: make([]string, 0),
	}
}

// Record records a stage transition
func (r *StageRecorder) Record(stage string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages = append(r.stages, stage)
}

// GetSequence returns a copy of the stage sequence
func (r *StageRecorder) GetSequence() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.stages...)
}

// Reset clears the recorded stages
func (r *StageRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stages = make([]string, 0)
}

// ========================================
// Mock Report Store (execution report記録)
// ========================================

// MockReportStore は execution report を記録する in-memory store
type MockReportStore struct {
	mu      sync.Mutex
	reports []execution.ExecutionReport
}

// NewMockReportStore creates a new MockReportStore
func NewMockReportStore() *MockReportStore {
	return &MockReportStore{
		reports: make([]execution.ExecutionReport, 0),
	}
}

// Save saves an execution report (implements orchestrator.ReportStore)
func (m *MockReportStore) Save(ctx context.Context, report execution.ExecutionReport) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, report)
	return nil
}

// GetLastReport returns the most recent report, or (empty, false) if no reports exist
func (m *MockReportStore) GetLastReport() (execution.ExecutionReport, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.reports) == 0 {
		return execution.ExecutionReport{}, false
	}

	return m.reports[len(m.reports)-1], true
}

// GetAllReports returns all reports (for debugging)
func (m *MockReportStore) GetAllReports() []execution.ExecutionReport {
	m.mu.Lock()
	defer m.mu.Unlock()

	return append([]execution.ExecutionReport{}, m.reports...)
}

// Reset clears all reports
func (m *MockReportStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = make([]execution.ExecutionReport, 0)
}

// ========================================
// Event Capture (orchestrator event記録)
// ========================================

// EventCapture は orchestrator event を記録する
type EventCapture struct {
	mu     sync.Mutex
	events []orchestrator.OrchestratorEvent
}

// NewEventCapture creates a new EventCapture
func NewEventCapture() *EventCapture {
	return &EventCapture{
		events: make([]orchestrator.OrchestratorEvent, 0),
	}
}

// OnEvent implements EventListener interface
func (e *EventCapture) OnEvent(event orchestrator.OrchestratorEvent) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, event)
}

// FindEvent finds the first event with the given type
func (e *EventCapture) FindEvent(eventType string) (orchestrator.OrchestratorEvent, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, ev := range e.events {
		if ev.Type == eventType {
			return ev, true
		}
	}

	return orchestrator.OrchestratorEvent{}, false
}

// GetAllEvents returns all events (for debugging)
func (e *EventCapture) GetAllEvents() []orchestrator.OrchestratorEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	return append([]orchestrator.OrchestratorEvent{}, e.events...)
}

// Reset clears all events
func (e *EventCapture) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = make([]orchestrator.OrchestratorEvent, 0)
}

// ========================================
// Coder Adapter (既存routing_test.goから流用)
// ========================================

// coderAdapter は main.go と同じアダプター
type coderAdapter struct {
	domainCoder *agent.CoderAgent
}

func (a *coderAdapter) Generate(ctx context.Context, t task.Task, systemPrompt string) (string, error) {
	return a.domainCoder.GenerateWithPrompt(ctx, t, systemPrompt)
}

// ========================================
// Orchestrator Builder
// ========================================

// buildTestOrchestrator は本番同等の Orchestrator を構築する（テスト用mock注入可能版）
// cfg は基本設定、mockMio/mockShiro/mockCoder1/mockCoder2/mockCoder3 は nil の場合は実装を使う
func buildTestOrchestrator(
	t *testing.T,
	cfg *config.Config,
	mockMio orchestrator.MioAgent,
	mockShiro orchestrator.ShiroAgent,
	mockCoder1, mockCoder2, mockCoder3 orchestrator.CoderAgent,
	reportStore orchestrator.ReportStore,
	eventListener orchestrator.EventListener,
) *orchestrator.MessageOrchestrator {
	t.Helper()

	// Mio (Chat) agent
	var mioAgent orchestrator.MioAgent
	if mockMio != nil {
		mioAgent = mockMio
	} else {
		ollamaProvider := ollama.NewOllamaProvider(cfg.Ollama.BaseURL, cfg.Ollama.Model)
		classifier := infrarouting.NewLLMClassifier(ollamaProvider, cfg.Prompts.Classifier)
		ruleDictionary := infrarouting.NewRuleDictionary()
		chatToolRunner := tools.NewToolRunner(tools.ToolRunnerConfig{
			GoogleAPIKey:         cfg.GoogleSearchChat.APIKey,
			GoogleSearchEngineID: cfg.GoogleSearchChat.SearchEngineID,
		})
		mcpClient := mcp.NewMCPClient()
		mioAgent = agent.NewMioAgent(ollamaProvider, classifier, ruleDictionary, chatToolRunner, mcpClient, nil)
	}

	// Shiro (Worker) agent
	var shiroAgent orchestrator.ShiroAgent
	if mockShiro != nil {
		shiroAgent = mockShiro
	} else {
		// Real Shiro implementation (Ollama worker model)
		// Note: 実際のOllama呼び出しはE2Eテストでは重いので、通常はmockを推奨
		shiroAgent = nil
	}

	// Coder1 (DeepSeek) — CODE ルートのデフォルト
	var coder1 orchestrator.CoderAgent
	if mockCoder1 != nil {
		coder1 = mockCoder1
	} else if cfg.DeepSeek.APIKey != "" {
		dp := deepseek.NewDeepSeekProvider(cfg.DeepSeek.APIKey, cfg.DeepSeek.Model)
		dc := agent.NewCoderAgent(dp, nil, nil, cfg.Prompts.CoderProposal)
		coder1 = &coderAdapter{domainCoder: dc}
	}

	// Coder2 (OpenAI)
	var coder2 orchestrator.CoderAgent
	if mockCoder2 != nil {
		coder2 = mockCoder2
	} else if cfg.OpenAI.APIKey != "" {
		op := openai.NewOpenAIProvider(cfg.OpenAI.APIKey, cfg.OpenAI.Model)
		dc := agent.NewCoderAgent(op, nil, nil, cfg.Prompts.CoderProposal)
		coder2 = &coderAdapter{domainCoder: dc}
	}

	// Coder3 (Claude) — mockのみサポート（本番はSSH transport経由）
	var coder3 orchestrator.CoderAgent
	if mockCoder3 != nil {
		coder3 = mockCoder3
	}

	sessionRepo := session.NewJSONSessionRepository(t.TempDir())
	workerExec := service.NewWorkerExecutionService(cfg.Worker)

	orch := orchestrator.NewMessageOrchestrator(
		sessionRepo, mioAgent, shiroAgent,
		coder1, coder2, coder3,
		workerExec,
	)

	// Inject ReportStore and EventListener (if provided)
	if reportStore != nil {
		orch.SetReportStore(reportStore)
	}
	if eventListener != nil {
		orch.SetEventListener(eventListener)
	}

	return orch
}

// ========================================
// Assertion Helpers
// ========================================

// assertStageSequence asserts that the stage sequence matches the expected sequence
func assertStageSequence(t *testing.T, recorder *StageRecorder, expected []string) {
	t.Helper()

	actual := recorder.GetSequence()
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("Stage sequence mismatch:\n  expected: %v\n  actual:   %v", expected, actual)
	}
}

// assertExecutionReport asserts that the execution report matches the expected values
//
//	checks は map[string]interface{}{
//	    "Route": "OPS",
//	    "Status": "passed",
//	    "AttemptCount": 1,
//	} の形式
func assertExecutionReport(t *testing.T, report execution.ExecutionReport, checks map[string]interface{}) {
	t.Helper()

	// Reflection-based field checking
	reportValue := reflect.ValueOf(report)
	for fieldName, expectedValue := range checks {
		field := reportValue.FieldByName(fieldName)
		if !field.IsValid() {
			t.Errorf("Field %s does not exist in ExecutionReport", fieldName)
			continue
		}

		actualValue := field.Interface()
		if !reflect.DeepEqual(actualValue, expectedValue) {
			t.Errorf("Field %s mismatch:\n  expected: %v (%T)\n  actual:   %v (%T)",
				fieldName, expectedValue, expectedValue, actualValue, actualValue)
		}
	}
}

// assertReportNotEmpty asserts that at least one report exists in the store
func assertReportNotEmpty(t *testing.T, store *MockReportStore) {
	t.Helper()

	_, ok := store.GetLastReport()
	if !ok {
		t.Error("Expected at least one execution report, but store is empty")
	}
}

// assertReportEmpty asserts that no reports exist in the store (for CHAT route bypass test)
func assertReportEmpty(t *testing.T, store *MockReportStore) {
	t.Helper()

	_, ok := store.GetLastReport()
	if ok {
		t.Error("Expected no execution reports (CHAT route should bypass executor), but found report(s)")
	}
}
