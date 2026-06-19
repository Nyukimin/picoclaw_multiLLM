---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: modules/worker
---

# pkg/modules/worker — Worker エージェントモジュール解析

## 概要

Worker エージェント（愛称: Shiro）用の機能モジュール群。
新アーキテクチャ（`UseNewArchitecture=true`）でのみ使用される。
ルーティング決定・タスク実行・結果集約・Heartbeat 収集の4モジュールで構成。

## ファイル構成

| ファイル | モジュール名 | 責務 |
|---------|------------|------|
| `pkg/modules/worker/routing.go` | RoutingModule | ルーティング決定（Router ラッパー） |
| `pkg/modules/worker/execution.go` | ExecutionModule | ルート別タスク実行 |
| `pkg/modules/worker/aggregation.go` | AggregationModule | 複数 Order の結果集約 |
| `pkg/modules/worker/heartbeat.go` | HeartbeatCollectorModule | エージェント状態収集 |

## RoutingModule

```go
type RoutingModule struct {
    router Router // pkg/agent.Router インターフェース（循環依存回避のため抽象化）
    core   *modules.AgentCore
}

type RoutingDecisionWithJobID struct {
    agent.RoutingDecision
    JobID string
}

func (m *RoutingModule) RouteTask(ctx, task string) (*RoutingDecisionWithJobID, error)
```

- `pkg/agent/router.go` の `Router` インターフェースをラップ
- RoutingDecision に JobID を付加して返す
- JobID は `pkg/jobid.Generator` で生成

## ExecutionModule

```go
type ExecutionModule struct {
    core *modules.AgentCore
}

func (m *ExecutionModule) ExecuteTask(ctx, task string, route agent.Route) (string, error)
```

ルート別実行ロジック:

| ルート | 実行内容 |
|-------|---------|
| CHAT | executeChat() — LLM で会話応答 |
| OPS | executeOps() — システム操作コマンド |
| RESEARCH | executeResearch() — 情報収集 |
| PLAN | executePlan() — 計画立案 |
| ANALYZE | executeAnalyze() — データ分析 |
| CODE1/2/3 | Order エージェントへ委譲（loop_new.go 側） |

### 既知バグ: Usage nil dereference

```go
// pkg/modules/worker/execution.go
func (m *ExecutionModule) executeChat(ctx, task string) (string, error) {
    response, err := m.core.Provider.Chat(ctx, messages, nil, m.core.Model, nil)
    // ...
    tokens := response.Usage.TotalTokens // Usage が nil の場合 panic
}
```

`response.Usage` の nil チェックが欠如。LLM プロバイダーが Usage を省略した場合（Ollama 等）に panic。

## AggregationModule

```go
type AggregationModule struct {
    core *modules.AgentCore
}

func (m *AggregationModule) AggregateResults(ctx, results []string) (string, error)
```

- 複数 Order（Aka/Ao/Gin）の提案を受け取り、Worker LLM で統合・要約
- 合議制（Deliberation Mode）を実装するための基盤

## HeartbeatCollectorModule

```go
type HeartbeatCollectorModule struct {
    mu      sync.RWMutex
    agents  map[string]AgentStatus // agent_id -> status
    timeout time.Duration          // デフォルト 60 秒
}

type AgentStatus struct {
    AgentID   string
    Alias     string
    Status    string
    UpdatedAt time.Time
}

func (m *HeartbeatCollectorModule) ReportHeartbeat(agentID, alias, status string)
func (m *HeartbeatCollectorModule) GetAllStatus() []AgentStatus
func (m *HeartbeatCollectorModule) CleanupStale() // 10分以上更新なしを削除
```

### 既知問題: 報告者不在

`ReportHeartbeat()` を呼ぶコードが存在しない。
各 Order エージェントは自分から `HeartbeatCollectorModule` に報告する仕組みを持たない。
コレクターは実装されているが、フィード（データ投入）が未接続。

## routeToOrderID マッピング（loop_new.go）

```go
func routeToOrderID(route agent.Route) string {
    switch route {
    case agent.RouteCode1:
        return "order1"
    case agent.RouteCode2:
        return "order2"
    case agent.RouteCode3:
        return "order3"
    case agent.RouteCode:
        return "order1" // CODE（汎用）は order1 へ（order2 が妥当かもしれない）
    default:
        return "order1"
    }
}
```

`RouteCode`（汎用 CODE）が `order1`（DeepSeek・低コスト）へ振られる設計判断。
コメントなし。`order2`（OpenAI）が適切かは要議論。

## 関連

- [modules_chat.md](modules_chat.md)
- [modules_order.md](modules_order.md)
- [潜在バグ一覧.md](../潜在バグ一覧.md)
