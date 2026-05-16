# Chat / Worker / Coder 仕様

## 目的

RenCrow は Chat / Worker / Coder の三分割を最重要境界として扱う。

この境界は UI 上の呼び名ではなく、実行責務、権限、安全境界、ログ契約を分けるためのシステム境界である。

## 役割

| 役割 | 主な実体 | 責務 | 担当しないこと |
| --- | --- | --- | --- |
| Chat | Mio | ユーザー対話、ルーティング判断、結果返却、全体モニタリング | 破壊的操作、patch 適用、file / shell / git 実行 |
| Worker | Shiro / Worker Core | 実行可否判断、Coder 選定、patch / command 実行、ログ記録、安全境界管理 | ユーザー向け自然対話の最終表現、Coder の提案生成 |
| Coder | Aka / Ao / Gin | plan / patch / proposal 生成、設計説明、複数案提示 | 採用決定、正本ブランチ管理、破壊的操作の直接実行 |

## 不変ルール

Coder は破壊的操作を直接実行しない。

Coder が返すものは候補である。

- plan
- patch
- proposal
- risk
- cost hint

実行、採用、ログ記録、安全確認は Worker が担当する。

## route chain

代表的な流れ:

```text
adapter input
  -> MessageOrchestrator
  -> Mio route decision
  -> CHAT / PLAN / ANALYZE / OPS / RESEARCH / CODE / CODE1 / CODE2 / CODE3
  -> route-specific handler
  -> Worker / Coder / Chat response
  -> event / report / Viewer / channel response
```

主な route:

| route | 意味 | 主な処理 |
| --- | --- | --- |
| `CHAT` | 通常対話 | Mio が応答する |
| `PLAN` | 計画 | 計画作成を優先する |
| `ANALYZE` | 分析 | 調査・解析を行う |
| `OPS` | 運用 | Worker が安全境界内で実行する |
| `RESEARCH` | 調査 | 外部検索や情報収集を行う |
| `CODE` | Coder 自動選択 | Worker が Coder を選ぶ |
| `CODE1` | Aka / DeepSeek | 仕様設計寄り |
| `CODE2` | Ao / OpenAI | 実装寄り |
| `CODE3` | Gin / Claude | 高品質推論・複雑作業 |

明示コマンド、ルール辞書、分類器、安全側 fallback の順で route を決める。

fallback は正常系ではなく、分類できなかった場合の安全側経路である。

## plan / patch / execution の境界

```text
Coder
  -> plan / patch / proposal を生成

Worker
  -> proposal を検証
  -> protected file / workspace / policy を確認
  -> file / shell / git / test を実行
  -> report / event / log を残す

Chat
  -> ユーザーへ結果を返す
```

## 実装箇所

| 仕様 | 主担当 |
| --- | --- |
| Mio / Shiro / Coder の domain entity | `internal/domain/agent` |
| route 値 | `internal/domain/routing` |
| route 分類 | `internal/infrastructure/routing` |
| orchestration | `internal/application/orchestrator/message_orchestrator_*.go` |
| Coder proposal / CodeExecutor | `internal/application/orchestrator/code_executor*.go`, `internal/domain/proposal`, `internal/domain/patch` |
| Worker execution | `internal/application/service/worker_execution_*.go` |
| execution persistence | `internal/infrastructure/persistence/execution` |
| standalone agent process | `cmd/picoclaw-agent/*.go` |

## 検証

確認対象:

- explicit route command が期待 route に入る。
- Coder route で Coder が直接実行しない。
- WorkerExecutionService が protected file / workspace / policy を守る。
- job_id、session_id、route、status が記録される。
- Viewer / event log に結果が追跡できる。

主なテスト:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/agent ./internal/domain/routing ./internal/application/orchestrator ./internal/application/service
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
```
