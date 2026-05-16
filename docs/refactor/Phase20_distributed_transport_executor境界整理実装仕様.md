# Phase20 distributed transport executor 境界整理実装仕様

## 1. Phase20 の目的

Phase20 は、`DistributedOrchestrator` に残っている mailbox / SSH / local router execution を `distributedTransportExecutor` へ分離する段階である。

目的は次の通り。

- `executeToAgent` / `executeToAgentViaMailbox` / `executeViaLocal` / `executeViaSSH` の責務を分散 transport 専用境界へ移す。
- mailbox event、timeout、CentralMemory record、SSH/local 分岐、error message の既存契約を維持する。
- Phase19 route dispatcher は transport callback へ依存したままにする。
- route dispatcher、Code route retry、node selection、coder config、attribution guard 本体には踏み込まない。

Phase20 は構造整理であり、transport protocol の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `executeViaSSH`
  - `executeToAgent`
  - `executeToAgentViaMailbox`
  - `executeViaLocal`
  - `distributedWaitTimeout` method
  - constructor
- 新規追加する `distributedTransportExecutor`
- transport focused tests。

## 3. 対象外

Phase20 では次を対象外にする。

- route dispatcher の追加分割。
- Code route retry 本体。
- node selection。
- coder config。
- attribution guard 本体。
- SSH transport 実装。
- `transport.MessageRouter` 実装。
- event / evidence / TTS / session / autonomous 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の transport execution 構造

### `executeToAgent`

`executeToAgent(ctx, targetAgent, msg)` は `receiveOnAgent = msg.From` として `executeToAgentViaMailbox` へ委譲する。

### `executeToAgentViaMailbox`

mailbox send log と `mailbox.sent` event を出し、target agent に SSH transport があれば SSH 経路、なければ local router 経路を使う。

SSH 経路では次を行う。

- `sshTransport.Send`
- timeout 付き `sshTransport.Receive`
- result を CentralMemory に記録
- `mailbox.waiting` / `mailbox.received` / `mailbox.error` event
- `MessageTypeError` の場合は `agent.error` event と error return

local 経路では `executeViaLocal` に委譲する。

### `executeViaLocal`

local router 経路では次を行う。

- `router.GetAgent(targetAgent)`
- `agentTransport.PutInboundMessage`
- receive transport は `receiveOnAgent`、なければ `mio` に fallback
- timeout 付き `receiveTransport.Receive`
- result を CentralMemory に記録
- `mailbox.waiting` / `mailbox.received` / `mailbox.error` event
- `MessageTypeError` の場合は `agent.error` event と error return

### `executeViaSSH`

現在は mailbox 経路とは別に、SSH transport へ direct send / receive して string response を返す helper として残っている。

## 5. 提案する collaborator

### `distributedTransportExecutor`

`distributedTransportExecutor` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_transport.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `transport.MessageRouter`
- `map[string]domaintransport.Transport`
- `session.CentralMemory`
- progress event emitter
- timeout resolver

## 6. `distributedTransportExecutor` の契約

入力:

- `context.Context`
- target agent
- `domaintransport.Message`
- receiveOnAgent
- SSH transport

出力:

- `domaintransport.Message`
- string response
- error

副作用:

- SSH transport send / receive。
- local router inbound message。
- receive transport wait。
- CentralMemory record。
- mailbox / agent error event emission。
- log 出力。

永続化:

- DB 永続化はしない。
- CentralMemory に transport message を記録する。

ログ:

- mailbox send / wait / receive / wait error。
- SSH direct send / receive。
- local send / receive。

エラー契約:

- send error は既存 message で wrap する。
- receive timeout / receive error は既存 message で wrap する。
- receive transport が見つからない場合は `receive transport not registered (agent=%s)` を返す。
- MessageTypeError は `agent <from> returned error: <content>` を返す。
- fallback response を正常系として作らない。

変更してはいけない既存挙動:

- `executeToAgent` は `receiveOnAgent = msg.From` で mailbox 実行に委譲する。
- SSH transport があれば SSH 経路を使い、なければ local router 経路を使う。
- `mailbox.sent` / `mailbox.waiting` / `mailbox.received` / `mailbox.error` / `agent.error` event。
- SSH / local とも受信 result を CentralMemory に記録する。
- local receive transport は receiveOnAgent、なければ mio に fallback する。
- timeout は `distributedWaitTimeout` 相当を使う。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedTransportExecutor` を `distributed_orchestrator_transport.go` に追加する。
3. `DistributedOrchestrator` に `transportExecutor *distributedTransportExecutor` field を追加する。
4. `NewDistributedOrchestrator` で executor を組み立てる。
5. `executeViaSSH` / `executeToAgent` / `executeToAgentViaMailbox` / `executeViaLocal` を executor への委譲に置き換える。
6. timeout resolver は `o.distributedWaitTimeout` 相当を渡し、coder timeout override を維持する。
7. 既存 event type / log prefix / error message を変えない。
8. gofmt を実行する。
9. focused test と全体 test を実行する。
10. `docs/refactor/Phase20_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

transport focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase20|TestDistributedWaitTimeout|TestDistributedOrchestrator_.*(Retry|CODE|LocalRoute)'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- receiveOnAgent の意味を変える。
- SSH/local 分岐順を変える。
- local receive transport fallback を落とす。
- CentralMemory record を落とす。
- mailbox event を落とす。
- timeout resolver を固定値にして coder timeout override を落とす。
- fallback response を正常系として作る。

## 10. 完了条件

Phase20 の完了条件は次の通り。

- `docs/refactor/Phase20_distributed_transport_executor境界整理実装仕様.md` が作成されている。
- 現在の transport execution 構造が棚卸しされている。
- `distributedTransportExecutor` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- route dispatcher / Code retry には踏み込まない方針が明記されている。
- コード変更は行っていない。
