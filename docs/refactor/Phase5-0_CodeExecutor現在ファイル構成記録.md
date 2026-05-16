# Phase5-0 CodeExecutor 現在ファイル構成記録

## 目的

Phase5-0 は、CodeExecutor ファイル分離を始める前の現在状態を固定する段階である。

この段階では実装変更を行わない。baseline test と現在の関数配置を記録し、以降の Phase5-1 以降で「挙動変更なしのファイル移動」だけを行っているか確認できる状態にする。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/orchestrator/*code*_test.go`
- `docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md`

## 対象外

- 関数移動。
- import 整理。
- selection / proposal / Generate / event / response の挙動変更。
- `MessageOrchestrator`。
- `WorkerExecutionService` 内部。
- 未追跡の `tests/`。

## baseline test

実行コマンド:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
```

結果:

- `internal/application/orchestrator`: ok
- `internal/application/service`: ok
- `internal/domain/capability`: ok
- `internal/domain/patch`: ok
- `cmd/picoclaw`: ok

## 現在の関数配置

確認コマンド:

```bash
rg "^(type|func) |^func \(e \*DefaultCodeExecutor\)" internal/application/orchestrator/code_executor.go
```

現在 `code_executor.go` にある定義:

- `type CodeExecutor interface`
- `type CodeExecutionRequest struct`
- `type CodeExecutionResponse struct`
- `type codeTarget struct`
- `type DefaultCodeExecutor struct`
- `NewDefaultCodeExecutor`
- `WithCapabilities`
- `ExecuteCode`
- `shouldUseProposalPath`
- `selectCoderForRoute`
- `selectDynamicCoderForRoute`
- `selectExplicitCoderForRoute`
- `selectAvailableCoderForGenericRoute`
- `systemPromptForRoute`
- `coderByName`
- `tryExecuteProposalPath`
- `proposalCoderForTarget`
- `generateProposalForTarget`
- `validateGeneratedProposal`
- `emitProposalPlan`
- `executeProposalWithWorker`
- `emitProposalExecutionResult`
- `emitDegradedRouteNotice`
- `emitCodeHandoffStart`
- `executeCoderGeneratePath`
- `buildProposalHandledResponse`
- `buildCoderGenerateResponse`
- `emitCoderGenerateError`
- `emitCoderGenerateResponse`
- `emit`
- `SetEventEmitter`
- `explicitCodeRouteTarget`

## 現在の責務集中

`code_executor.go` は現在、次の責務を同時に持っている。

- CODE 系 route の入口。
- request / response contract。
- `DefaultCodeExecutor` の依存保持。
- Coder selection。
- proposal path。
- Generate path。
- event emission。
- response assembly。

Phase5-1 以降では、このうち詳細 helper を責務別ファイルへ移動する。

## 完了条件

- baseline test が成功している。
- 現在の関数配置が記録されている。
- コード変更を行っていない。
- 未追跡の `tests/` に触れていない。
