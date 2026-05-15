# Phase4-4 CodeExecutionResponse 契約整理

## 目的

Phase4-4 は、`CodeExecutionResponse` と `Handled` の意味をコード上でも追いやすくする。

`Handled` は成功可否ではなく、proposal path が処理したかどうかを表す。proposal path と Generate path の response assembly を helper 化し、この意味を明確にする。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `CodeExecutionResponse`
- `tryExecuteProposalPath`
- `executeCoderGeneratePath`
- `formatExecutionResult` との接続

## 対象外

- `formatExecutionResult` の Markdown 構造変更。
- WorkerExecutionService result 内部。
- proposal / patch format。
- Viewer display rendering。
- Coder selection。
- proposal path の handoff 条件。
- Generate path の event 内容。
- 未追跡の `tests/`。

## 現在の責務

現在は response assembly が各 path の return 位置に直接書かれている。

- proposal path success は `CodeExecutionResponse{Response: formatted, Handled: true}` を返す。
- Generate path success は `CodeExecutionResponse{Response: resp, Handled: false}` を返す。
- error の場合は empty response と error を返す。

## 提案する分離単位

- `buildProposalHandledResponse(response string) CodeExecutionResponse`
  - proposal path が処理した response を作る。
  - `Handled: true` を明示する。

- `buildCoderGenerateResponse(response string) CodeExecutionResponse`
  - Generate path の response を作る。
  - `Handled: false` を明示する。

## 入力

- proposal execution formatted response。
- generated response。

## 出力

- `CodeExecutionResponse`

## 副作用

なし。

response assembly helper は event、log、Worker handoff、永続化を行わない。

## 永続化

なし。

## ログ

なし。

## エラー契約

- `Handled` は proposal path 処理有無であり、成功可否ではない。
- error は error として返し、fallback success にしない。
- response helper は error を握りつぶさない。

## 変更してはいけない既存挙動

- proposal path success は `Handled: true`。
- Generate path success は `Handled: false`。
- `formatExecutionResult` の出力構造。
- error 時の return 契約。

## 実装手順

1. baseline test を実行する。
2. response assembly helper を追加する。
3. proposal path と Generate path の return を helper 経由にする。
4. `Handled` の意味を変えない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## リスク

- `Handled` を success / failure と混同する。
- error path に response helper を使って error を隠す。
- proposal path と Generate path の return を逆にする。
- result Markdown の構造を変える。

## 完了条件

- `Handled` の意味が helper 名で追える。
- response assembly helper が副作用を持たない。
- 対象パッケージのテストが成功している。
- `git diff --check` が成功している。
