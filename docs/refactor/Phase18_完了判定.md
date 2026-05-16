# Phase18 完了判定

## Phase の目的

Phase18 は `DistributedOrchestrator.executeAutonomousDistributed` に残っていた `autonomousapp.RunExecutor` request assembly / observe / execute / verify callback を、分散実行専用の `distributedAutonomousCoordinator` へ分離する。

目的は構造整理であり、route dispatch、transport executor、Code route retry / mailbox retry、event / evidence、TTS lifecycle、session lifecycle、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedAutonomousCoordinator`
  - 入力: context、task、route、session ID、TTS session ID
  - 出力: response string、error
  - 副作用: `autonomousapp.RunExecutor` 実行、route direct executor 実行、`entry.stage` event emission、autonomous execution log
  - 永続化: `ReportStore` が設定されている場合、`autonomousapp.RunExecutor` 経由で execution report を保存
  - ログ: `[AutonomousExecutor] entry.stage=...`、`execute start`、`execute complete`、`verify`
  - エラー契約: contract normalize error はそのまま返し、RunExecutor error 時も `result.Response` を返す

## 維持した既存挙動

- `contractapp.NormalizeRequestWithRoute` を使う。
- `autonomousapp.RunExecutor` を使う。
- `ReportStore` を渡す。
- `MaxRepair` は `maxRepairOrDefault()` 相当を使う。
- observe stage は `entry.stage` event を emit する。
- attempt > 0 のときだけ `buildExecutorRetryMessage` を使う。
- execute は `executeDistributedDirect` 相当へ委譲する。
- verify は `verifyByContract` を使う。
- RunExecutor error 時も `result.Response` を返す。
- fallback response を正常系として作らない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_autonomous.go`
- `internal/application/orchestrator/distributed_orchestrator_phase18_autonomous_test.go`
- `docs/refactor/Phase18_完了判定.md`

## 検証

Phase18 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase18|TestDistributedOrchestrator_.*(CODE|OPS|Retry|Evidence)|TestPhase8AutonomousExecutionCoordinatorUsesUpdatedReportStore'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed autonomous execution の詳細が `distributedAutonomousCoordinator` へ分離されている。
- `DistributedOrchestrator` 本体は autonomous execution の構築と委譲だけを持つ。
- ReportStore setter、retry message 条件、RunExecutor error 時の partial response return がテストで固定されている。
- Phase18 の検証コマンドが成功している。
- Phase18 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase19 として、`DistributedOrchestrator` の route dispatcher 境界整理に進む候補がある。event、evidence、TTS、session、autonomous coordinator が分離済みになったため、`executeDistributed` / `executeDistributedDirect` の route 分岐を小さな distributed route dispatcher へ移す準備ができている。
