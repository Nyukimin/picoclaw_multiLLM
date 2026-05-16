# Phase5-3 Generate path ファイル分離

## 目的

Generate path を `code_executor_generate.go` へ移動し、proposal path とファイル単位で分ける。

## 対象範囲

- `executeCoderGeneratePath`
- `emitCoderGenerateError`
- `emitCoderGenerateResponse`

## 対象外

- proposal path。
- selection。
- common event emitter。
- response helper。
- WorkerExecutionService 内部。
- 未追跡の `tests/`。

## 実装手順

1. `internal/application/orchestrator/code_executor_generate.go` を作成する。
2. Generate path helper を移動する。
3. event type / from / to / content / route を変えない。
4. gofmt を実行する。
5. 対象テストを実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
rg "executeCoderGeneratePath|emitCoderGenerateError|emitCoderGenerateResponse" internal/application/orchestrator
```

## 完了条件

- Generate path helper が `code_executor_generate.go` にある。
- Generate error が error として返る。
- Generate path response が `Handled: false` のままである。
- `code_executor.go` から Generate path 詳細が消えている。
