# Phase18 distributed autonomous coordinator 境界整理実装仕様

## 1. Phase18 の目的

Phase18 は、`DistributedOrchestrator.executeAutonomousDistributed` に残っている `autonomousapp.RunExecutor` request assembly / observe / execute / verify callback を `distributedAutonomousCoordinator` へ分離する段階である。

目的は次の通り。

- autonomous execution の request assembly を route dispatch 本体から分ける。
- retry message、verify contract、execution steps、failure kind の既存契約を維持する。
- route direct execution は引き続き `DistributedOrchestrator.executeDistributedDirect` に委譲する。
- Phase15 event / evidence、Phase16 TTS lifecycle、Phase17 session lifecycle を維持する。
- MessageOrchestrator の `autonomousExecutionCoordinator` と安易に共通化せず、分散側のログと実行引数を固定する。

Phase18 は構造整理であり、autonomous executor の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `executeAutonomousDistributed`
  - `SetReportStore`
  - constructor
- 新規追加する `distributedAutonomousCoordinator`
- `internal/application/autonomous/executor.go`
- autonomous / distributed focused tests。

## 3. 対象外

Phase18 では次を対象外にする。

- distributed route dispatcher 分割。
- distributed transport executor 分割。
- Code route retry / mailbox retry の変更。
- `autonomousapp.RunExecutor` 本体の変更。
- `verifyByContract` の仕様変更。
- `buildExecutorRetryMessage` の仕様変更。
- event / evidence / TTS / session 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の autonomous execution 構造

`executeAutonomousDistributed` は次を 1 method 内で行っている。

- `contractapp.NormalizeRequestWithRoute(t.UserMessage(), route.String())`
- `autonomousapp.RunExecutor` request の組み立て。
- `JobID` / `Route` / `Capability` / `Contract` / `MaxRepair` の設定。
- observe stage のログと `entry.stage` event emit。
- `ReportStore: o.reporter` の受け渡し。
- attempt ごとの `executeDistributedDirect` 呼び出し。
- attempt > 0 の retry message 組み立て。
- `classifyExecutorFailure` / `routeExecutionSteps` / `errorString` の設定。
- `verifyByContract` による検証。
- RunExecutor error 時も `result.Response` を返す。

## 5. 提案する collaborator

### `distributedAutonomousCoordinator`

`distributedAutonomousCoordinator` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_autonomous.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `ReportStore`
- `maxRepair` resolver
- distributed event emitter
- distributed direct executor

setter 反映:

- `SetReportStore` 後は coordinator が最新 report store を使う。

MessageOrchestrator 側と共通化しない理由:

- distributed 側は `executeDistributedDirect(ctx, t, route, sessionID, ttsSessionID)` に接続する。
- distributed 側は `[AutonomousExecutor] ...` の詳細ログを持つ。
- route dispatcher / transport executor 分割前に共通化すると依存境界が曖昧になる。

## 6. `distributedAutonomousCoordinator` の契約

入力:

- `context.Context`
- `task.Task`
- `routing.Route`
- session ID
- TTS session ID

出力:

- response string
- error

副作用:

- `autonomousapp.RunExecutor` 実行。
- route direct executor 実行。
- `entry.stage` event emission。
- autonomous execution log。
- report store への保存は `autonomousapp.RunExecutor` 経由。

永続化:

- `ReportStore` が設定されている場合、`autonomousapp.RunExecutor` 経由で execution report が保存される。

ログ:

- `[AutonomousExecutor] entry.stage=...`
- `[AutonomousExecutor] execute start ...`
- `[AutonomousExecutor] execute complete ...`
- `[AutonomousExecutor] verify ...`

エラー契約:

- contract normalize error はそのまま返す。
- route direct execution error は `RunExecutor` 経由で apply / verify error として返る。
- `RunExecutor` が error を返した場合でも `result.Response` を返す。
- fallback response を正常系として作らない。

変更してはいけない既存挙動:

- `contractapp.NormalizeRequestWithRoute` を使う。
- `autonomousapp.RunExecutor` を使う。
- `ReportStore` を渡す。
- `MaxRepair` は `maxRepairOrDefault()` 相当を使う。
- observe stage は `entry.stage` event を emit する。
- attempt > 0 のときだけ `buildExecutorRetryMessage` を使う。
- execute は `executeDistributedDirect` 相当へ委譲する。
- verify は `verifyByContract` を使う。
- RunExecutor error 時も `result.Response` を返す。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedAutonomousCoordinator` を `distributed_orchestrator_autonomous.go` に追加する。
3. `DistributedOrchestrator` に `autonomousExecutions *distributedAutonomousCoordinator` field を追加する。
4. `NewDistributedOrchestrator` で coordinator を組み立てる。
5. `SetReportStore` で coordinator の report store を更新する。
6. `executeAutonomousDistributed` を coordinator への委譲に置き換える。
7. 既存ログ、retry message、verify、result.Response return を変えない。
8. gofmt を実行する。
9. focused test と全体 test を実行する。
10. `docs/refactor/Phase18_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

autonomous focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase18|TestDistributedOrchestrator_.*(CODE|OPS|Retry|Evidence)|TestPhase8AutonomousExecutionCoordinatorUsesUpdatedReportStore'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- retry attempt 0 から retry message を入れてしまう。
- RunExecutor error 時に `result.Response` を捨ててしまう。
- ReportStore の setter 反映を落とす。
- verify を `verifyByContract` 以外に変えてしまう。
- route dispatch / transport executor まで同時に分離して Phase18 の範囲を超える。
- fallback response を正常系として作る。

## 10. 完了条件

Phase18 の完了条件は次の通り。

- `docs/refactor/Phase18_distributed_autonomous_coordinator境界整理実装仕様.md` が作成されている。
- 現在の distributed autonomous execution 構造が棚卸しされている。
- `distributedAutonomousCoordinator` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- route direct execution と transport executor には踏み込まない方針が明記されている。
- コード変更は行っていない。
