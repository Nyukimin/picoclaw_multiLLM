# Phase5-5 response helper ファイル分離

## 目的

`CodeExecutionResponse` と `Handled` 契約を `code_executor_response.go` へ移動し、response assembly の意味をファイル単位で独立させる。

## 対象範囲

- `CodeExecutionResponse`
- `buildProposalHandledResponse`
- `buildCoderGenerateResponse`

## 対象外

- `CodeExecutionRequest`
- Worker result formatting。
- event emission。
- success / failure 判定。
- 未追跡の `tests/`。

## 実装手順

1. `internal/application/orchestrator/code_executor_response.go` を作成する。
2. `CodeExecutionResponse` と response helper を移動する。
3. `CodeExecutionRequest` は `code_executor.go` に残す。
4. gofmt を実行する。
5. 対象テストを実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/patch ./cmd/picoclaw
git diff --check
rg "type CodeExecutionResponse|buildProposalHandledResponse|buildCoderGenerateResponse" internal/application/orchestrator
```

## 完了条件

- response contract が `code_executor_response.go` にある。
- `Handled` の意味が変わっていない。
- response helper が副作用を持っていない。
- `CodeExecutionRequest` は `code_executor.go` に残っている。
