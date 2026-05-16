# Phase5-1 selection helper ファイル分離

## 目的

Coder selection の責務を `code_executor_selection.go` へ移動し、`code_executor.go` から selection の詳細を分離する。

## 対象範囲

- `codeTarget`
- `selectCoderForRoute`
- `selectDynamicCoderForRoute`
- `selectExplicitCoderForRoute`
- `selectAvailableCoderForGenericRoute`
- `systemPromptForRoute`
- `coderByName`
- `explicitCodeRouteTarget`

## 対象外

- proposal path。
- Generate path。
- event helper。
- response helper。
- WorkerExecutionService 内部。
- 未追跡の `tests/`。

## 実装手順

1. `internal/application/orchestrator/code_executor_selection.go` を作成する。
2. selection 関連の型と関数を `code_executor.go` から移動する。
3. 関数本体、error message、log message、route 条件を変えない。
4. `code_executor.go` の不要 import を削除する。
5. gofmt を実行する。
6. 対象テストを実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/capability ./cmd/picoclaw
git diff --check
rg "selectCoderForRoute|selectDynamicCoderForRoute|selectExplicitCoderForRoute|selectAvailableCoderForGenericRoute|explicitCodeRouteTarget|systemPromptForRoute|coderByName|type codeTarget" internal/application/orchestrator
```

## 完了条件

- selection helper が `code_executor_selection.go` にある。
- `code_executor.go` から selection 詳細が消えている。
- CoderStatus release 契約テストが成功している。
- generic `CODE` fallback order が変わっていない。
