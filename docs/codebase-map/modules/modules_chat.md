---
run_id: run_20260619_000000
generated_at: 2026-06-19
phase: phase2
module_group: modules/chat
---

# pkg/modules/chat — Chat エージェントモジュール解析

## 概要

Chat エージェント（愛称: Mio）用の機能モジュール群。
新アーキテクチャ（`UseNewArchitecture=true`）でのみ使用される。
`AgentCore` + `Module` インターフェースを通じて `processMessageNewArch()` から呼び出される。

## ファイル構成

| ファイル | 内容 |
|---------|------|
| `pkg/modules/chat/reception.go` | LightweightReceptionModule — タスク受領（軽量、LLM呼出なし） |
| `pkg/modules/chat/decision.go` | FinalDecisionModule — 最終応答決定 |

## LightweightReceptionModule

```go
// pkg/modules/chat/reception.go
type LightweightReceptionModule struct {
    core *modules.AgentCore
}

func (m *LightweightReceptionModule) Name() string { return "LightweightReception" }
func (m *LightweightReceptionModule) Initialize(ctx, core) error { ... }
func (m *LightweightReceptionModule) Shutdown(ctx) error { ... }

// ReceiveTask: ユーザーメッセージをパッケージングして TaskPackage を返す
func (m *LightweightReceptionModule) ReceiveTask(msg string) (*TaskPackage, error)
```

**重要**: `ReceiveTask()` は純粋なパッケージング処理のみ。LLM 呼出なし。
TaskPackage には JobID（`pkg/jobid` で生成）、メッセージ内容、タイムスタンプが含まれる。

## FinalDecisionModule

```go
// pkg/modules/chat/decision.go
type FinalDecisionModule struct {
    core *modules.AgentCore
}

func (m *FinalDecisionModule) Name() string { return "FinalDecision" }

// MakeFinalDecision: Worker/Order の集約結果から最終応答を生成
func (m *FinalDecisionModule) MakeFinalDecision(ctx, aggregatedResult string) (string, error)
```

- Chat エージェント（Mio）の LLM プロバイダーを使って最終応答を生成
- **注意**: `factory.go` の `resolveProvider()` が nil を返すため、現時点では nil panic が発生する

## 新アーキテクチャにおける Chat の役割

```
ユーザーメッセージ
      │
      ▼
LightweightReceptionModule.ReceiveTask()
      │  TaskPackage（JobID付き）
      ▼
Worker へ委譲（processMessageNewArch → WorkerRoutingModule）
      │  ...Order 処理...
      ▼
FinalDecisionModule.MakeFinalDecision(集約結果)
      │  最終応答文字列
      ▼
ユーザーへ返信
```

旧アーキテクチャとの違い:
- 旧: AgentLoop が CHAT ルートとして LLM を直接呼び出す
- 新: Chat は「受領」と「最終決定」のみ担当、ルーティングは Worker が担当

## 既知の問題

| 問題 | 場所 | 深刻度 |
|------|------|--------|
| provider が nil で panic | factory.go resolveProvider() | CRITICAL |
| 新アーキ有効時のみ発現 | UseNewArchitecture=true | — |

## 関連

- [modules_worker.md](modules_worker.md)
- [modules_order.md](modules_order.md)
- [core.md](core.md)
