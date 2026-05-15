# Phase3 完了判定

## 対象

Phase 3 は Worker execution chain の責務境界整理を対象とする。

対象は `CodeExecutor`、`WorkerExecutionService`、`ToolRunner`、`PolicyEngine`、`PatchExecutionResult`、result formatting、関連する契約テストである。

## 実施した Phase

- Phase 3 実装仕様作成。
- Phase 3-0: Worker execution 現在契約固定。
- Phase 3-1: WorkerExecutionService の入力・出力・副作用整理。
- Phase 3-2: ToolRunner 境界整理。
- Phase 3-3: PolicyEngine 境界整理。
- Phase 3-4: execution result / error contract 整理。
- Phase 3-5: 完了判定。

## 作成した文書

- `docs/refactor/Phase3_worker_execution境界整理実装仕様プロンプト.md`
- `docs/refactor/Phase3_worker_execution境界整理実装仕様.md`
- `docs/refactor/Phase3-0_worker_execution現在契約固定.md`
- `docs/refactor/Phase3-1_WorkerExecutionService入出力副作用整理.md`
- `docs/refactor/Phase3-2_ToolRunner境界整理.md`
- `docs/refactor/Phase3-3_PolicyEngine境界整理.md`
- `docs/refactor/Phase3-4_execution_result_error_contract整理.md`
- `docs/refactor/Phase3_完了判定.md`

## 実装で変更したファイル

- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/service/worker_execution_service.go`
- `internal/infrastructure/tools/runner.go`
- `internal/infrastructure/security/policy_engine.go`
- `internal/application/orchestrator/code_helpers.go`

## 完了状態

### CodeExecutor / Worker handoff

`DefaultCodeExecutor` は valid proposal の場合だけ `WorkerExecutionService.ExecuteProposal` に渡す。Phase 3-0 では、Worker に渡された `jobID` と proposal instance を直接確認する契約テストを追加した。

invalid proposal が WorkerExecutionService に到達しない契約は Phase 2 から維持している。

### WorkerExecutionService

`ExecuteProposal` は次の手順として読める状態になった。

1. `parseProposalCommands`
2. `showExecutionSummaryIfEnabled`
3. `autoCommitBeforeExecution`
4. `executeCommands`
5. `autoCommitAfterExecution`
6. `finalizeExecutionResult`

file edit / shell / git operation の実行内容、protected file check、StopOnError、ParallelExecution、AutoCommit の意味は変更していない。

### ToolRunner

ToolRunner の registration は次の境界へ分離した。

- `registerCoreTools`
- `registerOptionalTools`
- `registerToolMetadata`
- `registerSubagentTool`
- `registerToolRegistryTool`

ToolRunner は tool request execution adapter の責務に留め、Worker patch execution へ統合していない。

### PolicyEngine

PolicyEngine の判定は次の境界へ分離した。

- `evaluateShellPolicy`
- `evaluateWorkspacePolicy`
- `evaluateNetworkPolicy`
- `evaluateNetworkAllowlistPolicy`
- `allowDecision`

PolicyEngine は action -> decision の純粋な判定境界として維持し、tool 実行副作用は持たせていない。

### execution result / error contract

`formatExecutionResult` は次の副作用なし helper に分離した。

- `executionStatusEmoji`
- `formatGitCommitLine`
- `formatCommandDetails`

`## Plan`、`## Execution Result`、`## Risk` の user-facing response 構造は維持している。

## 守られている境界

- Coder は proposal を生成する。
- WorkerExecutionService は proposal の patch を実行する。
- Coder が file edit / shell / git を直接実行しない。
- WorkerExecutionService は Coder selection をしない。
- ToolRunner は Worker patch execution を扱わない。
- PolicyEngine は tool 実行副作用を持たない。
- command failure は `PatchExecutionResult` に記録する。
- fallback を正常系として扱わない。
- Viewer event、Worker log、execution result、execution report を混同しない。

## 検証

Phase 3 の完了確認では次を実行する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/infrastructure/tools ./internal/infrastructure/security ./internal/domain/patch ./internal/domain/execution ./cmd/picoclaw
git diff --check
git status --short
```

`git status --short` では、今回の対象外である未追跡 `tests/` を除き、未コミット差分がないことを確認する。

## 完了条件の判定

- WorkerExecutionService / ToolRunner / PolicyEngine の責務差が docs/refactor に記録されている。
- CodeExecutor から WorkerExecutionService への handoff 契約がテストで確認できる。
- WorkerExecutionService の入力、出力、副作用、エラー契約が関数名で追える。
- ToolRunner の registration 境界が関数名で追える。
- PolicyEngine の判定境界が関数名で追える。
- execution result formatting が副作用なし helper へ分かれている。
- WorkerExecutionService、ToolRunner、PolicyEngine を巨大 service / helper に統合していない。
- Phase 2 の route chain 契約を変更していない。
- Viewer JS / CSS、STT / TTS provider、LLM provider、IdleChat 契約を変更していない。
- 対象パッケージのテストが成功している。
- Phase 3 の文書と実装差分は commit / push 済みである。

上記を満たすため、Phase 3 は完了と判定する。

## Phase 4 前の確認事項

Phase 4 に進む前に、次の対象をどれにするか決める。

- CodeExecutor と Coder selection の責務整理。
- Viewer adapter 側の event 表示契約整理。
- Memory / Source Registry の adapter 境界整理。
- WorkerExecutionService の file / shell / git executor adapter 化。
