# Phase21 完了判定

## Phase の目的

Phase21 は `DistributedOrchestrator.executeCodeViaShiro` に残っていた Code route execution / coder selection / coder retry を、分散 Code 専用の `distributedCodeExecutionCoordinator` へ分離する。

目的は構造整理であり、route dispatcher、transport executor、node selector、proposal / worker execution contract、event / evidence、TTS lifecycle、session lifecycle、autonomous coordinator、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedCodeExecutionCoordinator`
  - 入力: context、task、route、session ID、job ID
  - 出力: response string、error
  - 副作用: Coder / Shiro transport execution、CentralMemory record、event / note emission、retry instruction generation
  - 永続化: DB 永続化は行わない。CentralMemory に Coder / Shiro message を記録する
  - ログ: `[DistributedOrch] code handoff route=... target=... job=...`
  - エラー契約: coder 未決定、retryable failure、retry budget exhausted を既存 message で返し、fallback response を正常系として作らない

## 維持した既存挙動

- coder が見つからない場合は `no coder mapped for route %s` を返す。
- attempt 0 は通常依頼、attempt > 0 は `worker.retry_request` と retry note を出す。
- coder message context に `route` / `retry_attempt` / `channel` / `chat_id` を入れる。
- coder config があれば `coder_config` を入れる。
- coder message / shiro task / exec message を CentralMemory に記録する。
- coder mailbox は receiveOnAgent `mio` を使う。
- coderResult.Proposal nil の場合は Shiro 整形へ回す。
- Proposal がある場合は Shiro Worker 実行へ回す。
- retryable failure は `buildCoderRetryInstruction` を使って次 attempt に進む。
- retry budget exhausted error を維持する。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_code.go`
- `internal/application/orchestrator/distributed_orchestrator_phase21_code_test.go`
- `docs/refactor/Phase21_完了判定.md`

## 検証

Phase21 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase21|TestDistributedOrchestrator_.*(CODE|Retry)|TestDistributedExecutionErrorClassification'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed Code route execution の詳細が `distributedCodeExecutionCoordinator` へ分離されている。
- `DistributedOrchestrator` 本体は Code execution coordinator の構築と委譲だけを持つ。
- coder config context、no coder error、retry instruction の主要契約がテストで固定されている。
- Phase21 の検証コマンドが成功している。
- Phase21 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase22 として、`DistributedOrchestrator` の coder selection / node selection 境界整理に進む候補がある。`routeToCoder`、`routeToCoderForMessage`、`isCoderConnected`、node capability selection をまとめて扱う。
