# Phase23 distributed attribution guard 境界整理実装仕様

## 1. Phase23 の目的

Phase23 は、`DistributedOrchestrator` に残っている `withAttributionGuard` / `buildAttributionGuardedMessage` を `distributedAttributionGuard` へ分離する段階である。

目的は次の通り。

- attribution guard 生成を route dispatcher から分ける。
- CentralMemory unified view、IdleChat 除外、既存 guard 文面、task metadata 継承を維持する。
- Phase19 route dispatcher は attribution guard callback へ依存したままにする。
- route dispatcher、transport executor、Code execution には踏み込まない。

Phase23 は構造整理であり、prompt / guard 文面の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `withAttributionGuard`
  - `buildAttributionGuardedMessage`
  - constructor
- 新規追加する `distributedAttributionGuard`
- attribution focused tests。

## 3. 対象外

Phase23 では次を対象外にする。

- guard 文面の仕様変更。
- CentralMemory 実装変更。
- route dispatcher の追加分割。
- transport executor 本体。
- Code execution。
- event / evidence / TTS / session / autonomous / coder selection 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat 本体。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の attribution guard 構造

`withAttributionGuard` は次の場合に task を変更せず返す。

- targetAgent が空。
- task route が Code route。
- user message に `【発言帰属ガード】` が既に含まれる。

guard が必要な場合、`buildAttributionGuardedMessage` で `CentralMemory.GetUnifiedView(120)` を読み、同じ session ID の会話だけを使う。

除外条件:

- session ID が違う。
- content が空。
- `domaintransport.MessageTypeIdleChat`。
- session ID が `idle-` prefix。

selfLines / otherLines は末尾から最大 3 件ずつ集め、空の場合は `なし` を入れる。

guard 文面:

```text
【発言帰属ガード】
あなたは <targetAgent>。
自分の過去発言: <selfLines>
他者の発言: <otherLines>
要件: 他者の発言や既出案を自分の新規アイデアとして言い換えない。参照時は発言者を明示する。

【ユーザー依頼】
<userMessage>
```

## 5. 提案する collaborator

### `distributedAttributionGuard`

`distributedAttributionGuard` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_attribution.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `session.CentralMemory`

## 6. `distributedAttributionGuard` の契約

入力:

- `task.Task`
- target agent
- session ID

出力:

- `task.Task`

副作用:

- なし。CentralMemory は読み取りだけ。

永続化:

- なし。

ログ:

- なし。

エラー契約:

- error は返さない。
- guard に使える memory がない場合は元 task を返す。

変更してはいけない既存挙動:

- skip 条件。
- `GetUnifiedView(120)`。
- IdleChat 除外条件。
- self / other 最大 3 件。
- `truncateForNote(..., 90)`。
- guard 文面。
- JobID / Channel / ChatID / ForcedRoute / Route の継承。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedAttributionGuard` を `distributed_orchestrator_attribution.go` に追加する。
3. `DistributedOrchestrator` に `attribution *distributedAttributionGuard` field を追加する。
4. `NewDistributedOrchestrator` で guard を組み立てる。
5. `withAttributionGuard` / `buildAttributionGuardedMessage` を guard への委譲に置き換える。
6. 既存 guard 文面と除外条件を変えない。
7. gofmt を実行する。
8. focused test と全体 test を実行する。
9. `docs/refactor/Phase23_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

attribution focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase23|TestDistributedOrchestrator_AttributionGuardOnUserChat'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- Code route に guard を入れてしまう。
- IdleChat message を guard に混ぜる。
- session ID が違う memory を混ぜる。
- guard 文面を変える。
- task metadata を落とす。
- attribution guard を prompt fallback と混同する。

## 10. 完了条件

Phase23 の完了条件は次の通り。

- `docs/refactor/Phase23_distributed_attribution_guard境界整理実装仕様.md` が作成されている。
- 現在の attribution guard 構造が棚卸しされている。
- `distributedAttributionGuard` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- route dispatcher / transport executor / Code execution には踏み込まない方針が明記されている。
- コード変更は行っていない。
