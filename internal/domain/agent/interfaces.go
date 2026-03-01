package agent

import (
	"context"

	"github.com/sipeed/picoclaw/internal/domain/routing"
	"github.com/sipeed/picoclaw/internal/domain/task"
)

// Classifier はタスク分類器のインターフェース
type Classifier interface {
	Classify(ctx context.Context, t task.Task) (routing.Decision, error)
}

// RuleDictionary はルール辞書のインターフェース
type RuleDictionary interface {
	Match(t task.Task) (routing.Route, float64, bool) // ルート, 確信度, マッチしたか
}

// ToolRunner はツール実行のインターフェース
type ToolRunner interface {
	Execute(ctx context.Context, toolName string, args map[string]interface{}) (string, error)
	List(ctx context.Context) ([]string, error)
}

// MCPClient はMCPクライアントのインターフェース
type MCPClient interface {
	CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (string, error)
	ListTools(ctx context.Context, serverName string) ([]string, error)
}
