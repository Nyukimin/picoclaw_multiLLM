# Phase21 distributed code execution 境界整理実装仕様

## 1. Phase21 の目的

Phase21 は、`DistributedOrchestrator.executeCodeViaShiro` に集中している Code route execution / coder selection / coder retry を `distributedCodeExecutionCoordinator` へ分離する段階である。

目的は次の通り。

- coder selection、Coder 依頼、Worker 実行、retry instruction 生成を分散 Code 専用境界へ移す。
- coder config、proposal payload、worker retryable failure、event / note、CentralMemory record の既存契約を維持する。
- Phase19 route dispatcher からは Code execution callback として呼ぶ。
- node selection helper と transport executor 本体には踏み込まない。

Phase21 は構造整理であり、Code route execution の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `executeCodeViaShiro`
  - `SetCoderConfigs`
  - `SetNodeCapabilities`
  - `SetDistributedTimeouts`
  - constructor
- 新規追加する `distributedCodeExecutionCoordinator`
- Code route focused tests。

## 3. 対象外

Phase21 では次を対象外にする。

- route dispatcher の追加分割。
- transport executor 本体。
- node selector 実装。
- `inferTaskRequirement` の仕様変更。
- proposal / worker execution contract の変更。
- event / evidence / TTS / session / autonomous / transport 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の Code route execution 構造

`executeCodeViaShiro` は次を 1 method 内で行っている。

- `routeToCoderForMessage` による coder selection。
- coder 未決定時の `no coder mapped for route %s` error。
- `mio -> shiro` の code handoff event / note。
- attempt loop。
- Coder への task message 作成。
- coder message context への route / retry_attempt / channel / chat_id / coder_config 設定。
- CentralMemory record。
- Coder mailbox execution。
- retryable failure 分類と retry instruction 作成。
- Coder result event / note。
- Proposal nil の場合の Shiro 整形 route。
- Proposal ありの場合の Shiro Worker 実行 route。
- Worker retry request による retry instruction 作成。
- retry budget exhausted error。

## 5. 提案する collaborator

### `distributedCodeExecutionCoordinator`

`distributedCodeExecutionCoordinator` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_code.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `session.CentralMemory`
- event emitter
- note emitter
- coder selector
- coder config provider
- retry max resolver
- mailbox executor
- transport executor

setter 反映:

- `SetCoderConfigs` 後は coordinator が最新 coder config を使う。
- `SetDistributedTimeouts` 後は retry max resolver 経由で最新 retry max を使う。
- `SetNodeCapabilities` は coder selector callback 経由で現行ロジックを使う。

## 6. `distributedCodeExecutionCoordinator` の契約

入力:

- `context.Context`
- `task.Task`
- `routing.Route`
- session ID
- job ID

出力:

- response string
- error

副作用:

- Coder / Shiro transport execution。
- CentralMemory record。
- event / note emission。
- retry instruction generation。

永続化:

- DB 永続化はしない。
- CentralMemory に Coder / Shiro message を記録する。

ログ:

- `[DistributedOrch] code handoff route=... target=... job=...`

エラー契約:

- coder 未決定時は `no coder mapped for route %s` を返す。
- retryable failure は retry budget 内で次 attempt に進む。
- retry budget を使い切った場合は `coder retry budget exhausted for job %s` を返す。
- fallback response を正常系として作らない。

変更してはいけない既存挙動:

- attempt 0 は通常依頼、attempt > 0 は `worker.retry_request` と retry note。
- coder message context の key。
- coder config 付与条件。
- coder mailbox receiveOnAgent は `mio`。
- Proposal nil path と Proposal path。
- Worker retryable failure の retry instruction。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedCodeExecutionCoordinator` を `distributed_orchestrator_code.go` に追加する。
3. `DistributedOrchestrator` に `codeExecution *distributedCodeExecutionCoordinator` field を追加する。
4. `NewDistributedOrchestrator` で coordinator を組み立てる。
5. `SetCoderConfigs` で coordinator の config provider を更新できる構造にする。
6. `executeCodeViaShiro` を coordinator への委譲に置き換える。
7. 既存 event / note / context / error message を変えない。
8. gofmt を実行する。
9. focused test と全体 test を実行する。
10. `docs/refactor/Phase21_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

code focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase21|TestDistributedOrchestrator_.*(CODE|Retry)|TestDistributedExecutionErrorClassification'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- coder message context から `retry_attempt` や `coder_config` を落とす。
- coder mailbox receiveOnAgent を変える。
- Proposal nil path と Proposal path を混同する。
- retryable failure を fallback success にする。
- retry budget を固定値にして `SetDistributedTimeouts` の retry max 反映を落とす。
- node selection helper まで同時に動かして Phase21 の範囲を超える。

## 10. 完了条件

Phase21 の完了条件は次の通り。

- `docs/refactor/Phase21_distributed_code_execution境界整理実装仕様.md` が作成されている。
- 現在の Code route execution 構造が棚卸しされている。
- `distributedCodeExecutionCoordinator` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- transport executor / node selector 本体には踏み込まない方針が明記されている。
- コード変更は行っていない。
