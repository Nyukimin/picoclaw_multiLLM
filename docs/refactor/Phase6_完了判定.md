# Phase6 完了判定

## 目的

Phase6 は、Phase2 から Phase5 で整理した Chat / Worker / Coder の責務境界が、route chain 全体で矛盾していないことを確認する段階である。

この完了判定では、`MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の現在契約を、実装変更ではなく契約テストで固定した結果を記録する。

## 実施範囲

対象にした範囲は次の通り。

- `MessageOrchestrator.ProcessMessage` から CODE 系 route へ進む経路。
- `executeCodeViaShiro` から `CodeExecutor.ExecuteCode` への委譲。
- explicit `CODE1` / `CODE2` / `CODE3` / `CODE4` と generic `CODE` の route chain。
- Coder proposal path と Generate path の分岐。
- valid proposal の `WorkerExecutionService.ExecuteProposal()` への handoff。
- Worker error / Generate error / degraded notice / `Handled` の混同防止。
- `routing.Route.IsCoderRoute()` の CODE4 契約。

対象外にした範囲は次の通り。

- `MessageOrchestrator` の大規模分割。
- `WorkerExecutionService` 内部の再分割。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS。
- runtime config。
- 未追跡の `tests/`。

## 追加した契約テスト

### CODE route chain

`internal/application/orchestrator/message_orchestrator_code_path_test.go` に次を追加した。

- generic `CODE` route でも Shiro 経由 event が維持されること。
- generic `CODE` route の event order が `mio -> shiro start`、`shiro -> coder start`、`coder -> shiro response`、`shiro -> mio response` の順であること。
- `CODE3` proposal path の event order が `mio -> shiro start`、`shiro -> coder start`、`coder -> shiro plan`、`shiro -> mio Worker start`、`shiro -> mio result` の順であること。

### proposal / Generate / error 境界

`internal/application/orchestrator/code_executor_test.go` に次を追加した。

- proposal interface 非対応 Coder は Generate path に戻り、WorkerExecutionService に到達しないこと。
- WorkerExecutionService error は wrapped error として返り、handled success response にならないこと。
- Generate path error は fallback success response にならず、`shiro -> mio` success response を出さないこと。

`internal/application/orchestrator/message_orchestrator_route_chain_contract_test.go` に次を追加した。

- WorkerExecutionService error は `ProcessMessage` error になり、user-facing success event に変換されないこと。
- Generate error は `ProcessMessage` error になり、fallback response text に変換されないこと。

### routing domain

`internal/domain/routing/route_test.go` に次を追加した。

- `CODE4` が `Route.IsCoderRoute()` で coder route と判定されること。

## 維持した既存契約

Phase6 では次の既存契約を変更していない。

- handler 本体、DTO、SSE event は変更しない。
- Viewer 表示契約、音声、口パク、ログを混同しない。
- IdleChat 契約は変更しない。
- STT / TTS provider の挙動は変更しない。
- LLM provider の挙動は変更しない。
- runtime config の意味は変更しない。
- fallback / degraded / error / `Handled` を同じ状態として扱わない。
- Coder は破壊的操作を直接実行しない。
- valid proposal の適用は WorkerExecutionService が担当する。

## 検証結果

baseline と after で、次を実行した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

after の結果は成功。

```text
ok  	github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator
ok  	github.com/Nyukimin/picoclaw_multiLLM/internal/application/service
ok  	github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing
ok  	github.com/Nyukimin/picoclaw_multiLLM/internal/domain/proposal
ok  	github.com/Nyukimin/picoclaw_multiLLM/internal/domain/patch
ok  	github.com/Nyukimin/picoclaw_multiLLM/cmd/picoclaw
```

加えて、差分チェックとして次を実行し、どちらも成功した。

```bash
git diff --check
git diff --stat
```

## 完了条件との対応

Phase6 の完了条件に対する判定は次の通り。

- `MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の流れを契約テストで固定した。
- CODE 系 route の Shiro 経由 event を generic `CODE` と `CODE3` proposal path まで確認した。
- proposal interface 非対応時に Generate path へ戻ることを確認した。
- nil / invalid proposal が Worker に渡らない既存契約を維持した。
- valid proposal が WorkerExecutionService に渡る既存契約を維持した。
- Worker error を success に変換しない契約を追加した。
- Generate error を fallback success に変換しない契約を追加した。
- degraded notice と `Handled` を success と混同しない既存契約を維持した。
- CODE4 を coder route として扱う domain 契約を追加した。
- production code は変更していない。

## Phase7 前の確認事項

Phase7 以降で `MessageOrchestrator` の分割へ進む場合は、次を先に確認する。

- `ProcessMessage` の session / event / route decision / TTS / response assembly を一度に分割しない。
- route dispatch と CodeExecutor handoff の契約を崩さない。
- autonomous executor の retry によって WorkerExecutionService が複数回呼ばれる可能性を、success / failure 判定と混同しない。
- Viewer event は execution log の代替にしない。
- proposal path の `Handled` は final success 判定ではなく、proposal path 処理有無として扱う。
- fallback を正常系として扱わない。
