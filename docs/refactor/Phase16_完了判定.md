# Phase16 完了判定

## Phase の目的

Phase16 は `DistributedOrchestrator` に残っていた TTS / VTuber lifecycle の詳細を、分散実行専用の `distributedTTSLifecycle` へ分離する。

目的は構造整理であり、route dispatch、transport executor、autonomous coordinator、TTS / VTuber provider、Viewer、IdleChat、STT、LLM provider、runtime config の挙動は変更しない。

## 実装した境界

- `distributedTTSLifecycle`
  - 入力: context、request、job ID、routing decision、route、session / channel / chat ID、event type、response text、stream token
  - 出力: TTS session ID、stream hook context、`*streamBundle`
  - 副作用: TTS start / push / end、VTuber push、`agent.thinking` event emission、degraded log
  - 永続化: なし
  - ログ: `[DistributedOrch] TTS start degraded:`、`[DistributedOrch] TTS end degraded:`、`[DistributedOrch] TTS push degraded:`、`[DistributedOrch] VTuber push degraded:`
  - エラー契約: TTS / VTuber error は degraded log とし、route execution error へ変換しない

## 維持した既存挙動

- TTS bridge が nil の場合は session ID を作らない。
- StartSession error 時は `ttsSessionID` を空にする。
- 空 session ID では EndSession を呼ばない。
- EndSession error は degraded log とし、処理結果へ混ぜない。
- previous stream callback を先に呼ぶ。
- stream token ごとに `agent.thinking` event を emit する。
- TTS / VTuber stream forwarder は bridge と session ID が有効な場合だけ動く。
- TTS chunk は Viewer 表示本文の唯一根拠にしない。
- MessageOrchestrator の `messageTTSLifecycle` とは共通化しない。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_tts_lifecycle.go`
- `internal/application/orchestrator/distributed_orchestrator_phase16_tts_lifecycle_test.go`
- `docs/refactor/Phase16_完了判定.md`

## 検証

Phase16 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase16|TestDistributedOrchestrator_TTSBridge_|TestTTSStreamForwarder'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed TTS / VTuber lifecycle の詳細が `distributedTTSLifecycle` へ分離されている。
- `DistributedOrchestrator` 本体は TTS lifecycle の構築と委譲だけを持つ。
- StartSession success / failure、EndSession、stream hook の主要契約がテストで固定されている。
- Phase16 の検証コマンドが成功している。
- Phase16 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase17 として、`DistributedOrchestrator` の session lifecycle 境界整理に進む候補がある。`loadOrCreateSession` は load error をすべて新規 session にする既存挙動を持つため、仕様確認と契約テストを先に固定してから分離する。
