# Phase15 完了判定

## Phase の目的

Phase15 は `DistributedOrchestrator` に残っていた distributed event emission と execution evidence 保存の詳細を、分散実行専用の小さな境界へ分離する。

目的は `DistributedOrchestrator` 本体を、分散実行の流れを読むための composition に近づけることであり、route、transport、TTS、Viewer、IdleChat、STT/TTS provider、LLM provider の挙動は変更しない。

## 実装した境界

- `distributedEventPort`
  - 入力: event type、from/to、content、route、job/session/channel/chat ID、または `domaintransport.Message`
  - 出力: なし
  - 副作用: `EventListener.OnEvent` への通知
  - 永続化: なし
  - ログ: なし
  - エラー契約: listener が nil の場合は no-op

- `distributedEvidenceReporter`
  - 入力: context、job ID、goal、route、startedAt、finishedAt、実行エラー
  - 出力: なし
  - 副作用: `ReportStore.Save` への保存
  - 永続化: `domainexecution.ExecutionReport`
  - ログ: 保存失敗時のみ `[DistributedOrch] evidence save failed`
  - エラー契約: reporter nil、job ID 空、goal 空は no-op。保存失敗はログに残し、既存の処理結果へ伝播しない

## 維持した既存挙動

- distributed event listener が nil の場合は no-op のままにする。
- distributed event の skipped ログは追加しない。
- `EmitProgress` は `domaintransport.Message` の context から route、channel、chat ID を取り出す。
- evidence は `ReportStore` が未設定、job ID 空、goal 空のとき保存しない。
- evidence の route は大文字化する。
- evidence の success / failure、acceptance、verification、steps、error kind の生成規則を維持する。
- route path、handler、DTO、SSE event、Viewer 表示、IdleChat 契約、STT/TTS provider、LLM provider には触れない。
- fallback を正常系として扱わない。
- Viewer 表示、音声、口パク、ログを混同しない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_events.go`
- `internal/application/orchestrator/distributed_orchestrator_evidence.go`
- `internal/application/orchestrator/distributed_orchestrator_phase15_event_evidence_test.go`
- `docs/refactor/Phase15_完了判定.md`

## 検証

Phase15 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase15|TestDistributed'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed event emission の詳細が `distributedEventPort` へ分離されている。
- distributed execution evidence 保存の詳細が `distributedEvidenceReporter` へ分離されている。
- `DistributedOrchestrator` 本体は event/evidence の構築と委譲だけを持つ。
- listener nil no-op と evidence no-op の契約がテストで固定されている。
- success / failure evidence の主要契約がテストで固定されている。
- Phase15 の検証コマンドが成功している。
- Phase15 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase16 として、`DistributedOrchestrator` の TTS lifecycle 境界を整理する候補がある。`MessageOrchestrator` 側の Phase10 と同じく、音声開始、エラー時の session ID 空化、完了通知、VTuber bridge への接続を混同しないことを前提に、小さな collaborator へ切り出す。
