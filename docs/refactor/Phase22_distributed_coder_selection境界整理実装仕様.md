# Phase22 distributed coder selection 境界整理実装仕様

## 1. Phase22 の目的

Phase22 は、`DistributedOrchestrator` に残っている `routeToCoder` / `routeToCoderForMessage` / `isCoderConnected` / node capability selection を `distributedCoderSelection` へ分離する段階である。

目的は次の通り。

- Coder 選択の責務を Code execution から分ける。
- explicit CODE1-4、CODE fallback chain、capability selection、connection 判定、既存ログを維持する。
- Phase21 Code execution coordinator は Coder selector callback に依存したままにする。
- Code execution、transport executor、node selector 実装本体には踏み込まない。

Phase22 は構造整理であり、Coder selection policy の仕様変更ではない。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/distributed_orchestrator.go`
  - `routeToCoder`
  - `routeToCoderForMessage`
  - `isCoderConnected`
  - `SetNodeCapabilities`
  - constructor
- 新規追加する `distributedCoderSelection`
- coder selection focused tests。

## 3. 対象外

Phase22 では次を対象外にする。

- Code execution coordinator の追加分割。
- transport executor 本体。
- `NodeSelector` 実装本体。
- `inferTaskRequirement` の仕様変更。
- route dispatcher の追加変更。
- event / evidence / TTS / session / autonomous 境界の追加変更。
- handler / DTO / SSE event / Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- 未追跡の `tests/`。

## 4. 現在の coder selection 構造

`routeToCoder` は route に応じて coder を選ぶ。

- CODE: coder1、coder2、coder3、coder4 の順に接続済み coder を探す。
- CODE1-4: 対応する coder が接続済みの場合だけ返す。
- 未接続の場合は skip log を出して空を返す。

`routeToCoderForMessage` は route が CODE で、node selector と capability map がある場合だけ capability selection を使う。

- 接続済み coder を candidates にする。
- `inferTaskRequirement(userMessage)` を使う。
- `nodeSelector.Select` が selected を返せばそれを使う。
- selected が空の場合は fallback chain に戻る。

`isCoderConnected` は SSH transport か router registered agent のどちらかがあれば接続済みとする。

## 5. 提案する collaborator

### `distributedCoderSelection`

`distributedCoderSelection` は private struct とする。初期段階では interface 化しない。

配置:

- `internal/application/orchestrator/distributed_orchestrator_coder_selection.go` に定義する。
- `DistributedOrchestrator` に field として持たせる。
- `NewDistributedOrchestrator` で組み立てる。

dependency:

- `transport.MessageRouter`
- `map[string]domaintransport.Transport`
- `NodeSelector`
- `map[string]domainnode.ResourceProfile`

setter 反映:

- `SetNodeCapabilities` 後は selector が最新 capability map を使う。

## 6. `distributedCoderSelection` の契約

入力:

- `routing.Route`
- user message
- node capabilities
- router / SSH transport state

出力:

- coder agent ID string

副作用:

- selection log。

永続化:

- なし。

ログ:

- coder selected。
- coder skip。
- capability select fell back。

エラー契約:

- coder が選べない場合は空 string を返す。
- fallback response を作らない。

変更してはいけない既存挙動:

- CODE fallback chain 順。
- CODE1-4 explicit route。
- SSH transport / router registered agent の接続判定。
- capability selection が CODE route のみに適用されること。
- capability selected が空の場合に fallback chain へ戻ること。

## 7. 実装手順

1. baseline test を実行する。
2. `distributedCoderSelection` を `distributed_orchestrator_coder_selection.go` に追加する。
3. `DistributedOrchestrator` に `coderSelection *distributedCoderSelection` field を追加する。
4. `NewDistributedOrchestrator` で selector を組み立てる。
5. `SetNodeCapabilities` で selector の capability map を更新する。
6. `routeToCoder` / `routeToCoderForMessage` / `isCoderConnected` を selector への委譲に置き換える。
7. 既存ログと selection policy を変えない。
8. gofmt を実行する。
9. focused test と全体 test を実行する。
10. `docs/refactor/Phase22_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

coder selection focused:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase22|TestDistributedOrchestrator_.*(CODE|Retry)|TestNodeSelector'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- CODE fallback chain の順序を変える。
- CODE1-4 の未接続時に別 coder へ fallback してしまう。
- capability selection を CODE1-4 にも適用してしまう。
- SSH transport 接続判定を落とす。
- router registered agent 接続判定を落とす。
- capability selected 空時の fallback chain を落とす。

## 10. 完了条件

Phase22 の完了条件は次の通り。

- `docs/refactor/Phase22_distributed_coder_selection境界整理実装仕様.md` が作成されている。
- 現在の coder selection 構造が棚卸しされている。
- `distributedCoderSelection` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Code execution / transport executor / node selector 本体には踏み込まない方針が明記されている。
- コード変更は行っていない。
