# Phase16 distributed TTS lifecycle 境界整理実装仕様

## 1. Phase16 の目的

Phase16 は、`DistributedOrchestrator` に残っている TTS / VTuber lifecycle を `distributedTTSLifecycle` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- TTS session start / end / stream hook / final push の責務を明確にする。
- 音声、口パク、Viewer 表示本文、event log、execution log を混同しない。
- TTS / VTuber degraded log を route execution success と混同しない。
- Phase15 で整理した event / evidence 境界を維持する。
- route dispatch、transport executor、autonomous coordinator には踏み込まない。
- MessageOrchestrator の `messageTTSLifecycle` と安易に共通化せず、分散側の既存契約を固定する。

Phase16 は構造整理であり、TTS / VTuber provider の挙動変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `ProcessMessage` の TTS start / end。
  - `withStreamHooks`
  - `pushTTS`
  - `SetTTSBridge`
  - `SetVTuberBridge`
- 新規追加する `distributedTTSLifecycle`
- `internal/application/orchestrator/tts_support.go`
- `internal/application/orchestrator/vtuber_stream.go`
- distributed TTS focused tests。

## 3. 対象外

Phase16 では次を対象外にする。

- distributed route dispatcher 分割。
- distributed transport executor 分割。
- distributed autonomous coordinator 分割。
- event / evidence 境界の追加変更。
- node selection。
- coder config。
- MessageOrchestrator 側の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT。
- TTS / VTuber provider 実装。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の TTS lifecycle 構造

### start

route decision 後、route execution 前に呼ばれる。TTS bridge がある場合だけ `ttsSessionID` を `sessionID-jobID` 形式で作り、`TTSSessionStart` を組み立てて `TTSBridge.StartSession` を呼ぶ。

契約:

- TTS bridge が nil の場合は session ID を作らない。
- route に応じて speaker / voice / speech mode / event / emotion context を決める。
- StartSession error は `[DistributedOrch] TTS start degraded:` としてログに残す。
- StartSession error 時は `ttsSessionID` を空にする。
- StartSession error を route execution error に変換しない。

### end

route execution 成功後、session save 前に呼ばれる。

契約:

- `ttsSessionID` が空の場合は何もしない。
- EndSession error は `[DistributedOrch] TTS end degraded:` としてログに残す。
- EndSession error を route execution success / failure 判定に混ぜない。

### stream hook

agent stream callback を差し替え、前段 callback、`agent.thinking` event、TTS stream forwarder、VTuber stream forwarder を接続する。

契約:

- 既存 callback がある場合は先に呼ぶ。
- `agent.thinking` event は token ごとに emit する。
- TTS / VTuber stream forwarder はそれぞれの bridge と session ID が有効な場合だけ動く。
- stream token は Viewer 表示本文の唯一根拠にしない。

### push

route execution 成功後の final response を TTS / VTuber に push する。

契約:

- TTS payload は `ttsapp.FilterSpeakableText` と `ttsapp.PlanEmotion` を通す。
- VTuber request は `buildVTuberRequest` が許可した場合だけ push する。
- push error は degraded log にする。
- push error を success response に変換しない。

## 5. 提案する collaborator

### `distributedTTSLifecycle`

`distributedTTSLifecycle` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_tts_lifecycle.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `TTSBridge`
- `VTuberBridge`
- event emit function

setter 反映:

- `SetTTSBridge` 後は lifecycle が最新 TTS bridge を使う。
- `SetVTuberBridge` 後は lifecycle が最新 VTuber bridge を使う。
- `SetEventListener` は event emit function が `DistributedOrchestrator.emit` を経由するため、最新 listener を使う。

MessageOrchestrator の `messageTTSLifecycle` と共通化しない理由:

- distributed 側は StartSession error 時に `ttsSessionID` を空にする既存契約を持つ。
- distributed 側は route dispatch、transport、mailbox、remote result と TTS が接続されるため、先に分散専用境界として固定する方が安全である。
- 現時点で共通 interface を作ると、差し替え可能性よりも抽象化コストが上がる。

## 6. `distributedTTSLifecycle` の契約

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

- start は TTS session ID を返す。
- stream hook は callback を差し込んだ `context.Context` と `*streamBundle` を返す。
- end / push は値を返さない。

副作用:

- TTS bridge start / push / end。
- VTuber bridge push。
- `agent.thinking` event emission。
- degraded log。

永続化:

- 直接永続化しない。

ログ:

- TTS start degraded: `[DistributedOrch] TTS start degraded:`
- TTS end degraded: `[DistributedOrch] TTS end degraded:`
- TTS push degraded: `[DistributedOrch] TTS push degraded:`
- VTuber push degraded: `[DistributedOrch] VTuber push degraded:`

エラー契約:

- TTS / VTuber bridge error は degraded log にする。
- StartSession error 時は空 session ID を返す。
- degraded log を route execution success と混同しない。
- TTS lifecycle error を fallback response に変換しない。

変更してはいけない既存挙動:

- StartSession / EndSession / PushText の呼び出し条件。
- StartSession error 時に `ttsSessionID` を空にすること。
- `speakerForRoute` / `voiceForSpeaker` / `speechModeForRoute` / `eventForRoute` の意味。
- 前段 stream callback を呼ぶ順序。
- `agent.thinking` event emission。
- TTS chunk は Viewer 表示本文の唯一根拠ではない。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedTTSLifecycle` を `distributed_orchestrator_tts_lifecycle.go` に追加する。
3. `DistributedOrchestrator` に `ttsLifecycle *distributedTTSLifecycle` field を追加する。
4. `NewDistributedOrchestrator` で lifecycle を組み立てる。
5. `SetTTSBridge` / `SetVTuberBridge` で lifecycle の bridge も更新する。
6. `ProcessMessage` の TTS start / end を lifecycle 経由にする。
7. `withStreamHooks` / `pushTTS` を lifecycle 経由にする。
8. 既存 TTS helper の意味を変えない。
9. gofmt を実行する。
10. focused test と全体 test を実行する。
11. `docs/refactor/Phase16_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

TTS focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase16|TestDistributedOrchestrator_TTSBridge_|TestTTSStreamForwarder'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- StartSession error 時に `ttsSessionID` を空にしなくなる。
- EndSession を start failure 後に呼んでしまう。
- previous stream callback を落とす。
- `agent.thinking` event を落とす。
- TTS chunk を Viewer 表示本文の根拠にしてしまう。
- 音声、口パク、表示、ログを混同する。
- TTS / VTuber provider 挙動変更に踏み込む。
- MessageOrchestrator 側と無理に共通化する。

## 10. 完了条件

Phase16 の完了条件は次の通り。

- `docs/refactor/Phase16_distributed_TTS_lifecycle境界整理実装仕様.md` が作成されている。
- 現在の distributed TTS lifecycle 構造が棚卸しされている。
- `distributedTTSLifecycle` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Phase15 event / evidence 契約を維持する方針が明記されている。
- コード変更は行っていない。
