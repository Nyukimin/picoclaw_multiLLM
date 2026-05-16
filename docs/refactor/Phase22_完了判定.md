# Phase22 完了判定

## Phase の目的

Phase22 は `DistributedOrchestrator` に残っていた `routeToCoder` / `routeToCoderForMessage` / `isCoderConnected` / node capability selection を、分散 Coder 選択専用の `distributedCoderSelection` へ分離する。

目的は構造整理であり、Code execution、transport executor、node selector 実装本体、event / evidence、TTS lifecycle、session lifecycle、autonomous coordinator、provider、Viewer、IdleChat、runtime config の挙動は変更しない。

## 実装した境界

- `distributedCoderSelection`
  - 入力: route、user message、node capabilities、router / SSH transport state
  - 出力: coder agent ID string
  - 副作用: selection log
  - 永続化: なし
  - エラー契約: coder が選べない場合は空 string を返し、fallback response を作らない

## 維持した既存挙動

- CODE は coder1-4 の接続済み fallback chain を使う。
- CODE1-4 は明示 coder が接続済みの場合だけ選ぶ。
- CODE route かつ node selector / capability がある場合だけ capability selection を使う。
- selected が空の場合は fallback chain に戻す。
- SSH transport または router registered agent を接続済みとする。
- coder selected / skip / capability fallback のログを維持する。

## 変更ファイル

- `internal/application/orchestrator/distributed_orchestrator.go`
- `internal/application/orchestrator/distributed_orchestrator_coder_selection.go`
- `internal/application/orchestrator/distributed_orchestrator_phase22_coder_selection_test.go`
- `docs/refactor/Phase22_完了判定.md`

## 検証

Phase22 の最終確認では次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase22|TestDistributedOrchestrator_.*(CODE|Retry)|TestNodeSelector'
GOCACHE=/tmp/picoclaw-gocache go test -count=1 ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## 完了条件

- distributed coder selection の詳細が `distributedCoderSelection` へ分離されている。
- `DistributedOrchestrator` 本体は coder selector の構築と委譲だけを持つ。
- CODE fallback chain、explicit route no-fallback、SSH transport connection の主要契約がテストで固定されている。
- Phase22 の検証コマンドが成功している。
- Phase22 の文書と実装差分が Push 済みである。

## 次の候補

次は Phase23 として、`DistributedOrchestrator` の attribution guard 境界整理に進む候補がある。CentralMemory unified view、IdleChat message exclusion、発言帰属ガード文面を固定してから分離する。
