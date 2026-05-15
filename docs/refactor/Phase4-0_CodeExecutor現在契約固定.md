# Phase4-0 CodeExecutor 現在契約固定

## 目的

Phase4-0 は、`CodeExecutor` の境界整理に入る前に、現在の実行契約を文書とテストで固定する段階である。

この段階では実装分割を目的にしない。`DefaultCodeExecutor` が現在どの入力から Coder を選び、どの条件で proposal path / Generate path / Worker handoff に進むかを明確にし、以降の Phase4-1 以降で挙動を変えていないことを確認できる状態にする。

## 対象範囲

- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/orchestrator/coder_status.go`
- `internal/domain/capability/`
- `internal/domain/proposal/`
- `internal/domain/patch/`
- `internal/application/service/worker_execution_service.go` への接続点

## 対象外

- `MessageOrchestrator` の route chain 変更。
- `WorkerExecutionService` の内部実行方式変更。
- `ToolRunner` / `PolicyEngine` の変更。
- Coder provider の挙動変更。
- proposal / patch format の意味変更。
- handler、DTO、SSE event、Viewer JS / CSS、IdleChat、STT / TTS、runtime config の変更。
- 未追跡の `tests/`。

## 固定する現在契約

### Coder selection

- `CODE1` は `coder1` を選ぶ。
- `CODE2` は `coder2` を選ぶ。
- `CODE3` は `coder3` を選ぶ。
- `CODE4` は `coder4` を選ぶ。
- 明示 route の Coder が nil の場合は error にする。
- 汎用 `CODE` は `coder1`、`coder2`、`coder3`、`coder4` の順で利用可能な Coder を選ぶ。
- `CoderStatus` がある場合、busy な Coder は skip し、acquire できた Coder を選ぶ。
- `coderCaps` が nil でない場合、`capability.SelectCoder` に選択を委譲する。
- dynamic selection で選ばれた Coder が初期化されていない場合は error にする。

### CoderStatus acquire / release

- `CoderStatus` は汎用 `CODE` route の自動選択時だけ使う。
- acquire に成功した Coder は、`ExecuteCode` の終了時に release する。
- release は success / error の両方で実行する。
- release 漏れにより busy state が残る状態を正常系にしない。

### proposal path

- `CODE1` / `CODE2` / `CODE3` / `CODE4` は proposal path を試行対象にする。
- dynamic selection で `degradedRoute == CODE3` の場合も proposal path を試行対象にする。
- selected Coder が `CoderAgentWithProposal` を満たさない場合、proposal path は handled false で Generate path に戻す。
- proposal generation error は handled true + error とする。
- nil / invalid proposal は handled true + error とし、Worker に渡さない。
- valid proposal の場合だけ `WorkerExecutionService.ExecuteProposal(ctx, req.Task.JobID(), proposal)` に渡す。
- Worker execution error は handled true + wrapped error とする。
- proposal path の response は `Handled: true` とする。

### Generate path

- proposal path が処理しない場合、selected Coder の `Generate` を呼ぶ。
- Generate error は event を出して error を返す。
- Generate success は coder -> shiro と shiro -> mio の response event を出す。
- Generate path の response は `Handled: false` とする。

### degraded route notice

- degraded route notice は品質縮退の通知であり、fallback success ではない。
- degraded route notice が出ても、proposal generation、Worker execution、Generate path はそれぞれ失敗し得る。
- notice は Viewer-facing event であり、Worker log や実行結果の代替ではない。

### event order

- `ExecuteCode` は Coder 選択後に code handoff log を出す。
- degraded route がある明示 route では `agent.notice` を出す。
- その後、mio -> shiro の `agent.start` と shiro -> Coder の `agent.start` を出す。
- proposal path では plan response、Worker start、Worker result / error の順を維持する。
- Generate path では Coder response、Shiro response の順を維持する。

## 入力

- `CodeExecutionRequest`
- `routing.Route`
- `CoderAgent`
- `CoderAgentWithProposal`
- `capability.CoderCapability`
- `CoderStatus`

## 出力

- `CodeExecutionResponse`
- error
- CodeExecutor event

## 副作用

- CoderStatus acquire / release。
- Coder `Generate` / `GenerateProposal` 呼び出し。
- valid proposal の場合だけ WorkerExecutionService への handoff。
- event emission。
- log 出力。

## 永続化

CodeExecutor 自体は永続化を行わない。

WorkerExecutionService に渡った後の副作用は Phase3 の契約に従う。Phase4-0 では WorkerExecutionService 内部に踏み込まない。

## ログ

- coder selected / skip。
- code handoff。
- degraded route。

ログは契約確認の補助であり、Viewer 表示、音声、口パク、Worker 実行結果とは混同しない。

## エラー契約

- 明示 route の Coder missing は error。
- dynamic selection の selected coder missing は error。
- 汎用 `CODE` で全 Coder unavailable は error。
- `CoderStatus` ありで全 Coder busy / unavailable は error。
- proposal generation error は handled true + error。
- nil / invalid proposal は handled true + error で Worker に渡さない。
- Worker execution error は `worker execution failed` で wrap する。
- Generate error は fallback success にしない。

## 変更してはいけない既存挙動

- route と Coder の対応。
- 汎用 `CODE` の fallback order。
- dynamic selection の `capability.SelectCoder` 利用。
- CoderStatus release 条件。
- valid proposal だけ Worker に渡す条件。
- `Handled` の意味。
- event type / from / to / route。

## 実装手順

1. baseline test を実行する。
2. `CodeExecutor` の現在契約をテストで追加固定する。
3. 実装分割は行わない。
4. gofmt を実行する。
5. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## リスク

- CoderStatus release 漏れで busy state が残る。
- degraded route notice を成功扱いに見せる。
- invalid proposal が Worker に渡る。
- `Handled` を success / failure と混同する。
- event の from / to / route を変えて Viewer 側の観測を壊す。

## 完了条件

- Phase4-0 の現在契約が `docs/refactor/` に記録されている。
- CoderStatus release、degraded route notice、proposal / Generate path の契約を確認できるテストがある。
- baseline / after test が成功している。
- `git diff --check` が成功している。
- コードの挙動変更を目的とした差分を入れていない。
