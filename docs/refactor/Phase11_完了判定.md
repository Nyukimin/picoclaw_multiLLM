# Phase11 完了判定

## 目的

Phase11 は、`MessageOrchestrator.emit` / `emitMessageReceived` を `messageEventPort` へ分離し、Viewer event emission の境界を明確にする段階である。

## 実施範囲

対象にした範囲は次の通り。

- `messageEventPort`
- `MessageOrchestrator` constructor での event port 初期化
- collaborator へ渡す event emitter dependency の port 化
- CodeExecutor event emitter 接続の port 化
- `SetEventListener` 後の port listener 更新
- event port 境界の契約テスト

対象外にした範囲は次の通り。

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

## 追加・変更した collaborator

### `messageEventPort`

`message_orchestrator_events.go` に追加した。

責務:

- listener が nil の場合に skipped log を出す。
- listener がある場合に `NewEvent` で `OrchestratorEvent` を作成して listener に渡す。
- `message.received` event を request から組み立てる。

契約:

- 入力は event type、from、to、content、route、jobID、sessionID、channel、chatID、または `ProcessMessageRequest`。
- 出力はなし。
- 副作用は log output と listener callback。
- 直接永続化しない。
- nil listener は error にしない。
- event payload の意味を変えない。

## `MessageOrchestrator` に残した責務

`MessageOrchestrator` には次を残した。

- request / response / agent interface / repository interface の定義。
- dependency holding。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。
- task context builder。

`emit` / `emitMessageReceived` は互換 wrapper として残し、実体は `messageEventPort` に委譲した。

## 維持した契約

Phase11 では次を維持した。

- nil listener は panic しない。
- nil listener skipped log を維持する。
- route chain event order を維持する。
- `message.received` は route decision より前に user -> mio として emit する。
- `message.received` は route / jobID を持たない。
- CodeExecutor への event emitter 接続を維持する。
- TTS lifecycle の `agent.thinking` event を維持する。
- Viewer event と execution log / response text / audio chunk / lipsync を混同しない。

## 追加した契約テスト

`internal/application/orchestrator/message_orchestrator_phase11_event_port_test.go` を追加した。

追加した確認は次の通り。

- nil listener でも `Emit` / `EmitMessageReceived` が panic しないこと。
- `SetListener` 後に最新 listener が event を受け取ること。
- `message.received` が user -> mio で route / jobID を持たないこと。

## 検証結果

baseline として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

実装途中の focused test として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_RouteChainContract_|TestMessageOrchestrator_CodeRoute_|TestCodeExecutor_'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase11|TestMessageOrchestrator_RouteChainContract_'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder'
```

最終確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

## 完了条件との対応

Phase11 の完了条件に対する判定は次の通り。

- `messageEventPort` を追加した。
- event port の入力、出力、副作用、永続化、ログ、エラー契約を本書に記録した。
- collaborator / CodeExecutor に渡す event emitter dependency を port method に寄せた。
- task context builder を同時に collaborator 化していない。
- Phase6 / Phase8 / Phase9 / Phase10 契約を変更していない。
- 未追跡の `tests/` は触っていない。

## Phase12 前の確認事項

Phase12 のおすすめは task context builder の attachment / TTS session ID 境界整理である。Phase11 で event port が明確になったため、attachment event emission を含む task context builder を安全に切り出しやすい。
