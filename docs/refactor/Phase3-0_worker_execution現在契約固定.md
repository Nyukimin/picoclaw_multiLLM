# Phase3-0 worker execution 現在契約固定

## 目的

Phase 3-0 では、Worker execution chain の現在契約を固定する。

Phase 3 の後続作業で WorkerExecutionService / ToolRunner / PolicyEngine の境界を整理する前に、Coder proposal から Worker execution へ渡る条件、Worker 実行結果、policy / tool 実行の既存契約を確認できる状態にする。

## 対象範囲

- `DefaultCodeExecutor` から `WorkerExecutionService.ExecuteProposal` への handoff。
- invalid proposal が WorkerExecutionService に到達しない契約。
- `WorkerExecutionService.ExecuteProposal` の parse / execute / result 契約。
- protected file、shell failure、git operation、parallel execution の既存契約。
- ToolRunner / PolicyEngine の既存 unit test。

## 対象外

- WorkerExecutionService の関数分割。
- ToolRunner の登録処理変更。
- PolicyEngine の policy 意味変更。
- Tool 実処理の変更。
- MessageOrchestrator の route chain 変更。
- Viewer JS / CSS。
- STT / TTS provider。
- LLM provider。

## 現在契約

### CodeExecutor から WorkerExecutionService

- CODE1 / CODE2 / CODE3 / CODE4 は proposal path を試す。
- 動的 CODE route が CODE3 品質へ選択された場合も proposal path を試す。
- Coder が `CoderAgentWithProposal` を実装していない場合、proposal path は使わず Generate path に進む。
- proposal が nil または invalid の場合、WorkerExecutionService に渡さず error を返す。
- valid proposal の場合だけ `WorkerExecutionService.ExecuteProposal(ctx, task.JobID(), p)` を呼ぶ。

### WorkerExecutionService

- 入力は `context.Context`、`task.JobID`、`*proposal.Proposal`。
- `p.Patch()` を `patch.ParsePatch` で `[]patch.PatchCommand` に変換する。
- parse error は `patch parse error` として error を返す。
- command failure は `PatchExecutionResult` に失敗として集約する。
- `StopOnError` が true の場合、失敗後の実行を止める。
- parallel execution は `file_edit -> shell_command -> git_operation` の phase order を守る。
- protected file は `ActionOnProtected` に従い error / skip / log を扱う。
- auto-commit は有効時のみ pre / post commit を行う。

### ToolRunner / PolicyEngine

- ToolRunner は tool request の実行 adapter であり、Coder proposal の patch 実行を扱わない。
- PolicyEngine は `execution.Action` から `execution.PolicyDecision` を返す。
- PolicyEngine は tool 実行の副作用を持たない。
- PolicyRunner は `RunnerV2` を policy 付きで包み、denied action を permission error response に変換する。

## 入力

- `CodeExecutionRequest`
- `task.Task`
- `proposal.Proposal`
- `config.WorkerConfig`
- `PatchCommand`
- `execution.Action`
- ToolRunner の tool name / args

## 出力

- `CodeExecutionResponse`
- `patch.PatchExecutionResult`
- `patch.CommandResult`
- `execution.PolicyDecision`
- `tool.ToolResponse`
- error

## 副作用

- WorkerExecutionService は file edit / shell / git operation を実行する。
- auto-commit が有効な場合、git repository に commit を作る。
- CodeExecutor は orchestrator event を emit する。
- ToolRunner は tool request に応じて file / shell / web / subagent / registry の副作用を持つ。
- PolicyEngine 単体は副作用を持たない。

## 永続化

- WorkerExecutionService は専用 DB 永続化を持たない。
- auto-commit 有効時のみ git repository に永続化する。
- PolicyRunner は execution repository に record を保存する。
- ToolRunner は web_search cache / ToolRegistry / file_write など adapter ごとの永続化を持つ。

## ログ

- CodeExecutor は handoff と coder selection を `log.Printf` で出す。
- WorkerExecutionService は execution summary、command result、auto-commit result を `fmt.Printf` で出す。
- ログは `PatchExecutionResult` の代替ではない。
- Viewer event、Worker log、execution report を混同しない。

## エラー契約

- invalid proposal は WorkerExecutionService に渡さない。
- patch parse error は `ExecuteProposal` の error として返す。
- command failure は `PatchExecutionResult` に失敗として記録する。
- protected file error は unsafe operation として分類される。
- missing command は retryable failure として分類される。
- policy deny は ToolResponse の permission error として返す。
- fallback は正常系 response として扱わない。

## 変更してはいけない既存挙動

- Coder が file edit / shell / git を直接実行しない。
- WorkerExecutionService が Coder selection を行わない。
- ToolRunner と WorkerExecutionService を統合しない。
- PolicyEngine が tool 実行副作用を持たない。
- protected file check を弱めない。
- StopOnError / ParallelExecution / AutoCommit の意味を変えない。

## 実装手順

1. baseline test を実行する。
2. 既存テストが固定している契約を確認する。
3. 不足している handoff 契約だけを最小テストで追加する。
4. 実装挙動は変更しない。
5. gofmt を実行する。
6. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/infrastructure/tools ./internal/infrastructure/security ./cmd/picoclaw
git diff --check
```

## リスク

- proposal path の条件を誤解し、CODE route すべてを Worker 実行に見なす。
- invalid proposal を Worker に渡してしまう。
- command failure と `ExecuteProposal` の error を混同する。
- WorkerExecutionService と ToolRunner の安全境界を統合してしまう。
- fallback を成功扱いする。

## 完了条件

- Phase 3-0 の現在契約が文書化されている。
- CodeExecutor から WorkerExecutionService への handoff 契約がテストで確認できる。
- invalid proposal が WorkerExecutionService に到達しない契約が維持されている。
- WorkerExecutionService / ToolRunner / PolicyEngine の既存テストが成功している。
- コード挙動は変更していない。
