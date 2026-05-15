# Phase4-2 proposal path 境界整理

## 目的

Phase4-2 は、CodeExecutor の proposal path を proposal generation、validation、Worker handoff、result formatting に分けて追える状態にする。

Coder は proposal を生成するだけで、実行は WorkerExecutionService が担当する。この責務境界を維持したまま、`tryExecuteProposalPath` の内部責務を小さく分ける。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `CoderAgentWithProposal`
- proposal validation
- WorkerExecutionService handoff
- `formatExecutionResult` との接続

## 対象外

- Coder selection。
- Generate path。
- WorkerExecutionService 内部。
- patch parser。
- ToolRunner / PolicyEngine。
- proposal / patch format の意味変更。
- Viewer JS / CSS、handler、DTO、SSE event、IdleChat、STT / TTS、runtime config。
- 未追跡の `tests/`。

## 現在の責務

`tryExecuteProposalPath` は現在、次の責務を同じ関数内で扱っている。

- selected Coder が `CoderAgentWithProposal` を満たすか確認する。
- `GenerateProposal` を呼ぶ。
- proposal generation error を event と error にする。
- nil / invalid proposal を event と error にする。
- valid proposal の plan event を emit する。
- Worker execution start event を emit する。
- `WorkerExecutionService.ExecuteProposal(ctx, req.Task.JobID(), proposal)` を呼ぶ。
- Worker error を event と wrapped error にする。
- `formatExecutionResult` で response を作る。
- `Handled: true` の response を返す。

## 提案する分離単位

- `proposalCoderForTarget(target codeTarget) (CoderAgentWithProposal, bool)`
  - selected Coder が proposal path に対応するかだけを見る。

- `generateProposalForTarget(ctx context.Context, req CodeExecutionRequest, target codeTarget, coder CoderAgentWithProposal) (*proposal.Proposal, error)`
  - Coder に proposal generation を依頼する。
  - generation error の event と error 契約を持つ。

- `validateGeneratedProposal(req CodeExecutionRequest, target codeTarget, p *proposal.Proposal) error`
  - nil / invalid proposal を Worker に渡さない境界を持つ。

- `emitProposalPlan(req CodeExecutionRequest, target codeTarget, p *proposal.Proposal)`
  - plan event のみを扱う。

- `executeProposalWithWorker(ctx context.Context, req CodeExecutionRequest, p *proposal.Proposal) (*patch.PatchExecutionResult, error)`
  - WorkerExecutionService への handoff だけを扱う。

- `emitProposalExecutionResult(req CodeExecutionRequest, formatted string)`
  - Worker result event を扱う。

## 入力

- `CodeExecutionRequest`
- `codeTarget`
- `CoderAgentWithProposal`
- `proposal.Proposal`

## 出力

- `CodeExecutionResponse`
- handled bool
- error
- `patch.PatchExecutionResult`

## 副作用

- Coder `GenerateProposal` 呼び出し。
- event emission。
- valid proposal の場合だけ WorkerExecutionService handoff。

## 永続化

CodeExecutor 自体は永続化しない。

WorkerExecutionService に渡った後の副作用は Phase3 の契約に従う。

## ログ

Phase4-2 では proposal path のログ意味を追加しない。既存の event と WorkerExecutionService 側のログを維持する。

## エラー契約

- selected Coder が proposal interface を持たない場合は handled false。
- proposal generation error は handled true + `%s proposal generation failed: %w`。
- nil / invalid proposal は handled true + `%s proposal generation failed: invalid proposal`。
- Worker execution error は handled true + `worker execution failed: %w`。
- error は fallback success にしない。

## 変更してはいけない既存挙動

- proposal interface を持たない Coder は Generate path に戻す。
- nil / invalid proposal は Worker に渡さない。
- Worker に渡す jobID は `req.Task.JobID()`。
- Worker に渡す proposal は Coder が生成した instance。
- plan event、Worker start event、Worker result / error event の内容。
- proposal path response の `Handled: true`。

## 実装手順

1. baseline test を実行する。
2. proposal path 内部を helper に分ける。
3. WorkerExecutionService 内部へ踏み込まない。
4. event 内容と error message を変えない。
5. `formatExecutionResult` の Markdown 構造を変えない。
6. gofmt を実行する。
7. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## リスク

- invalid proposal を Worker に渡してしまう。
- Coder が実行責務を持つように読める構造に戻る。
- proposal interface 非対応時に Generate path へ戻らなくなる。
- Worker handoff の jobID を `req.JobID` 文字列へ変えてしまう。
- `Handled` を success / failure と混同する。
- event 内容を変えて Viewer-facing 観測を壊す。

## 完了条件

- proposal generation、validation、Worker handoff、result formatting の境界が関数名で追える。
- invalid proposal が Worker に渡らない契約が維持されている。
- valid proposal の Worker handoff 契約が維持されている。
- 対象パッケージのテストが成功している。
- `git diff --check` が成功している。
