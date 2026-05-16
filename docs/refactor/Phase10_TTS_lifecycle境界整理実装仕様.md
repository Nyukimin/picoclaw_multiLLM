# Phase10 TTS lifecycle 境界整理実装仕様

## 1. Phase10 の目的

Phase10 は、`MessageOrchestrator` に残っている TTS / VTuber lifecycle を `messageTTSLifecycle` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- TTS session start / end / stream hook / final push の責務を明確にする。
- route dispatcher から呼ばれる stream hook と push の dependency を整理する。
- 音声、口パク、Viewer 表示本文、event log、execution log を混同しない。
- TTS / VTuber degraded log を response success と混同しない。
- Phase6 route chain、Phase8 collaborator、Phase9 route dispatcher 契約を維持する。

Phase10 は構造整理であり、TTS / VTuber provider の挙動変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/message_orchestrator_tts_lifecycle.go`
  - `startTTSSessionForRoute`
  - `endTTSSession`
  - `withStreamHooks`
  - `pushTTS`
  - `speechModeForRoute`
- `internal/application/orchestrator/tts_support.go`
- `internal/application/orchestrator/vtuber_stream.go`
- `internal/application/orchestrator/stream_bundle.go`
- `internal/application/orchestrator/message_orchestrator.go`
  - constructor
  - `SetTTSBridge`
  - `SetVTuberBridge`
  - `ProcessMessage`
- `internal/application/orchestrator/message_orchestrator_routes.go`
  - route dispatcher へ渡す stream hook / push dependency
- TTS focused tests
- Phase9 route dispatcher tests

## 3. 対象外

Phase10 では次を対象外にする。

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

## 4. 現在の TTS lifecycle 構造

### `startTTSSessionForRoute`

route decision 後、route execution 前に呼ばれる。TTS bridge があり TTS session ID が空でない場合だけ `TTSSessionStart` を組み立て、`TTSBridge.StartSession` を呼ぶ。

契約:

- TTS bridge が nil の場合は何もしない。
- TTS session ID が空の場合は何もしない。
- route に応じて speaker / voice / speech mode / event / emotion context を決める。
- StartSession error は degraded log にする。
- StartSession error を route execution error に変換しない。

### `endTTSSession`

route execution 成功後、session save 前に呼ばれる。

契約:

- TTS session ID が空の場合は何もしない。
- EndSession error は degraded log にする。
- EndSession error を route execution success / failure 判定に混ぜない。

### `withStreamHooks`

agent stream callback を差し替え、前段 callback、`agent.thinking` event、TTS stream forwarder、VTuber stream forwarder を接続する。

契約:

- 既存 callback がある場合は先に呼ぶ。
- `agent.thinking` event は token ごとに emit する。
- TTS / VTuber stream forwarder はそれぞれの bridge と session ID が有効な場合だけ動く。
- stream token は表示本文の唯一根拠にしない。

### `pushTTS`

route execution 成功後の final response を TTS / VTuber に push する。

契約:

- TTS payload は `ttsapp.FilterSpeakableText` と `ttsapp.PlanEmotion` を通す。
- VTuber request は `buildVTuberRequest` が許可した場合だけ push する。
- push error は degraded log にする。
- push error を success response に変換しない。

## 5. 提案する collaborator

### `messageTTSLifecycle`

`messageTTSLifecycle` は private struct とする。初期段階では interface 化しない。

配置:

- `message_orchestrator_tts_lifecycle.go` に定義する。
- `MessageOrchestrator` に field として持たせる。
- `NewMessageOrchestrator` で組み立てる。

dependency:

- `TTSBridge`
- `VTuberBridge`
- event emit function

setter 反映:

- `SetTTSBridge` 後は lifecycle が最新 TTS bridge を使う。
- `SetVTuberBridge` 後は lifecycle が最新 VTuber bridge を使う。
- `SetEventListener` は event emit function が `MessageOrchestrator.emit` を経由するため、最新 listener を使う。

route dispatcher との接続:

- `messageRouteDispatcher` へ渡す `withStreamHooks` dependency は `messageTTSLifecycle.WithStreamHooks` にする。
- `messageRouteDispatcher` へ渡す `pushTTS` dependency は `messageTTSLifecycle.Push` にする。
- `ProcessMessage` の start / end 呼び出しは `messageTTSLifecycle.StartSessionForRoute` / `EndSession` にする。

## 6. `messageTTSLifecycle` の契約

入力:

- `context.Context`
- `ProcessMessageRequest`
- `task.JobID`
- `routing.Decision`
- `routing.Route`
- session ID
- route event type
- response text
- stream token

出力:

- stream hook context
- `*streamBundle`
- start / end / push は値を返さない。

副作用:

- TTS bridge start / push / end。
- VTuber bridge push。
- `agent.thinking` event emission。
- degraded log。

永続化:

- 直接永続化しない。

ログ:

- TTS start degraded: `[MessageOrch] TTS route update degraded:`
- TTS end degraded: `[MessageOrch] TTS end degraded:`
- TTS push degraded: `[MessageOrch] TTS push degraded:`
- VTuber push degraded: `[MessageOrch] VTuber push degraded:`

エラー契約:

- TTS / VTuber bridge error は degraded log にする。
- degraded log を route execution success と混同しない。
- TTS lifecycle error を fallback response に変換しない。

変更してはいけない既存挙動:

- StartSession / EndSession / PushText の呼び出し条件。
- `speakerForRoute` / `voiceForSpeaker` / `speechModeForRoute` / `eventForRoute` の意味。
- 前段 stream callback を呼ぶ順序。
- `agent.thinking` event emission。
- TTS chunk は表示本文の唯一根拠ではない。

## 7. 実装手順

1. baseline test を実行する。
2. `messageTTSLifecycle` を `message_orchestrator_tts_lifecycle.go` に追加する。
3. `MessageOrchestrator` に `ttsLifecycle *messageTTSLifecycle` field を追加する。
4. `NewMessageOrchestrator` で lifecycle を組み立てる。
5. `SetTTSBridge` / `SetVTuberBridge` で lifecycle の bridge も更新する。
6. `ProcessMessage` の start / end 呼び出しを lifecycle 経由にする。
7. route dispatcher に渡す stream hook / push dependency を lifecycle method に差し替える。
8. 既存 TTS helper の意味を変えない。
9. gofmt を実行する。
10. focused test と全体 test を実行する。
11. `docs/refactor/Phase10_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

TTS:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

route chain:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase9|TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- TTS degraded を success と混同する。
- stream callback の前段 callback を落とす。
- Viewer 表示本文を音声 chunk に依存させる。
- 口パクと音声 push の境界を混同する。
- route dispatcher の event order を壊す。
- provider 挙動変更に踏み込む。
- SetTTSBridge / SetVTuberBridge 後に古い bridge を使い続ける。

## 10. 完了条件

Phase10 の完了条件は次の通り。

- `docs/refactor/Phase10_TTS_lifecycle境界整理実装仕様.md` が作成されている。
- 現在の TTS lifecycle 構造が棚卸しされている。
- `messageTTSLifecycle` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Phase6 / Phase8 / Phase9 契約を維持する方針が明記されている。
- コード変更は行っていない。
