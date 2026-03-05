package routing

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

func TestNewRuleDictionary(t *testing.T) {
	dict := NewRuleDictionary()

	if dict == nil {
		t.Fatal("NewRuleDictionary should not return nil")
	}
}

func TestRuleDictionary_Match_NoMatch(t *testing.T) {
	dict := NewRuleDictionary()

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "普通の会話メッセージ", "line", "U123")

	route, confidence, matched := dict.Match(testTask)

	if matched {
		t.Error("Should not match for normal conversation")
	}

	if route != "" {
		t.Errorf("Route should be empty when not matched, got '%s'", route)
	}

	if confidence != 0.0 {
		t.Errorf("Confidence should be 0.0 when not matched, got %f", confidence)
	}
}

func TestRuleDictionary_Match_CodeKeywords(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		expectRoute routing.Route
	}{
		{
			name:       "実装してのキーワード",
			message:    "このファイルを実装して",
			expectRoute: routing.RouteCODE,
		},
		{
			name:       "修正してのキーワード",
			message:    "このバグを修正して",
			expectRoute: routing.RouteCODE,
		},
		{
			name:       "リファクタリングのキーワード",
			message:    "このコードをリファクタリングして",
			expectRoute: routing.RouteCODE,
		},
		{
			name:       "テストを追加",
			message:    "テストを追加してください",
			expectRoute: routing.RouteCODE,
		},
	}

	dict := NewRuleDictionary()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			route, confidence, matched := dict.Match(testTask)

			if !matched {
				t.Error("Should match code-related keywords")
			}

			if route != tt.expectRoute {
				t.Errorf("Expected route '%s', got '%s'", tt.expectRoute, route)
			}

			if confidence <= 0.7 {
				t.Errorf("Expected high confidence (>0.7), got %f", confidence)
			}
		})
	}
}

func TestRuleDictionary_Match_PlanKeywords(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "計画を立てて",
			message: "この機能の実装計画を立てて",
		},
		{
			name:    "プランニング",
			message: "プランニングしてください",
		},
		{
			name:    "設計して",
			message: "この機能を設計して",
		},
	}

	dict := NewRuleDictionary()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			route, confidence, matched := dict.Match(testTask)

			if !matched {
				t.Error("Should match plan-related keywords")
			}

			if route != routing.RoutePLAN {
				t.Errorf("Expected route PLAN, got '%s'", route)
			}

			if confidence <= 0.7 {
				t.Errorf("Expected high confidence (>0.7), got %f", confidence)
			}
		})
	}
}

func TestRuleDictionary_Match_AnalyzeKeywords(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "分析して",
			message: "このコードを分析して",
		},
		{
			name:    "調査して",
			message: "このエラーを調査して",
		},
		{
			name:    "解析して",
			message: "このログを解析して",
		},
	}

	dict := NewRuleDictionary()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			route, confidence, matched := dict.Match(testTask)

			if !matched {
				t.Error("Should match analyze-related keywords")
			}

			if route != routing.RouteANALYZE {
				t.Errorf("Expected route ANALYZE, got '%s'", route)
			}

			if confidence <= 0.7 {
				t.Errorf("Expected high confidence (>0.7), got %f", confidence)
			}
		})
	}
}

func TestRuleDictionary_Match_OpsKeywords(t *testing.T) {
	tests := []struct {
		name    string
		message string
	}{
		{
			name:    "実行して",
			message: "このスクリプトを実行して",
		},
		{
			name:    "起動して",
			message: "サーバーを起動して",
		},
		{
			name:    "デプロイして",
			message: "本番環境にデプロイして",
		},
	}

	dict := NewRuleDictionary()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			route, confidence, matched := dict.Match(testTask)

			if !matched {
				t.Error("Should match ops-related keywords")
			}

			if route != routing.RouteOPS {
				t.Errorf("Expected route OPS, got '%s'", route)
			}

			if confidence <= 0.7 {
				t.Errorf("Expected high confidence (>0.7), got %f", confidence)
			}
		})
	}
}

func TestRuleDictionary_Match_ResearchKeywords(t *testing.T) {
	dict := NewRuleDictionary()

	// 「調べて」「検索して」はChatのWeb検索で即答するためルール辞書から除外
	// → マッチしない（ルータがCHATにフォールバックする）
	t.Run("調べて_はルール辞書でマッチしない", func(t *testing.T) {
		jobID := task.NewJobID()
		testTask := task.NewTask(jobID, "この技術について調べて", "line", "U123")
		_, _, matched := dict.Match(testTask)
		if matched {
			t.Error("「調べて」はChatのWeb検索で処理するためルール辞書でマッチすべきでない")
		}
	})

	t.Run("検索して_はルール辞書でマッチしない", func(t *testing.T) {
		jobID := task.NewJobID()
		testTask := task.NewTask(jobID, "最新の情報を検索して", "line", "U123")
		_, _, matched := dict.Match(testTask)
		if matched {
			t.Error("「検索して」はChatのWeb検索で処理するためルール辞書でマッチすべきでない")
		}
	})

	// 「リサーチして」は深い調査タスクとしてRESEARCHにルーティング
	t.Run("リサーチして_はRESEARCHにマッチする", func(t *testing.T) {
		jobID := task.NewJobID()
		testTask := task.NewTask(jobID, "競合をリサーチして", "line", "U123")
		route, confidence, matched := dict.Match(testTask)
		if !matched {
			t.Error("「リサーチして」はRESEARCHキーワードにマッチすべき")
		}
		if route != routing.RouteRESEARCH {
			t.Errorf("Expected route RESEARCH, got '%s'", route)
		}
		if confidence <= 0.7 {
			t.Errorf("Expected high confidence (>0.7), got %f", confidence)
		}
	})
}

func TestRuleDictionary_Match_MultipleKeywords(t *testing.T) {
	dict := NewRuleDictionary()

	jobID := task.NewJobID()
	// 複数のキーワードが含まれる場合、最初にマッチしたものを返す
	testTask := task.NewTask(jobID, "このコードを分析して実装して", "line", "U123")

	route, confidence, matched := dict.Match(testTask)

	if !matched {
		t.Error("Should match when multiple keywords present")
	}

	// どちらかにマッチすればOK（最初にマッチしたものが返される）
	if route != routing.RouteANALYZE && route != routing.RouteCODE {
		t.Errorf("Expected route ANALYZE or CODE, got '%s'", route)
	}

	if confidence <= 0.7 {
		t.Errorf("Expected high confidence (>0.7), got %f", confidence)
	}
}
