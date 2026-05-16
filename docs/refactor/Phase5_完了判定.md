# Phase5 完了判定

## 対象

Phase5 は `CodeExecutor` 周辺のファイル分離を対象とした。

対象ファイル:

- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_executor_selection.go`
- `internal/application/orchestrator/code_executor_proposal.go`
- `internal/application/orchestrator/code_executor_generate.go`
- `internal/application/orchestrator/code_executor_events.go`
- `internal/application/orchestrator/code_executor_response.go`
- `internal/application/orchestrator/code_executor_test.go`
- `docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md`
- `docs/refactor/Phase5-0_CodeExecutor現在ファイル構成記録.md`
- `docs/refactor/Phase5-1_selection_helperファイル分離.md`
- `docs/refactor/Phase5-2_proposal_pathファイル分離.md`
- `docs/refactor/Phase5-3_Generate_pathファイル分離.md`
- `docs/refactor/Phase5-4_event_helperファイル分離.md`
- `docs/refactor/Phase5-5_response_helperファイル分離.md`

未追跡の `tests/` は Phase5 の対象外として触っていない。

## 実施した Phase

### Phase5-0: 現在ファイル構成と baseline test 記録

実施内容:

- Phase5 開始前の `code_executor.go` 関数配置を記録した。
- baseline test を実行した。
- コード変更は行っていない。

### Phase5-1: selection helper ファイル分離

実施内容:

- `codeTarget` を `code_executor_selection.go` へ移動した。
- `selectCoderForRoute` を `code_executor_selection.go` へ移動した。
- dynamic / explicit / generic CODE selection helper を `code_executor_selection.go` へ移動した。
- `explicitCodeRouteTarget`、`systemPromptForRoute`、`coderByName` を `code_executor_selection.go` へ移動した。

維持した契約:

- `coderCaps != nil` の場合は dynamic selection を優先する。
- `CODE1` / `CODE2` / `CODE3` / `CODE4` と Coder slot の対応を変えない。
- generic `CODE` の fallback order を変えない。
- CoderStatus acquire / release 条件を変えない。

### Phase5-2: proposal path ファイル分離

実施内容:

- `shouldUseProposalPath` を `code_executor_proposal.go` へ移動した。
- `tryExecuteProposalPath` と proposal path helper を `code_executor_proposal.go` へ移動した。
- proposal generation、validation、Worker handoff、proposal result event を同じ責務ファイルに集約した。

維持した契約:

- proposal interface 非対応 Coder は Generate path に戻す。
- nil / invalid proposal は Worker に渡さない。
- valid proposal だけ `WorkerExecutionService.ExecuteProposal(ctx, req.Task.JobID(), proposal)` に渡す。
- Worker execution error は `worker execution failed` で wrap する。

### Phase5-3: Generate path ファイル分離

実施内容:

- `executeCoderGeneratePath` を `code_executor_generate.go` へ移動した。
- `emitCoderGenerateError` / `emitCoderGenerateResponse` を `code_executor_generate.go` へ移動した。

維持した契約:

- Generate error は event を出した上で error として返す。
- Generate path は proposal validation や Worker handoff を持たない。
- Generate path response は `Handled: false` のままにする。

### Phase5-4: event helper ファイル分離

実施内容:

- `emit` を `code_executor_events.go` へ移動した。
- `SetEventEmitter` を `code_executor_events.go` へ移動した。
- `emitDegradedRouteNotice` / `emitCodeHandoffStart` を `code_executor_events.go` へ移動した。

維持した契約:

- degraded route notice を fallback success として扱っていない。
- event type / from / to / route / content の意味を変えていない。
- proposal path 専用 event と Generate path 専用 event は、それぞれの責務ファイルに残した。

### Phase5-5: response helper ファイル分離

実施内容:

- `CodeExecutionResponse` を `code_executor_response.go` へ移動した。
- `buildProposalHandledResponse` / `buildCoderGenerateResponse` を `code_executor_response.go` へ移動した。
- `CodeExecutionRequest` は `code_executor.go` に残した。

維持した契約:

- `Handled: true` は proposal path が処理したことを表す。
- `Handled: false` は Generate path を表す。
- `Handled` は success / failure の一般状態ではない。

## 最終ファイル構成

- `code_executor.go`
  - interface、request DTO、DefaultCodeExecutor、constructor、capability setup、`ExecuteCode`。
- `code_executor_selection.go`
  - Coder selection、route prompt、Coder slot lookup、`codeTarget`。
- `code_executor_proposal.go`
  - proposal path、proposal validation、Worker handoff、proposal result event。
- `code_executor_generate.go`
  - Generate path、Generate error / success event。
- `code_executor_events.go`
  - common event emitter、degraded route notice、handoff start event。
- `code_executor_response.go`
  - response contract、`Handled` response builder。

## 検証結果

実行した検証:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
rg "func \\(e \\*DefaultCodeExecutor\\) select|func shouldUseProposalPath|func \\(e \\*DefaultCodeExecutor\\) tryExecuteProposalPath|func \\(e \\*DefaultCodeExecutor\\) executeCoderGeneratePath|func \\(e \\*DefaultCodeExecutor\\) emit|func buildProposalHandledResponse|type CodeExecutionResponse|type CodeExecutionRequest" internal/application/orchestrator
```

