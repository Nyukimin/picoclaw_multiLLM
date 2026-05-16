# Phase9 完了判定

## 目的

Phase9 は、`MessageOrchestrator` に残っていた route-specific execution を `messageRouteDispatcher` へ分離し、route dispatch の境界を明確にする段階である。

この完了判定では、`ProcessMessage` の top-level orchestration を維持したまま、CHAT / autonomous route 分岐、route direct dispatch、route-specific execution、CodeExecutor handoff を private collaborator に整理した結果を記録する。

## 実施範囲

対象にした範囲は次の通り。

- `messageRouteDispatcher`
- `MessageOrchestrator.ProcessMessage` から dispatcher への委譲
- autonomous coordinator に渡す route direct executor の dispatcher 化
- `SetWildAgent` / `SetHeavyAgent` 後の dispatcher dependency 更新
- route dispatcher 境界の契約テスト

対象外にした範囲は次の通り。

- TTS lifecycle collaborator 化。
- event emitter collaborator 化。
- task context builder collaborator 化。
- CodeExecutor 再分割。
- WorkerExecutionService 内部。
- ToolRunner / PolicyEngine。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- distributed orchestrator。
- 未追跡の `tests/`。

## 追加・変更した collaborator

### `messageRouteDispatcher`

`message_orchestrator_routes.go` に追加した。

責務:

- `ExecuteTask` で `CHAT` と non-CHAT route を分岐する。
- `CHAT` は autonomous executor を通さず、Mio chat route を直接実行する。
- non-CHAT route は autonomous execution coordinator に委譲する。
- `ExecuteDirect` で OPS / CODE / WILD / PLAN / ANALYZE / RESEARCH を route-specific execution へ分岐する。
- CODE 系 route は `CodeExecutor.ExecuteCode` に委譲する。

契約:

- 入力は `context.Context`、`task.Task`、`routing.Route`、session ID、channel、chat ID、TTS session ID。
- 出力は response text と error。
- 副作用は agent call、event emission、stream hook 接続、TTS / VTuber push または finalize、CodeExecutor 呼び出し。
- dispatcher 自体は直接永続化しない。
- dispatcher 自体は新しいログを増やさない。
- unsupported route、nil wild、CodeExecutor error、Generate error を success response にしない。

## `MessageOrchestrator` に残した責務

`MessageOrchestrator` には次を残した。

- request / response / agent interface / repository interface の定義。
- dependency holding。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。
- TTS lifecycle。
- event emitter。
- task context builder。

route-specific execution の詳細は `messageRouteDispatcher` に移した。

## 維持した契約

Phase9 では次を維持した。

- `CHAT` は autonomous executor を通らない。
- non-CHAT route は autonomous execution coordinator を通る。
- autonomous coordinator の route direct executor は dispatcher の `ExecuteDirect` を使う。
- CODE 系 route は `CodeExecutor.ExecuteCode` を通る。
- Phase6 の Shiro relay / CodeExecutor / WorkerExecutionService 契約を維持する。
- Worker error / Generate error を success response に変換しない。
- TTS push / stream finalize のタイミングを変えない。
- ANALYZE の heavy nil fallback を変えない。
- WILD の nil error を変えない。
- handler / DTO / SSE event / Viewer JS / CSS / IdleChat / STT / TTS provider / LLM provider / runtime config は変更しない。

## 追加した契約テスト

`internal/application/orchestrator/message_orchestrator_phase9_route_dispatcher_test.go` を追加した。

追加した確認は次の通り。

- `CHAT` route が autonomous executor を呼ばないこと。
- non-CHAT route が autonomous executor を呼ぶこと。
- `SetHeavyAgent` 後に ANALYZE route が最新 heavy agent を使うこと。

## 検証結果

baseline として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

実装途中の focused test として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase9|TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase8'
```

最終確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

## 完了条件との対応

Phase9 の完了条件に対する判定は次の通り。

- `messageRouteDispatcher` を追加した。
- route dispatcher の入力、出力、副作用、永続化、ログ、エラー契約を本書に記録した。
- `ProcessMessage` が top-level orchestration として説明できる状態を維持した。
- route dispatcher と TTS lifecycle / event emitter / task context builder を同時に collaborator 化していない。
- Phase6 / Phase8 契約を変更していない。
- 未追跡の `tests/` は触っていない。

## Phase10 前の確認事項

Phase10 の候補は次の通り。

1. TTS lifecycle 境界整理。
2. event emitter port 化。
3. task context builder の attachment / TTS session ID 境界整理。

おすすめは TTS lifecycle 境界整理である。route dispatcher から TTS push / stream finalize の呼び出しが残っているため、次に音声・口パク・Viewer event の境界を明文化すると、その後の event emitter port 化を安全に進めやすい。
