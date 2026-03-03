package agent

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// Mock LLMProvider
type mockLLMProvider struct {
	generateFunc func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error)
}

func (m *mockLLMProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return llm.GenerateResponse{Content: "Mock response"}, nil
}

func (m *mockLLMProvider) Name() string {
	return "mock"
}

// Mock Classifier
type mockClassifier struct {
	classifyFunc func(ctx context.Context, t task.Task) (routing.Decision, error)
}

func (m *mockClassifier) Classify(ctx context.Context, t task.Task) (routing.Decision, error) {
	if m.classifyFunc != nil {
		return m.classifyFunc(ctx, t)
	}
	return routing.NewDecision(routing.RouteCHAT, 0.8, "Mock classification"), nil
}

// Mock RuleDictionary
type mockRuleDictionary struct {
	matchFunc func(t task.Task) (routing.Route, float64, bool)
}

func (m *mockRuleDictionary) Match(t task.Task) (routing.Route, float64, bool) {
	if m.matchFunc != nil {
		return m.matchFunc(t)
	}
	return "", 0.0, false
}

func TestMioAgentDecideAction_ExplicitCommand(t *testing.T) {
	mio := NewMioAgent(
		&mockLLMProvider{},
		&mockClassifier{},
		&mockRuleDictionary{},
	)

	tests := []struct {
		message      string
		expectedRoute routing.Route
	}{
		{"/chat hello", routing.RouteCHAT},
		{"/code3 implement feature", routing.RouteCODE3},
		{"/plan create project", routing.RoutePLAN},
		{"/analyze logs", routing.RouteANALYZE},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			decision, err := mio.DecideAction(context.Background(), testTask)
			if err != nil {
				t.Fatalf("DecideAction failed: %v", err)
			}

			if decision.Route != tt.expectedRoute {
				t.Errorf("Expected route %s, got %s", tt.expectedRoute, decision.Route)
			}

			if decision.Confidence != 1.0 {
				t.Errorf("Expected confidence 1.0 for explicit command, got %f", decision.Confidence)
			}
		})
	}
}

func TestMioAgentDecideAction_RuleDictionary(t *testing.T) {
	ruleDictionary := &mockRuleDictionary{
		matchFunc: func(t task.Task) (routing.Route, float64, bool) {
			if t.UserMessage() == "ファイルを作成" {
				return routing.RouteCODE, 0.95, true
			}
			return "", 0.0, false
		},
	}

	mio := NewMioAgent(
		&mockLLMProvider{},
		&mockClassifier{},
		ruleDictionary,
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "ファイルを作成", "line", "U123")

	decision, err := mio.DecideAction(context.Background(), testTask)
	if err != nil {
		t.Fatalf("DecideAction failed: %v", err)
	}

	if decision.Route != routing.RouteCODE {
		t.Errorf("Expected route CODE, got %s", decision.Route)
	}

	if decision.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", decision.Confidence)
	}
}

func TestMioAgentDecideAction_DefaultChatWhenNoRuleMatch(t *testing.T) {
	// ルール辞書にマッチしない場合、LLM分類器をスキップしてCHATにフォールバック
	classifierCalled := false
	classifier := &mockClassifier{
		classifyFunc: func(ctx context.Context, tk task.Task) (routing.Decision, error) {
			classifierCalled = true
			return routing.Decision{}, nil
		},
	}

	mio := NewMioAgent(
		&mockLLMProvider{},
		classifier,
		&mockRuleDictionary{},
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "こんにちは", "line", "U123")

	decision, err := mio.DecideAction(context.Background(), testTask)
	if err != nil {
		t.Fatalf("DecideAction failed: %v", err)
	}

	if decision.Route != routing.RouteCHAT {
		t.Errorf("Expected route CHAT, got %s", decision.Route)
	}

	if decision.Confidence != 0.7 {
		t.Errorf("Expected confidence 0.7, got %f", decision.Confidence)
	}

	if classifierCalled {
		t.Error("Classifier should not be called when defaulting to CHAT")
	}
}

func TestMioAgentChat(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{
				Content:      "こんにちは！何かお手伝いできますか？",
				TokensUsed:   20,
				FinishReason: "stop",
			}, nil
		},
	}

	mio := NewMioAgent(
		llmProvider,
		&mockClassifier{},
		&mockRuleDictionary{},
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "こんにちは", "line", "U123")

	response, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if response != "こんにちは！何かお手伝いできますか？" {
		t.Errorf("Unexpected chat response: %s", response)
	}
}
