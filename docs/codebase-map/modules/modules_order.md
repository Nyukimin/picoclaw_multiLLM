---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: modules/order
---

# pkg/modules/order — Order エージェントモジュール解析

## 概要

Order エージェント（Aka=Order1, Ao=Order2, Gin=Order3）用の機能モジュール群。
新アーキテクチャでのみ使用。各 Order は同じモジュール群を持ち、LLM プロバイダーで個性が付く。

## ファイル構成

| ファイル | モジュール | 担当 |
|---------|----------|------|
| `pkg/modules/order/proposal.go` | ProposalGenerationModule | plan/patch/risk の提案生成 |
| `pkg/modules/order/analysis.go` | CodeAnalysisModule | コード分析・品質評価 |
| `pkg/modules/order/approval.go` | ApprovalFlowModule | 提案の承認フロー（Order3 専用） |

## Order エージェント対応表

| Order | 愛称 | LLM | 特徴 |
|-------|------|-----|------|
| Order1 (Aka) | — | DeepSeek | 低コスト大量処理、CODE1 ルート |
| Order2 (Ao) | — | OpenAI GPT-4 | コーディング中核、CODE2 ルート |
| Order3 (Gin) | — | Anthropic Claude API | 高品質推論、CODE3 ルート、承認フロー必須 |

## ProposalGenerationModule

```go
type ProposalGenerationModule struct {
    core *modules.AgentCore
}

type Proposal struct {
    JobID    string
    Plan     string
    Patch    string
    Risk     string
    CostHint string
}

func (m *ProposalGenerationModule) GenerateProposal(ctx, task string) (*Proposal, error)
```

- `plan`（何をするか）・`patch`（差分）・`risk`（リスク評価）・`cost_hint`（コスト見積）を含む JSON を生成
- 旧アーキテクチャの `parseCoder3Output()` と同様の構造
- **注意**: `factory.go` の `resolveProvider()` が nil を返すため、現時点では nil panic

## CodeAnalysisModule

```go
type CodeAnalysisModule struct {
    core *modules.AgentCore
}

func (m *CodeAnalysisModule) AnalyzeCode(ctx, code string) (string, error)
```

- コードの品質・バグ・セキュリティ問題を分析
- 分析結果を構造化テキストで返す

## ApprovalFlowModule（Order3 専用）

```go
// pkg/modules/order/approval.go
type ApprovalFlowModule struct {
    mu               sync.RWMutex // MISSING: ロックはあるが...
    pendingProposals map[string]Proposal
    core             *modules.AgentCore
}
```

### 既知バグ1: pendingProposals のミューテックス不使用

```go
func (m *ApprovalFlowModule) AddPendingProposal(jobID string, proposal Proposal) {
    // mu.Lock() が呼ばれていない
    m.pendingProposals[jobID] = proposal
}
```

`mu` フィールドは定義されているが、`AddPendingProposal` / `GetPendingProposal` で
ロックが取得されていない（コンパイル時に警告なし）。並行アクセスで map の競合状態が発生する。

### 既知バグ2: 承認後の実行トリガーが未実装

```go
func (m *ApprovalFlowModule) ApprovePendingProposal(jobID string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    if _, ok := m.pendingProposals[jobID]; !ok {
        return false
    }
    // TODO: Trigger execution of approved proposal
    delete(m.pendingProposals, jobID)
    return true
}
```

承認したジョブは map から削除されるだけで、Worker へ実行委譲されない。
承認フローが機能的に完結していない。

## 新アーキテクチャにおける Order の呼び出しフロー

```
processMessageNewArch()
    │
    ├── RoutingModule.RouteTask() → CODE1/2/3 決定
    │
    └── delegateToOrderNewArch(route, task)
            │
            ├── factory.NewAgentWithModules(orderID) → AgentCore 生成
            │       [!] resolveProvider() は nil を返す → panic
            │
            ├── ProposalGenerationModule.GenerateProposal(task)
            │
            └── AggregationModule.AggregateResults(proposals)
```

## 設計上の制約

- Order は `plan` と `patch` を生成するのみ。直接の破壊的実行は禁止（CLAUDE.md 4.1節）
- 実行は Worker（ExecutionModule）が承認後に担当
- 現在の承認フロー実装では、承認後に実行が発火しない（TODO 状態）

## 関連

- [modules_worker.md](modules_worker.md)
- [approval.md](approval.md)
- [潜在バグ一覧.md](../潜在バグ一覧.md)
