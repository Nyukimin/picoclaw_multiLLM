# Phase2-0 route chain 契約固定

## 目的

Phase 2-0 では、`MessageOrchestrator` の route chain を分割する前に、現在の入力、出力、event、error contract を固定する。

この段階では実装構造を変えない。後続 Phase で `ProcessMessage` や dispatch を分けても、Chat / Worker / Coder の責務境界、Viewer event、session、report、TTS hook が落ちないことを確認できる土台を作る。

## 対象範囲

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/*_test.go`
- `internal/application/service/worker_execution_service.go`
- `internal/application/service/*_test.go`

## 対象外

- route dispatch の実装分割。
- `WorkerExecutionService` の内部実行方式変更。
- `ToolRunner` / `PolicyEngine` の変更。
- LLM provider、Viewer JS / CSS、STT / TTS provider、IdleChat 契約の変更。
- route 名や外部 API 契約の変更。

## 固定する契約

### `ProcessMessage`

- session を load または create する。
- `message.received` を route 判断前に emit する。
- `MioAgent.HandleChatCommand` は route 判断前に実行する。
- chat command handled の場合、`RouteCHAT`、confidence `1.0`、新規 job ID を返す。
- normal route では `task.NewJobID()` で task / response の job ID を作る。
- attachment がある場合は `viewer.attachment.received` を emit する。
- `MioAgent.DecideAction` の結果を `routing.decision` として emit する。
- CHAT 以外では worker busy を true にし、終了時に false へ戻す。
- route execution 後に session へ task を追加し、session を保存する。

### route dispatch

- CHAT は `MioAgent.Chat`。
- OPS は `ShiroAgent.Execute`。
- PLAN / RESEARCH は `MioAgent.Chat`。
- ANALYZE は `HeavyAgent.Generate` があれば Heavy、なければ `MioAgent.Chat`。
- WILD は `WildAgent.Generate`。Wild agent がなければ error。
- CODE / CODE1 / CODE2 / CODE3 / CODE4 は `CodeExecutor.ExecuteCode`。
- unknown route は error。成功 response に変換しない。

### Coder / Worker 境界

- Coder は proposal または text を生成する。
- valid proposal だけが `WorkerExecutionService.ExecuteProposal` に渡る。
- invalid proposal、no coder、proposal generation error、worker execution error は成功扱いしない。
- Coder に file edit / shell / git 実行責務を戻さない。

### event / TTS / report

- `agent.start`、`agent.response`、`agent.thinking`、`entry.stage`、`agent.notice` の意味を維持する。
- TTS / VTuber failure は degraded log として扱い、route execution error と混同しない。
- autonomous executor の report store 接続を維持する。
- Viewer 表示本文と TTS chunk / 口パク trigger / log を混同しない。

## 追加する検証

既存テストで大部分は固定済みだが、Phase 2-0 では次を明示的に追加または確認する。

- route decision event が route execution より前に出ること。
- chat command handled では route decision が走らないこと。
- unknown route が error になり、成功 response にならないこと。
- invalid proposal が Worker 実行へ進まないこと。
- TTS start failure が degraded 扱いで、route response を壊さないこと。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

## 完了条件

- Phase 2-0 の契約固定文書が `docs/refactor/` にある。
- route chain の契約を固定するテストが追加または既存テストで確認済みである。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw` が成功している。
- `git diff --check` が成功している。
- 実装分割はまだ行っていない。
