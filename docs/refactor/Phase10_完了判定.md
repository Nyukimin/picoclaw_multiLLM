# Phase10 完了判定

## 目的

Phase10 は、`MessageOrchestrator` に残っていた TTS / VTuber lifecycle を `messageTTSLifecycle` へ分離し、音声、口パク、Viewer event、response text の境界を維持したまま TTS 周辺責務を整理する段階である。

## 実施範囲

対象にした範囲は次の通り。

- `messageTTSLifecycle`
- `MessageOrchestrator.ProcessMessage` から TTS lifecycle への start / end 委譲
- route dispatcher へ渡す stream hook / push dependency の lifecycle 化
- `SetTTSBridge` / `SetVTuberBridge` 後の lifecycle dependency 更新
- TTS lifecycle 境界の契約テスト

対象外にした範囲は次の通り。

- event emitter port 化。
- task context builder。
- route dispatcher の追加分割。
- TTS / VTuber provider 挙動変更。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT。
- LLM provider。
- runtime config。
- distributed orchestrator。
- 未追跡の `tests/`。

## 追加・変更した collaborator

### `messageTTSLifecycle`

`message_orchestrator_tts_lifecycle.go` に追加した。

責務:

- TTS session start request を組み立てる。
- TTS session end を bridge に渡す。
- stream callback に前段 callback、`agent.thinking` event、TTS stream、VTuber stream を接続する。
- final response を TTS / VTuber に push する。

契約:

- TTS / VTuber bridge が nil の場合は no-op。
- TTS session ID が空の場合は no-op。
- bridge error は degraded log として扱う。
- degraded log を route execution success と混同しない。
- stream token は Viewer 表示本文の唯一根拠にしない。
- 前段 stream callback を維持する。

## `MessageOrchestrator` に残した責務

`MessageOrchestrator` には次を残した。

- request / response / agent interface / repository interface の定義。
- dependency holding。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。
- event emitter。
- task context builder。

TTS / VTuber lifecycle の詳細は `messageTTSLifecycle` に移した。

## 維持した契約

Phase10 では次を維持した。

- TTS start は route decision 後、route execution 前に行う。
- TTS end は route execution 成功後、session save 前に行う。
- TTS / VTuber push error は degraded log に留める。
- TTS degraded を success response に変換しない。
- route dispatcher の event order を変えない。
- `speakerForRoute` / `voiceForSpeaker` / `speechModeForRoute` / `eventForRoute` の意味を変えない。
- handler / DTO / SSE event / Viewer JS / CSS / IdleChat / STT / TTS provider / LLM provider / runtime config は変更しない。

## 追加した契約テスト

`internal/application/orchestrator/message_orchestrator_phase10_tts_lifecycle_test.go` を追加した。

追加した確認は次の通り。

- `SetTTSBridge` 後に lifecycle が最新 TTS bridge を使って start / push / end すること。
- `WithStreamHooks` が前段 stream callback を維持すること。
- `WithStreamHooks` が `agent.thinking` event を emit すること。

## 検証結果

baseline として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

実装途中の focused test として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

最終確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase9|TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
git diff --check
git diff --stat
```

## 完了条件との対応

Phase10 の完了条件に対する判定は次の通り。

- `messageTTSLifecycle` を追加した。
- TTS lifecycle の入力、出力、副作用、永続化、ログ、エラー契約を本書に記録した。
- route dispatcher へ渡す stream hook / push dependency を lifecycle method に差し替えた。
- TTS lifecycle と event emitter / task context builder を同時に collaborator 化していない。
- Phase6 / Phase8 / Phase9 契約を変更していない。
- 未追跡の `tests/` は触っていない。

## Phase11 前の確認事項

Phase11 の候補は次の通り。

1. event emitter port 化。
2. task context builder の attachment / TTS session ID 境界整理。

おすすめは event emitter port 化である。Phase10 で TTS lifecycle が event emit function を明示的に受け取る形になったため、次に event emitter を小さな port として切り出すと、Viewer event と execution log の境界をさらに明確にできる。
