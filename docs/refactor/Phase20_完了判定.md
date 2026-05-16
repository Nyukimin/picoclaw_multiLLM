# Phase20 完了判定

## Phase の目的

Phase20 は `DistributedOrchestrator` に残っていた mailbox / SSH / local router execution を、分散 transport 専用の `distributedTransportExecutor` へ分離する。

目的は構造整理であり、route dispatcher、Code route retry、node selection、coder config、attribution guard、event / evidence、TTS lifecycle、session lifecycle、autonomous coordinator、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedTransportExecutor`
  - 入力: context、target agent、transport message、receiveOnAgent、SSH transport
  - 出力: transport message、string response、error
  - 副作用: SSH send / receive、local router inbound message、receive wait、CentralMemory record、mailbox / agent error event emission、log 出力
  - 永続化: DB 永続化は行わない。CentralMemory に transport message を記録する
  - エラー契約: send / receive / MessageTypeError を既存 message で返し、fallback response を正常系として作らない

## 維持した既存挙動

- `executeToAgent` は `receiveOnAgent = msg.From` で mailbox 実行に委譲する。
- SSH transport があれば SSH 経路を使い、なければ local router 経路を使う。
- `mailbox.sent` / `mailbox.waiting` / `mailbox.received` / `mailbox.error` / `agent.error` event を維持する。
- SSH / local とも受信 result を CentralMemory に記録する。
- SSH / local とも MessageTypeError は `agent <from> returned error: <content>` として返す。
- local receive transport は receiveOnAgent、なければ mio に fallback する。両方なければ error。
- timeout は `distributedWaitTimeout` 相当を使う。
- fallback を正常系として扱わない。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_transport.go`
- `internal/application/orchestrator/distributed_orchestrator_phase20_transport_test.go`
- `docs/refactor/Phase20_完了判定.md`

## 検証

Phase20 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase20|TestDistributedWaitTimeout|TestDistributedOrchestrator_.*(Retry|CODE|LocalRoute)'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed transport execution の詳細が `distributedTransportExecutor` へ分離されている。
- `DistributedOrchestrator` 本体は transport executor の構築と委譲だけを持つ。
- `executeToAgent` の receiveOnAgent 契約と local receive transport missing error がテストで固定されている。
- Phase20 の検証コマンドが成功している。
- Phase20 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase21 として、`DistributedOrchestrator` の Code route execution / coder selection 境界整理に進む候補がある。`executeCodeViaShiro`、`routeToCoderForMessage`、coder config、node selector、retry instruction が絡むため、まず契約を厚めに固定してから切り出す。
