# Phase12 完了判定

## 目的

Phase12 は、`MessageOrchestrator` に残っていた `buildTaskForRequest` を `messageTaskContextBuilder` へ分離し、task / job ID / attachment event / TTS session ID 生成の境界を明確にする段階である。

## 実施範囲

対象にした範囲は次の通り。

- `messageTaskContextBuilder`
- `MessageOrchestrator.ProcessMessage` から task context builder への委譲
- attachment event emission の event port 化
- TTS session ID 生成条件の collaborator 化
- task context builder 境界の契約テスト

対象外にした範囲は次の通り。

- event payload / DTO / SSE event の変更。
- TTS lifecycle の追加分割。
- route dispatcher の追加分割。
- provider 挙動変更。
- Viewer JS / CSS。
- IdleChat。
- STT。
- LLM provider。
- runtime config。
- distributed orchestrator。
- 未追跡の `tests/`。

## 追加・変更した collaborator

### `messageTaskContextBuilder`

`message_orchestrator_task.go` に追加した。

責務:

- `ProcessMessageRequest` から `task.Task` を作る。
- `task.JobID` を作る。
- attachment を task に引き継ぐ。
- attachment がある場合に `viewer.attachment.received` event を emit する。
- TTS enabled の場合だけ TTS session ID を作る。

契約:

- 入力は `ProcessMessageRequest`。
- 出力は `task.Task`、`task.JobID`、TTS session ID。
- 副作用は attachment event emission。
- 直接永続化しない。
- 新しいログは追加しない。
- error は返さない。
- listener nil は event port 側の skipped log になり、task build は継続する。

## `MessageOrchestrator` に残した責務

`MessageOrchestrator` には次を残した。

- request / response / agent interface / repository interface の定義。
- dependency holding。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。

task context assembly の詳細は `messageTaskContextBuilder` に移した。

## 維持した契約

Phase12 では次を維持した。

- task の user message / channel / chat ID / attachments を変えない。
- job ID を task / event / TTS session ID / response で共有する。
- attachment event type / from / to / content / route / jobID / sessionID / channel / chatID を変えない。
- TTS session ID は TTS enabled の場合だけ `sessionID-jobID` 形式で作る。
- TTS session ID が空の場合、TTS lifecycle は no-op のまま。
- Viewer event と execution log / response text / audio chunk を混同しない。

## 追加した契約テスト

`internal/application/orchestrator/message_orchestrator_phase12_task_context_test.go` を追加した。

追加した確認は次の通り。

- attachment が task に引き継がれること。
- attachment event が `viewer.attachment.received` / viewer -> mio で emit されること。
- attachment event が task と同じ job ID を持つこと。
- TTS disabled の場合、TTS session ID が空であること。
- TTS enabled の場合、TTS session ID が `sessionID-jobID` 形式であること。

## 検証結果

baseline として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

実装途中の focused test として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase12|TestPhase11|TestMessageOrchestrator_RouteChainContract_'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase10|TestMessageOrchestrator_TTSBridge_'
```

最終確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

## 完了条件との対応

Phase12 の完了条件に対する判定は次の通り。

- `messageTaskContextBuilder` を追加した。
- task context builder の入力、出力、副作用、永続化、ログ、エラー契約を本書に記録した。
- event payload の意味を変えていない。
- Phase8-11 契約を変更していない。
- 未追跡の `tests/` は触っていない。

## Phase13 前の確認事項

Phase13 のおすすめは `MessageOrchestrator` の残責務棚卸しと完了判定である。Phase8-12 で session、response、command、routing、idle、autonomous、route dispatch、TTS lifecycle、event port、task context builder を分離したため、次は `MessageOrchestrator` に残すべき責務と、次の大きな対象に進むべきかを判断する。
