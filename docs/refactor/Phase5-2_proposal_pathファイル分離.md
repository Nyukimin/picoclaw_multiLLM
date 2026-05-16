# Phase5-2 proposal path ファイル分離

## 目的

proposal path と Worker handoff 境界を `code_executor_proposal.go` へ移動し、`code_executor.go` から proposal path の詳細を分離する。

## 対象範囲

- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `proposalCoderForTarget`
- `generateProposalForTarget`
- `validateGeneratedProposal`
- `emitProposalPlan`
- `executeProposalWithWorker`
- `emitProposalExecutionResult`

## 対象外

- WorkerExecutionService 内部。
- patch parser。
- selection。
- Generate path。
- event helper。
- response helper。
- 未追跡の `tests/`。

## 実装手順

1. `internal/application/orchestrator/code_executor_proposal.go` を作成する。
2. proposal path helper を移動する。
3. `proposal` / `patch` import を移動先へ寄せる。
4. error message、event content、Worker handoff 条件を変えない。
5. gofmt を実行する。
6. 対象テストを実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/patch ./cmd/picoclaw
git diff --check
rg "shouldUseProposalPath|tryExecuteProposalPath|proposalCoderForTarget|generateProposalForTarget|validateGeneratedProposal|executeProposalWithWorker" internal/application/orchestrator
```

## 完了条件

- proposal path helper が `code_executor_proposal.go` にある。
- invalid proposal が Worker に渡らない契約が維持されている。
- valid proposal の Worker handoff 契約が維持されている。
- `code_executor.go` から proposal path 詳細が消えている。
