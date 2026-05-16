# Phase19 完了判定

## Phase の目的

Phase19 は `DistributedOrchestrator` に残っていた `executeDistributed` / `executeDistributedDirect` の route 分岐を、分散実行専用の `distributedRouteDispatcher` へ分離する。

目的は構造整理であり、transport executor、Code route retry、node selection、attribution guard 本体、event / evidence、TTS lifecycle、session lifecycle、autonomous coordinator、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedRouteDispatcher`
  - 入力: context、task、route、session ID、TTS session ID
  - 出力: response string、error
  - 副作用: agent 実行、CentralMemory record、event / note emission、TTS / VTuber stream hook と push、transport executor callback 呼び出し
  - 永続化: DB 永続化は行わない。CentralMemory record は維持する
  - ログ: route dispatcher 自体の新規ログは追加しない
  - エラー契約: downstream agent / transport error をそのまま返し、fallback response を正常系として作らない

## 維持した既存挙動

- CHAT は autonomous coordinator を通さず direct execution に入る。
- 非 CHAT は autonomous coordinator を通る。
- CODE route 成功時の user response event / note / TTS push を維持する。
- WILD / ANALYZE heavy の stream finalize を維持する。
- local route の attribution guard と CentralMemory task/result record を維持する。
- remote route の message context `route` / `channel` / `chat_id` を維持する。
- remote route の response event / note / TTS push を維持する。
- transport executor 本体と Code route retry は変更しない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_routes.go`
- `internal/application/orchestrator/distributed_orchestrator_phase19_routes_test.go`
- `docs/refactor/Phase19_完了判定.md`

## 検証

Phase19 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase19|TestDistributedOrchestrator_.*(LocalRoute|CODE|OPS|Retry|TTSBridge)'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed route dispatch の詳細が `distributedRouteDispatcher` へ分離されている。
- `DistributedOrchestrator` 本体は route dispatcher の構築と委譲だけを持つ。
- CHAT が autonomous executor を bypass すること、非 CHAT が autonomous executor を使うことがテストで固定されている。
- Phase19 の検証コマンドが成功している。
- Phase19 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase20 として、`DistributedOrchestrator` の transport executor 境界整理に進む候補がある。`executeToAgent` / mailbox / SSH / local router / timeout が絡むため、実装前に timeout、retry、message context、CentralMemory record の契約を厚めに固定する。
