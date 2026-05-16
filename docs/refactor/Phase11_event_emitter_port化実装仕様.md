# Phase11 event emitter port 化実装仕様

## 1. Phase11 の目的

Phase11 は、`MessageOrchestrator.emit` / `emitMessageReceived` を `messageEventPort` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- Viewer event emission を小さな port として明確化する。
- Viewer event と execution log / response text / audio chunk / lipsync を混同しない。
- nil listener の skipped log を維持する。
- Phase6 route chain の event order を維持する。
- Phase8 / Phase9 / Phase10 で collaborator に渡している event emitter dependency を整理する。

Phase11 は構造整理であり、event payload、SSE event、Viewer JS / CSS の挙動変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/message_orchestrator_events.go`
  - `emit`
  - `emitMessageReceived`
- `internal/application/orchestrator/event.go`
  - `EventListener`
  - `OrchestratorEvent`
  - `NewEvent`
- `internal/application/orchestrator/message_orchestrator.go`
  - constructor
  - `SetEventListener`
  - `ProcessMessage`
- collaborator に渡している `messageEventEmitter`
- CodeExecutor event emitter 接続
- route chain / event / TTS tests

## 3. 対象外

Phase11 では次を対象外にする。

- task context builder collaborator 化。
- event payload / DTO / SSE event の変更。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- distributed orchestrator。
- CodeExecutor 再分割。
- WorkerExecutionService 内部。
- 未追跡の `tests/`。

## 4. 現在の event emitter 構造

### `emit`

`MessageOrchestrator.emit` は listener が nil の場合、panic せず skipped log を出して戻る。listener がある場合は `NewEvent` で `OrchestratorEvent` を作り、listener に渡す。

契約:

- nil listener は error ではない。
- nil listener でも skipped log を残す。
- listener がある場合、emit log を出す。
- `NewEvent` の timestamp は JST。

### `emitMessageReceived`

`emitMessageReceived` は request から `message.received` event を user -> mio として emit する。

契約:

- route は未決定のため空文字。
- job ID も task 作成前のため空文字。
- route decision より前に emit する。

### collaborator dependency

Phase8 以降、`messageEventEmitter` function が次に渡されている。

- `preRoutingCommandHandler`
- `routeDecisionCoordinator`
- `autonomousExecutionCoordinator`
- `messageRouteDispatcher`
- `messageTTSLifecycle`
- `DefaultCodeExecutor`

Phase11 では function の実体を `messageEventPort.Emit` に寄せる。

## 5. 提案する collaborator

### `messageEventPort`

`messageEventPort` は private struct とする。初期段階では interface 化しない。

配置:

- `message_orchestrator_events.go` に定義する。
- `MessageOrchestrator` に field として持たせる。
- `NewMessageOrchestrator` で組み立てる。

dependency:

- `EventListener`

setter 反映:

- `SetEventListener` 後は port が最新 listener を使う。
- `DefaultCodeExecutor.SetEventEmitter` へ渡す function は `messageEventPort.Emit` にする。

method:

- `Emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string)`
- `EmitMessageReceived(req ProcessMessageRequest)`
- `SetListener(listener EventListener)`

互換 wrapper:

- 必要なら `MessageOrchestrator.emit` / `emitMessageReceived` は thin wrapper として残す。
- ただし collaborator へ渡す dependency は port method に寄せる。

## 6. `messageEventPort` の契約

入力:

- event type
- from
- to
- content
- route
- job ID
- session ID
- channel
- chat ID
- `ProcessMessageRequest`

出力:

- なし。

副作用:

- log output。
- listener callback。

永続化:

- 直接永続化しない。

ログ:

- listener nil の場合: `[MessageOrch] emit SKIPPED: no listener ...`
- listener ありの場合: `[MessageOrch] emit: eventType=...`

エラー契約:

- listener nil は error にしない。
- listener callback の戻り値はないため、emit は error を返さない。

差し替え可能性:

- 将来 event hub / recorder / viewer event adapter へ置き換えやすくする。
- ただし Phase11 では外部公開 interface は作らない。

変更してはいけない既存挙動:

- event type / from / to / content / route / jobID / sessionID / channel / chatID を変えない。
- route chain event order を変えない。
- nil listener skipped log を消さない。
- Viewer event を execution log として扱わない。
- audio chunk / lipsync / response text を event port に統合しない。

## 7. 実装手順

1. baseline test を実行する。
2. `messageEventPort` を追加する。
3. `MessageOrchestrator` に `events *messageEventPort` field を追加する。
4. `NewMessageOrchestrator` で port を初期化する。
5. collaborator へ渡す event emitter function を `events.Emit` に差し替える。
6. `SetEventListener` で port listener を更新する。
7. `DefaultCodeExecutor.SetEventEmitter` に `events.Emit` を渡す。
8. `ProcessMessage` の message received emission を `events.EmitMessageReceived` にする。
9. `MessageOrchestrator.emit` / `emitMessageReceived` は必要なら thin wrapper として残す。
10. gofmt を実行する。
11. focused test と全体 test を実行する。
12. `docs/refactor/Phase11_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

route chain:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

TTS:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder'
```

event:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase11|TestMessageOrchestrator_RouteChainContract_'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- route chain event order を壊す。
- nil listener を panic に変える。
- nil listener skipped log を消す。
- Viewer event と execution log を混同する。
- raw content / response text / audio chunk を同じ扱いにする。
- CodeExecutor への event emitter 接続を落とす。
- TTS lifecycle の `agent.thinking` event を落とす。

## 10. 完了条件

Phase11 の完了条件は次の通り。

- `docs/refactor/Phase11_event_emitter_port化実装仕様.md` が作成されている。
- 現在の event emitter 構造が棚卸しされている。
- `messageEventPort` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Phase6 / Phase8 / Phase9 / Phase10 契約を維持する方針が明記されている。
- コード変更は行っていない。
