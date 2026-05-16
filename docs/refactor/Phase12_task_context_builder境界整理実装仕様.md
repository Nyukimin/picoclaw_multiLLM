# Phase12 task context builder 境界整理実装仕様

## 1. Phase12 の目的

Phase12 は、`MessageOrchestrator` に残っている `buildTaskForRequest` を `messageTaskContextBuilder` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- request から `task.Task`、`task.JobID`、TTS session ID を組み立てる責務を明確にする。
- attachment event emission を event port 経由にする。
- attachment event は Viewer event であり、response text、execution log、audio chunk と混同しない。
- TTS session ID 生成は provider 挙動ではなく request context assembly として扱う。
- Phase8-11 の collaborator 契約を維持する。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/message_orchestrator_task.go`
  - `buildTaskForRequest`
- `internal/application/orchestrator/message_orchestrator.go`
  - constructor
  - field
  - `ProcessMessage`
  - `SetTTSBridge`
- attachment event
- TTS session ID generation
- ProcessMessage / event / TTS tests

## 3. 対象外

Phase12 では次を対象外にする。

- event payload / DTO / SSE event の変更。
- TTS lifecycle の追加分割。
- route dispatcher の追加分割。
- provider 挙動変更。
- Viewer JS / CSS。
- IdleChat。
- STT。
- LLM provider。
- runtime config。
- distributed orchestrator。
- 未追跡の `tests/`。

## 4. 現在の task context builder 構造

`buildTaskForRequest` は次を行っている。

- `task.NewJobID` で job ID を作る。
- `task.NewTask` で user message / channel / chat ID を task にする。
- request attachments を task に引き継ぐ。
- attachment がある場合、`viewer.attachment.received` event を emit する。
- TTS bridge がある場合だけ `sessionID-jobID` 形式の TTS session ID を作る。

現在の結合点は次の通り。

- attachment event emission が `MessageOrchestrator.emit` に依存している。
- TTS session ID 生成が `MessageOrchestrator.ttsBridge` の nil 判定に依存している。
- job ID は task、attachment event、TTS session ID、response assembly で共有される。

## 5. 提案する collaborator

### `messageTaskContextBuilder`

`messageTaskContextBuilder` は private struct とする。初期段階では interface 化しない。

配置:

- `message_orchestrator_task.go` に定義する。
- `MessageOrchestrator` に field として持たせる。
- `NewMessageOrchestrator` で組み立てる。

dependency:

- event emit function
- TTS enabled 判定 function

setter 反映:

- `SetTTSBridge` 後、TTS enabled 判定が最新 bridge 状態を反映する。
- event emit function は `messageEventPort.Emit` を経由する。

method:

- `Build(req ProcessMessageRequest) (task.Task, task.JobID, string)`

## 6. `messageTaskContextBuilder` の契約

入力:

- `ProcessMessageRequest`

出力:

- `task.Task`
- `task.JobID`
- TTS session ID

副作用:

- attachment がある場合に `viewer.attachment.received` event を emit する。

永続化:

- 直接永続化しない。

ログ:

- 新しいログは追加しない。

エラー契約:

- error は返さない。
- attachment event listener が nil の場合は event port 側で skipped log になり、task build 自体は継続する。

変更してはいけない既存挙動:

- task の user message / channel / chat ID / attachments を変えない。
- job ID を task / event / TTS session ID / response で共有する。
- attachment event type / from / to / content / route / jobID / sessionID / channel / chatID を変えない。
- TTS session ID は TTS enabled の場合だけ `sessionID-jobID` 形式で作る。
- TTS session ID が空の場合、TTS lifecycle は no-op のまま。

## 7. 実装手順

1. baseline test を実行する。
2. `messageTaskContextBuilder` を追加する。
3. `MessageOrchestrator` に `taskContexts *messageTaskContextBuilder` field を追加する。
4. `NewMessageOrchestrator` で builder を組み立てる。
5. TTS enabled 判定は `MessageOrchestrator` の method または closure として渡す。
6. `ProcessMessage` の `buildTaskForRequest` 呼び出しを builder 経由にする。
7. `buildTaskForRequest` は削除するか thin wrapper にする。
8. gofmt を実行する。
9. focused test と全体 test を実行する。
10. `docs/refactor/Phase12_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

ProcessMessage / session / response:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
```

event:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase11|TestPhase12|TestMessageOrchestrator_RouteChainContract_'
```

TTS:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- job ID が task / event / response / TTS session ID でずれる。
- attachment event が落ちる。
- attachment event を execution log と混同する。
- TTS enabled 判定が古い bridge を参照する。
- TTS session ID を常に作ってしまい、TTS no-op 契約を崩す。
- `ProcessMessage` の順序を変える。

## 10. 完了条件

Phase12 の完了条件は次の通り。

- `docs/refactor/Phase12_task_context_builder境界整理実装仕様.md` が作成されている。
- 現在の task context builder 構造が棚卸しされている。
- `messageTaskContextBuilder` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Phase8-11 契約を維持する方針が明記されている。
- コード変更は行っていない。
