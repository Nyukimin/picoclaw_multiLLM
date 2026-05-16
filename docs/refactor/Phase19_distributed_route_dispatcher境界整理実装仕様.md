# Phase19 distributed route dispatcher 境界整理実装仕様

## 1. Phase19 の目的

Phase19 は、`DistributedOrchestrator` に残っている `executeDistributed` / `executeDistributedDirect` の route 分岐を `distributedRouteDispatcher` へ分離する段階である。

目的は次の通り。

- route dispatch の責務を `DistributedOrchestrator` 本体から分ける。
- remote / local agent dispatch の接続点を明確にする。
- Phase15 event / evidence、Phase16 TTS lifecycle、Phase17 session lifecycle、Phase18 autonomous coordinator 境界を維持する。
- transport executor、Code route retry、node selection、attribution guard 本体には踏み込まない。

Phase19 は構造整理であり、route 実行仕様の変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `executeDistributed`
  - `executeDistributedDirect`
  - constructor
  - `SetWildAgent`
  - `SetHeavyAgent`
- 新規追加する `distributedRouteDispatcher`
- route dispatch focused tests。

## 3. 対象外

Phase19 では次を対象外にする。

- transport executor 本体。
- mailbox / SSH / local router 実行。
- Code route retry 本体。
- node selection。
- coder config。
- attribution guard 本体。
- autonomous executor 本体。
- event / evidence / TTS / session 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の route dispatch 構造

`executeDistributed` は route が CHAT 以外の場合に `executeAutonomousDistributed` へ入り、CHAT の場合は `executeDistributedDirect` へ入る。

`executeDistributedDirect` は次の分岐を持つ。

- CODE route: `executeCodeViaShiro` へ委譲し、成功時に user response event / note / TTS push を行う。
- WILD route with wild agent: stream hook、`wild.Generate`、response event、TTS finalize。
- ANALYZE route with heavy agent: stream hook、`heavy.Generate`、response event、TTS finalize。
- local route: attribution guard、CentralMemory task record、Mio.Chat、CentralMemory result record、event / note、TTS finalize。
- remote agent route: attribution guard、transport message context、CentralMemory record、`executeToAgent`、response event / note、TTS push。

## 5. 提案する collaborator

### `distributedRouteDispatcher`

`distributedRouteDispatcher` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_routes.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `MioAgent`
- `WildAgent`
- `HeavyAgent`
- `session.CentralMemory`
- event emitter
- note emitter
- stream hook
- TTS pusher
- autonomous executor
- code executor callback
- route-to-agent callback
- attribution guard callback
- transport executor callback

setter 反映:

- `SetWildAgent` 後は dispatcher が最新 wild agent を使う。
- `SetHeavyAgent` 後は dispatcher が最新 heavy agent を使う。
- Phase18 の autonomous coordinator は direct executor として dispatcher の direct execution を使う。

## 6. `distributedRouteDispatcher` の契約

入力:

- `context.Context`
- `task.Task`
- `routing.Route`
- session ID
- TTS session ID

出力:

- response string
- error

副作用:

- agent 実行。
- CentralMemory record。
- event / note emission。
- TTS / VTuber stream hook と push。
- transport executor callback 呼び出し。

永続化:

- 直接の DB 永続化はしない。
- CentralMemory への distributed message record は維持する。

ログ:

- 既存の route dispatch 本体では新規ログを追加しない。
- downstream の transport / Code route / autonomous log を維持する。

エラー契約:

- downstream agent / transport error をそのまま返す。
- fallback response を正常系として作らない。
- unsupported route を勝手に CHAT 扱いしない。

変更してはいけない既存挙動:

- CHAT は autonomous coordinator を通さず direct execution に入る。
- 非 CHAT は autonomous coordinator を通る。
- CODE route 成功時の user response event / note / TTS push。
- WILD / ANALYZE heavy の stream finalize。
- local route の attribution guard と CentralMemory task/result record。
- remote route の message context `route` / `channel` / `chat_id`。
- remote route の response event / note / TTS push。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedRouteDispatcher` を `distributed_orchestrator_routes.go` に追加する。
3. `DistributedOrchestrator` に `routeDispatcher *distributedRouteDispatcher` field を追加する。
4. `NewDistributedOrchestrator` で dispatcher を組み立てる。
5. `SetWildAgent` / `SetHeavyAgent` で dispatcher に最新 agent を反映する。
6. `executeDistributed` / `executeDistributedDirect` を dispatcher への委譲に置き換える。
7. Phase18 autonomous coordinator の direct executor が dispatcher の direct execution を使うようにする。
8. transport executor / Code retry / attribution guard 本体は動かさない。
9. gofmt を実行する。
10. focused test と全体 test を実行する。
11. `docs/refactor/Phase19_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

route focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase19|TestDistributedOrchestrator_.*(LocalRoute|CODE|OPS|Retry|TTSBridge)'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- CHAT を autonomous coordinator に通して event order を変える。
- remote route の message context を落とす。
- CentralMemory record を落とす。
- WILD / ANALYZE stream finalize を落とす。
- CODE route retry や transport executor まで同時に触って Phase19 の範囲を超える。
- unsupported route を fallback success にする。

## 10. 完了条件

Phase19 の完了条件は次の通り。

- `docs/refactor/Phase19_distributed_route_dispatcher境界整理実装仕様.md` が作成されている。
- 現在の distributed route dispatch 構造が棚卸しされている。
- `distributedRouteDispatcher` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- transport executor / Code retry には踏み込まない方針が明記されている。
- コード変更は行っていない。