結果:

- `internal/application/orchestrator`: ok
- `internal/application/service`: ok
- `internal/domain/capability`: ok
- `internal/domain/patch`: ok
- `cmd/picoclaw`: ok
- `git diff --check`: 成功
- `git status --short`: `?? tests/` のみ

関数配置確認:

- `CodeExecutionRequest`: `code_executor.go`
- `CodeExecutionResponse`: `code_executor_response.go`
- selection helper: `code_executor_selection.go`
- proposal path helper: `code_executor_proposal.go`
- Generate path helper: `code_executor_generate.go`
- event helper: `code_executor_events.go`
- response helper: `code_executor_response.go`

## Push 済み commit

- `e18d3eb docs: Phase5-0 CodeExecutor現在構成を記録`
- `84dc88e docs: Phase5-1 selection helper分離手順を追加`
- `0e86661 refactor: CodeExecutor selection helperを分離`
- `dce7fcf docs: Phase5-2 proposal path分離手順を追加`
- `7644095 refactor: CodeExecutor proposal pathを分離`
- `d88cf5c docs: Phase5-3 Generate path分離手順を追加`
- `27251a4 refactor: CodeExecutor Generate pathを分離`
- `d97edcc docs: Phase5-4 event helper分離手順を追加`
- `ee32f80 refactor: CodeExecutor event helperを分離`
- `099f8ca docs: Phase5-5 response helper分離手順を追加`
- `75893e1 refactor: CodeExecutor response contractを分離`

## 完了判定

Phase5 は完了と判定する。

理由:

- `code_executor.go` が入口、依存注入、`ExecuteCode` orchestration に絞られている。
- selection / proposal / Generate / event / response helper が責務別ファイルへ移動している。
- Coder は proposal / Generate を担当し、実行は WorkerExecutionService が担当する境界を維持している。
- invalid proposal が Worker に渡らない契約を維持している。
- degraded route notice を fallback success として扱っていない。
- `Handled` を success / failure と混同しない構造を維持している。
- 対象パッケージのテストが成功している。
- Phase5 の文書と実装差分は push 済みである。

## Phase6 に進む前の確認事項

Phase6 に進む場合、候補は次のどちらかを先に決める。

- Phase2 / Phase3 / Phase4 / Phase5 の境界を踏まえ、Chat / Worker / Coder route chain 全体の統合確認を行う。
- `MessageOrchestrator` 側に残る大きな責務を、route dispatch / response assembly / event report の単位でさらに分ける。

どちらを選ぶ場合でも、仕様変更と構造変更を混ぜない。
