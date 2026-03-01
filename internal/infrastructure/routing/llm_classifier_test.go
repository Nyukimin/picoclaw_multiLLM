package routing

import (
	"context"
	"fmt"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// mockLLMProvider はテスト用のLLMプロバイダー
type mockLLMProvider struct {
	response string
	err      error
}

func (m *mockLLMProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	if m.err != nil {
		return llm.GenerateResponse{}, m.err
	}
	return llm.GenerateResponse{
		Content:    m.response,
		TokensUsed: 100,
	}, nil
}

func (m *mockLLMProvider) Name() string {
	return "mock-llm"
}

func TestNewLLMClassifier(t *testing.T) {
	mock := &mockLLMProvider{response: "CHAT"}
	classifier := NewLLMClassifier(mock)

	if classifier == nil {
		t.Fatal("NewLLMClassifier should not return nil")
	}
}

func TestLLMClassifier_Classify_CHAT(t *testing.T) {
	mock := &mockLLMProvider{response: "CHAT"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "こんにちは、調子はどう？", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCHAT {
		t.Errorf("Expected route CHAT, got '%s'", decision.Route)
	}

	if decision.Confidence < 0.6 {
		t.Errorf("Expected confidence >= 0.6, got %f", decision.Confidence)
	}

	if decision.Reason == "" {
		t.Error("Reason should not be empty")
	}
}

func TestLLMClassifier_Classify_CODE(t *testing.T) {
	mock := &mockLLMProvider{response: "CODE"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "新しい機能を追加したい", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCODE {
		t.Errorf("Expected route CODE, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_PLAN(t *testing.T) {
	mock := &mockLLMProvider{response: "PLAN"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "この機能の実装アプローチを考えたい", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RoutePLAN {
		t.Errorf("Expected route PLAN, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_ANALYZE(t *testing.T) {
	mock := &mockLLMProvider{response: "ANALYZE"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "このエラーの原因を特定したい", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteANALYZE {
		t.Errorf("Expected route ANALYZE, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_OPS(t *testing.T) {
	mock := &mockLLMProvider{response: "OPS"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "ログを確認したい", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteOPS {
		t.Errorf("Expected route OPS, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_RESEARCH(t *testing.T) {
	mock := &mockLLMProvider{response: "RESEARCH"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "Goのベストプラクティスを知りたい", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteRESEARCH {
		t.Errorf("Expected route RESEARCH, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_InvalidRoute(t *testing.T) {
	// LLMが無効なルート名を返した場合、CHATにフォールバック
	mock := &mockLLMProvider{response: "INVALID_ROUTE"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "テストメッセージ", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCHAT {
		t.Errorf("Expected fallback to CHAT, got '%s'", decision.Route)
	}

	if decision.Confidence > 0.5 {
		t.Errorf("Expected low confidence for invalid route, got %f", decision.Confidence)
	}
}

func TestLLMClassifier_Classify_CODE1(t *testing.T) {
	mock := &mockLLMProvider{response: "CODE1"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "仕様を設計して", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCODE1 {
		t.Errorf("Expected route CODE1, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_CODE2(t *testing.T) {
	mock := &mockLLMProvider{response: "CODE2"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "コードを実装して", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCODE2 {
		t.Errorf("Expected route CODE2, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_CODE3(t *testing.T) {
	mock := &mockLLMProvider{response: "CODE3"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "高品質なコードレビューをして", "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCODE3 {
		t.Errorf("Expected route CODE3, got '%s'", decision.Route)
	}
}

func TestLLMClassifier_Classify_LLMError(t *testing.T) {
	// LLMがエラーを返した場合
	mock := &mockLLMProvider{err: fmt.Errorf("LLM error")}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "テストメッセージ", "line", "U123")

	_, err := classifier.Classify(context.Background(), testTask)
	if err == nil {
		t.Error("Expected error when LLM fails")
	}
}

func TestLLMClassifier_Classify_MultilineMessage(t *testing.T) {
	mock := &mockLLMProvider{response: "CODE"}
	classifier := NewLLMClassifier(mock)

	jobID := task.NewJobID()
	multilineMessage := `このファイルに以下の機能を追加して：
1. ユーザー認証
2. ログイン機能
3. セッション管理`
	testTask := task.NewTask(jobID, multilineMessage, "line", "U123")

	decision, err := classifier.Classify(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Classify failed: %v", err)
	}

	if decision.Route != routing.RouteCODE {
		t.Errorf("Expected route CODE, got '%s'", decision.Route)
	}
}
