# Phase15 distributed event / evidence 境界整理実装仕様

## 1. Phase15 の目的

Phase15 は、`DistributedOrchestrator` の event emission と evidence report 保存を collaborator 境界へ整理する段階である。

目的は次の通り。

- Viewer event emission を `distributedEventPort` へ分離する。
- execution report 保存を `distributedEvidenceReporter` へ分離する。
- Viewer event、transport log、execution report、response text を混同しない。
- route dispatch、TTS lifecycle、transport executor には踏み込まない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `emit`
  - `emitNote`
  - `emitProgress`
  - `saveExecutionReport`
  - `distributedAcceptance`
  - `distributedVerification`
  - `distributedEvidenceSteps`
  - `distributedEvidenceErrorKind`
- `internal/application/orchestrator/distributed_orchestrator_test.go`
- `internal/domain/execution/report.go`

## 3. 対象外

Phase15 では次を対象外にする。

- distributed route dispatcher。
- distributed TTS lifecycle。
- distributed autonomous coordinator。
- transport executor。
- node selection。
- coder config。
- MessageOrchestrator 側の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の event / evidence 構造

### event emission

`DistributedOrchestrator.emit` は listener が nil の場合、何もせず戻る。MessageOrchestrator の `messageEventPort` と異なり skipped log は出さない。

`emitNote` は `agent.note` event を作る thin helper である。

`emitProgress` は `domaintransport.Message` から route / channel / chat ID を復元し、progress event を emit する。

### evidence report

`saveExecutionReport` は reporter が nil、job ID が空、goal が空の場合は no-op になる。

report は次の helper で組み立てられる。

- `distributedAcceptance`
- `distributedVerification`
- `distributedEvidenceSteps`
- `distributedEvidenceErrorKind`

CHAT route success / failure と CODE route error などで `ProcessMessage` から呼ばれる。

## 5. 提案する collaborator

### `distributedEventPort`

private struct とする。初期段階では interface 化しない。

責務:

- distributed event の emit。
- note event の emit。
- transport message 由来の progress event emit。

dependency:

- `EventListener`

setter 反映:

- `SetEventListener` 後は最新 listener を使う。

MessageOrchestrator の `messageEventPort` と共通化しない理由:

- distributed 側は listener nil で skipped log を出さない既存挙動を持つ。
- 分散 transport event と Viewer event の境界をまず分散側で固定する必要がある。

### `distributedEvidenceReporter`

private struct とする。初期段階では interface 化しない。

責務:

- distributed execution report の組み立て。
- report store への保存。
- acceptance / verification / steps / error kind の生成。

dependency:

- `ReportStore`

setter 反映:

- `SetReportStore` 後は最新 store を使う。

## 6. collaborator 契約

### `distributedEventPort`

入力:

- event type
- from / to / content / route / jobID / sessionID / channel / chatID
- `domaintransport.Message`

出力:

- なし。

副作用:

- listener callback。

永続化:

- 直接永続化しない。

ログ:

- 新しいログは追加しない。

エラー契約:

- listener nil は no-op。
- error は返さない。

変更してはいけない挙動:

- distributed listener nil 時に skipped log を新設しない。
- event payload を変えない。
- note / progress event type を変えない。

### `distributedEvidenceReporter`

入力:

- context
- job ID
- goal
- route
- startedAt
- finishedAt
- error

出力:

- なし。

副作用:

- report store への保存。
- report store save error の log。

永続化:

- `ReportStore.Save` 経由。

ログ:

- save error 時に `[DistributedOrch] evidence save failed` を維持する。

エラー契約:

- reporter nil / jobID empty / goal empty は no-op。
- save error は caller に返さない。
- run error がある場合は report status を `failed` にする。
- run error がない場合は report status を `passed` にする。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedEventPort` を追加する。
3. `DistributedOrchestrator` に `events *distributedEventPort` field を追加する。
4. constructor で event port を初期化する。
5. `SetEventListener` で event port listener を更新する。
6. `emit` / `emitNote` / `emitProgress` を event port へ委譲する。
7. `distributedEvidenceReporter` を追加する。
8. `DistributedOrchestrator` に `evidence *distributedEvidenceReporter` field を追加する。
9. constructor で evidence reporter を初期化する。
10. `SetReportStore` で evidence reporter store を更新する。
11. `saveExecutionReport` を evidence reporter へ委譲する。
12. helper 関数の挙動を変えない。
13. focused test と全体 test を実行する。
14. `docs/refactor/Phase15_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestDistributed'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- listener nil no-op を MessageOrchestrator と同じ skipped log に変えてしまう。
- evidence save error を caller error にしてしまう。
- report status / route / steps / verification を変えてしまう。
- Viewer event と evidence report を同じものとして扱う。
- route dispatch に踏み込んで Phase15 の範囲を超える。

## 10. 完了条件

Phase15 の完了条件は次の通り。

- `docs/refactor/Phase15_distributed_event_evidence境界整理実装仕様.md` が作成されている。
- event / evidence の現在構造が棚卸しされている。
- `distributedEventPort` と `distributedEvidenceReporter` の契約が書かれている。
- 実装手順と検証手順が書かれている。
- コード変更は行っていない。
