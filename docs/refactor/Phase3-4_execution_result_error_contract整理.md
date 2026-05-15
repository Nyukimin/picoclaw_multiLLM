# Phase3-4 execution result / error contract 整理

## 目的

Phase 3-4 では、Worker execution の result / error / log / event / report の違いを明確にする。

Worker command failure を success response にすり替えず、`PatchExecutionResult` と user-facing formatting の境界を読みやすくする。

## 対象範囲

- `internal/domain/patch/result.go`
- `internal/application/service/worker_execution_service.go`
- `internal/application/orchestrator/code_helpers.go`
- `formatExecutionResult`
- failure metadata

## 対象外

- Viewer 表示仕様。
- autonomous execution report schema。
- LLM provider retry。
- Coder proposal generation。
- WorkerExecutionService の file / shell / git 実行内容。
- ToolRunner / PolicyEngine の policy 意味。

## 現在の責務

- `PatchExecutionResult` は Worker 実行結果の domain value object。
- `CommandResult` は単一 command の実行結果。
- WorkerExecutionService は command failure を `PatchExecutionResult` に集約する。
- `formatExecutionResult` は proposal と result から user-facing Markdown を作る。
- CodeExecutor は Worker execution error と Worker result formatting を区別する。

## 提案する分離単位

- `executionStatusEmoji`
- `formatGitCommitLine`
- `formatCommandDetails`

`formatExecutionResult` の出力文言は維持し、formatting の内部だけを小さく分ける。

## 入力

- `*proposal.Proposal`
- `*patch.PatchExecutionResult`
- `patch.CommandResult`

## 出力

- user-facing Markdown response。
- failure metadata。

## 副作用

- formatting helper は副作用を持たない。

## 永続化

- なし。

## ログ

- formatting helper はログを出さない。
- Worker log と `PatchExecutionResult` を混同しない。

## エラー契約

- parse / pre-commit failure は `ExecuteProposal` の error。
- command failure は `PatchExecutionResult` の failed result。
- invalid proposal は Worker result ではなく proposal generation error。
- command failure を fallback success に変換しない。

## 変更してはいけない既存挙動

- `formatExecutionResult` の section 構造。
- success / warning emoji。
- git commit short hash 表示。
- command details の action / target / error 表示。
- success rate 計算。
- failure metadata の意味。

## 実装手順

1. baseline test を実行する。
2. `formatExecutionResult` の内部 formatting を小関数へ分ける。
3. 出力文言と順序を変えない。
4. formatting helper に副作用を入れない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

## リスク

- command failure を `ExecuteProposal` error と混同する。
- result formatting で failure を成功に見せる。
- Viewer event / report / log へ result の意味を移す。
- section heading を変えて既存テストや Viewer 表示期待を壊す。

## 完了条件

- result / error / log / event / report の違いが文書化されている。
- `formatExecutionResult` の内部が副作用なし helper へ分かれている。
- user-facing response の既存構造が維持されている。
- 対象テストが成功している。
