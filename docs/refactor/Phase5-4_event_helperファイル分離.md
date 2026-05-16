# Phase5-4 event helper ファイル分離

## 目的

共通 event emission と ExecuteCode 直下の notice / start event を `code_executor_events.go` へ移動する。

## 対象範囲

- `emit`
- `SetEventEmitter`
- `emitDegradedRouteNotice`
- `emitCodeHandoffStart`

## 対象外

- proposal path 専用 event。
- Generate path 専用 event。
- Viewer JS / CSS。
- SSE event schema。
- WorkerExecutionService 内部。
- 未追跡の `tests/`。

## 実装手順

1. `internal/application/orchestrator/code_executor_events.go` を作成する。
2. event helper を移動する。
3. `fmt` / `log` import を移動先へ寄せる。
4. event content と log message を変えない。
5. gofmt を実行する。
6. 対象テストを実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
rg "SetEventEmitter|emitDegradedRouteNotice|emitCodeHandoffStart|func \\(e \\*DefaultCodeExecutor\\) emit" internal/application/orchestrator
```

## 完了条件

- event helper が `code_executor_events.go` にある。
- degraded route notice を fallback success として扱っていない。
- Viewer-facing event の type / from / to / route を変えていない。
- `code_executor.go` から event helper 詳細が消えている。
