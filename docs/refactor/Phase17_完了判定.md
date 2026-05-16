# Phase17 完了判定

## Phase の目的

Phase17 は `DistributedOrchestrator` に残っていた session load / create / task add / save の詳細を、分散実行専用の `distributedSessionLifecycle` へ分離する。

目的は構造整理であり、session policy、route dispatch、transport executor、TTS lifecycle、event / evidence、Viewer、IdleChat、STT/TTS provider、LLM provider、runtime config の挙動は変更しない。

## 実装した境界

- `distributedSessionLifecycle`
  - 入力: context、`ProcessMessageRequest`、`*session.Session`、`task.Task`
  - 出力: `*session.Session` または error
  - 副作用: session load、task history 更新、session save、log 出力
  - 永続化: `SessionRepository.Load` / `SessionRepository.Save`
  - ログ: `[DistributedOrch] Session loaded/created:`、`[DistributedOrch] ProcessMessage ERROR: failed to save session:`
  - エラー契約: load error は新規 session に変換し、save error は caller に返す

## 維持した既存挙動

- load error は種類を問わず新規 session にする。
- load error を `ProcessMessage` error にしない。
- route execution 成功後だけ task add / save を行う。
- save 前に `sess.AddTask(t)` を行う。
- save error は `[DistributedOrch] ProcessMessage ERROR: failed to save session:` としてログに残す。
- `ProcessMessage` は save error を `failed to save session: ...` として返す。
- MessageOrchestrator の `messageSessionLifecycle` とは共通化しない。
- session save と evidence save を混同しない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_session.go`
- `internal/application/orchestrator/distributed_orchestrator_phase17_session_lifecycle_test.go`
- `docs/refactor/Phase17_完了判定.md`

## 検証

Phase17 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase17|TestDistributedOrchestrator_ProcessMessage_(LocalRoute|SavesEvidenceOnSuccess|SavesEvidenceOnChatError)'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed session lifecycle の詳細が `distributedSessionLifecycle` へ分離されている。
- `DistributedOrchestrator` 本体は session lifecycle の構築と委譲だけを持つ。
- load error 全般の新規 session 化、existing session 利用、save 時の task add、save error の主要契約がテストで固定されている。
- Phase17 の検証コマンドが成功している。
- Phase17 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase18 として、`DistributedOrchestrator` の autonomous execution coordinator 境界整理に進む候補がある。route direct executor と distributed route dispatcher の結合があるため、まず `executeAutonomousDistributed` の request assembly / observe / verify / retry contract だけを小さく固定する。
