# Phase3-1 WorkerExecutionService 入出力副作用整理

## 目的

Phase 3-1 では、`WorkerExecutionService.ExecuteProposal` の入力、出力、副作用、エラー契約を関数名で追える状態にする。

WorkerExecutionService は Coder proposal の patch 実行主体であり、Coder selection、ToolRunner、PolicyEngine の責務を持たない。

## 対象範囲

- `internal/application/service/worker_execution_service.go`
- `ExecuteProposal`
- patch parse
- execution summary
- pre / post auto-commit
- sequential / parallel execution selection
- result summary / failure classification

## 対象外

- patch format の変更。
- file edit / shell / git operation の挙動変更。
- ToolRunner との統合。
- PolicyEngine の policy 意味変更。
- CodeExecutor の route / Coder selection 変更。
- Viewer JS / CSS。

## 現在の責務

`ExecuteProposal` は現在、次の手順を 1 関数内で行っている。

1. `p.Patch()` を `patch.ParsePatch` で parse する。
2. 必要なら実行前 summary を表示する。
3. 必要なら pre auto-commit を行う。
4. sequential / parallel execution を選ぶ。
5. 必要なら post auto-commit を行う。
6. result summary と failure metadata を付与する。
7. result を返す。

## 提案する分離単位

- `parseProposalCommands`
- `showExecutionSummaryIfEnabled`
- `autoCommitBeforeExecution`
- `executeCommands`
- `autoCommitAfterExecution`
- `finalizeExecutionResult`

## 入力

- `context.Context`
- `task.JobID`
- `*proposal.Proposal`
- `config.WorkerConfig`
- `[]patch.PatchCommand`
- `*patch.PatchExecutionResult`

## 出力

- `[]patch.PatchCommand`
- `*patch.PatchExecutionResult`
- git commit hash
- error

## 副作用

- file edit。
- shell command。
- git operation。
- auto-commit。
- stdout への Worker log。

## 永続化

- filesystem。
- git repository。

## ログ

- 実行前 summary。
- pre / post auto-commit result。
- sequential / parallel phase progress。
- command success / failure。
- final execution summary。

## エラー契約

- patch parse error は `patch parse error: ...` を維持する。
- pre auto-commit failure は `pre-execution auto-commit failed: ...` を維持する。
- command failure は `PatchExecutionResult` に集約し、`ExecuteProposal` の error にはしない。
- post auto-commit failure は log のみで result error にはしない。
- failure metadata は最初の failed command から設定する。

## 変更してはいけない既存挙動

- `StopOnError` の意味。
- `ParallelExecution` の phase order。
- `MaxParallelism <= 0` の default 4。
- protected file check。
- workspace 外書き込み拒否。
- auto-commit の pre / post timing。
- `PatchExecutionResult` の summary 文言。

## 実装手順

1. baseline test を実行する。
2. `ExecuteProposal` の処理を小関数へ移す。
3. file / shell / git の dispatch は変更しない。
4. error wrapping の文言を変えない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/service ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

## リスク

- command failure を error return に変えてしまう。
- post auto-commit failure を失敗扱いに変えてしまう。
- summary / failure classification の順序を変えてしまう。
- parallel execution の phase order を崩す。
- WorkerExecutionService が ToolRunner / PolicyEngine の責務を持つ。

## 完了条件

- `ExecuteProposal` が Worker execution の composition root として読める。
- parse、commit、execution、finalize の境界が関数名で追える。
- WorkerExecutionService の外部挙動が変わっていない。
- 対象テストが成功している。
