# Phase4 完了判定

## 対象

Phase4 は `CodeExecutor` の境界整理を対象とした。

対象ファイル:

- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_executor_test.go`
- `docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md`
- `docs/refactor/Phase4-0_CodeExecutor現在契約固定.md`
- `docs/refactor/Phase4-1_Coder_selection境界整理.md`
- `docs/refactor/Phase4-2_proposal_path境界整理.md`
- `docs/refactor/Phase4-3_Generate_path_event境界整理.md`
- `docs/refactor/Phase4-4_CodeExecutionResponse契約整理.md`

未追跡の `tests/` は Phase4 の対象外として触っていない。

## 実施した Phase

### Phase4-0: CodeExecutor 現在契約固定

実施内容:

- 現在の selection / proposal path / Generate path / event / `Handled` 契約を文書化した。
- `CoderStatus` release が success / error の両方で行われることをテストで固定した。
- degraded route notice を proposal handled success と混同しないことをテストで固定した。

### Phase4-1: Coder selection 境界整理

実施内容:

- `selectCoderForRoute` を dispatcher として薄くした。
- dynamic selection を `selectDynamicCoderForRoute` へ分離した。
- explicit route selection を `selectExplicitCoderForRoute` へ分離した。
- generic `CODE` fallback chain を `selectAvailableCoderForGenericRoute` へ分離した。

維持した契約:

- `coderCaps != nil` の場合は dynamic selection を優先する。
- `CODE1` / `CODE2` / `CODE3` / `CODE4` の Coder slot 対応を変えない。
- generic `CODE` の fallback order を変えない。
- `CoderStatus` acquire / release 条件を変えない。

### Phase4-2: proposal path 境界整理

実施内容:

- proposal interface 判定を `proposalCoderForTarget` へ分離した。
- proposal generation を `generateProposalForTarget` へ分離した。
- nil / invalid proposal validation を `validateGeneratedProposal` へ分離した。
- plan event を `emitProposalPlan` へ分離した。
- Worker handoff を `executeProposalWithWorker` へ分離した。
- Worker result event を `emitProposalExecutionResult` へ分離した。

維持した契約:

- proposal interface 非対応 Coder は Generate path に戻す。
- nil / invalid proposal は Worker に渡さない。
- valid proposal だけ `WorkerExecutionService.ExecuteProposal(ctx, req.Task.JobID(), proposal)` に渡す。
- Worker execution error は `worker execution failed` で wrap する。

### Phase4-3: Generate path / event 境界整理

実施内容:

- degraded route notice を `emitDegradedRouteNotice` へ分離した。
- handoff start event を `emitCodeHandoffStart` へ分離した。
- Generate error event を `emitCoderGenerateError` へ分離した。
- Generate success event を `emitCoderGenerateResponse` へ分離した。

維持した契約:

- degraded route notice は fallback success ではない。
- event type / from / to / route / content の意味を変えない。
- Generate error は error として返す。
- Generate path response は `Handled: false` のままにする。

### Phase4-4: CodeExecutionResponse 契約整理

実施内容:

- proposal path response を `buildProposalHandledResponse` へ分離した。
- Generate path response を `buildCoderGenerateResponse` へ分離した。

維持した契約:

- `Handled: true` は proposal path が処理したことを表す。
- `Handled: false` は Generate path を表す。
- `Handled` は success / failure の一般状態ではない。

## 検証結果

実行した検証:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
```

結果:

- `internal/application/orchestrator`: ok
- `internal/application/service`: ok
- `internal/domain/capability`: ok
- `internal/domain/patch`: ok
- `cmd/picoclaw`: ok
- `git diff --check`: 成功
- `git status --short`: `?? tests/` のみ

## Push 済み commit

- `dcb95e8 docs: Phase4 CodeExecutor境界整理仕様を追加`
- `d0fd74f docs: Phase4-0 CodeExecutor現在契約を追加`
- `8e5bb7b test: Phase4-0 CodeExecutor契約を固定`
- `17ec590 docs: Phase4-1 Coder selection境界整理を追加`
- `2f28c58 refactor: Coder selection境界を分離`
- `050e031 docs: Phase4-2 proposal path境界整理を追加`
- `42e6f9e refactor: proposal path境界を分離`
- `19ba25e docs: Phase4-3 Generate path event境界整理を追加`
- `2ee6ef6 refactor: Generate path event境界を分離`
- `3284d4a docs: Phase4-4 CodeExecutionResponse契約整理を追加`
- `3b2b49b refactor: CodeExecutionResponse契約を明確化`

## 完了判定

Phase4 は完了と判定する。

理由:

- CodeExecutor / Coder selection / proposal path / Generate path / Worker handoff の責務境界を文書化した。
- Coder は proposal / Generate を担当し、実行は WorkerExecutionService が担当する境界を維持した。
- invalid proposal が Worker に渡らない契約を維持した。
- degraded route notice を fallback success として扱っていない。
- `Handled` を success / failure と混同しない形に整理した。
- 対象パッケージのテストが成功している。
- Phase4 の文書と実装差分は push 済みである。

## Phase5 に進む前の確認事項

Phase5 に進む場合、候補は次のどちらかを先に決める。

- CodeExecutor 周辺の helper を別ファイルへ分離するか。
- Phase2 / Phase3 / Phase4 の境界を踏まえて、Chat / Worker / Coder route chain 全体の統合確認を行うか。

どちらを選ぶ場合でも、仕様変更と構造変更を混ぜない。
