# Phase7 完了判定

## 目的

Phase7 は、Phase6 で固定した route chain 契約を守ったまま、`MessageOrchestrator` の責務をファイル単位で分離する段階である。

この完了判定では、`ProcessMessage` の top-level orchestration を残しつつ、session、response、command、routing、task、TTS、idle、autonomous、route dispatch、event の責務を分離した結果を記録する。

## 実施範囲

対象にした範囲は次の通り。

- `MessageOrchestrator.ProcessMessage`
- session load / create / save
- `message.received` event
- pre-routing chat command
- task / job / TTS session ID assembly
- route decision
- TTS session start / end / push / stream hook
- idle notifier busy state
- `executeTask`
- `executeAutonomousTask`
- route-specific execution entrypoint
- response assembly
- event emission helper

対象外にした範囲は次の通り。

- Phase6 で固定した route chain 契約の変更。
- CodeExecutor の再分割。
- WorkerExecutionService 内部の再分割。
- ToolRunner / PolicyEngine。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- distributed orchestrator の大規模変更。
- 未追跡の `tests/`。

## 追加したファイル

Phase7 で追加したファイルは次の通り。

- `internal/application/orchestrator/message_orchestrator_events.go`
- `internal/application/orchestrator/message_orchestrator_session.go`
- `internal/application/orchestrator/message_orchestrator_response.go`
- `internal/application/orchestrator/message_orchestrator_commands.go`
- `internal/application/orchestrator/message_orchestrator_task.go`
- `internal/application/orchestrator/message_orchestrator_routing.go`
- `internal/application/orchestrator/message_orchestrator_idle.go`
- `internal/application/orchestrator/message_orchestrator_tts_lifecycle.go`
- `internal/application/orchestrator/message_orchestrator_autonomous.go`
- `internal/application/orchestrator/message_orchestrator_routes.go`

## 分離した責務

### events

`message_orchestrator_events.go` に `emit` と `emitMessageReceived` を分離した。

維持した契約:

- listener が nil の場合は panic せず skipped log を出す。
- `message.received` event は user -> mio として route 未決定のまま emit する。
- Viewer event と execution log を混同しない。

### session

`message_orchestrator_session.go` に session load / create / save を分離した。

維持した契約:

- `session.ErrSessionNotFound` の場合だけ新規 session を作成する。
- route execution 成功後だけ completed task として保存する。
- load / save error は既存の wrap 文言を維持する。

### response

`message_orchestrator_response.go` に response assembly を分離した。

維持した契約:

- normal route response は route decision の route / confidence / jobID を使う。
- chat command handled response は `CHAT`、confidence `1.0` とする。
- error path で fallback response を生成しない。

### commands

`message_orchestrator_commands.go` に pre-routing chat command を分離した。

維持した契約:

- handled command は route decision を bypass する。
- handled command では `agent.response` event を emit する。
- command error は `chat command failed` として返す。

### task

`message_orchestrator_task.go` に task / job / TTS session ID assembly を分離した。

維持した契約:

- job ID を task と response で共有する。
- attachments を task に引き継ぐ。
- attachment event を emit する。
- TTS bridge がある場合だけ TTS session ID を作る。

### routing

`message_orchestrator_routing.go` に Mio route decision を分離した。

維持した契約:

- `MioAgent.DecideAction` に route 判断を委譲する。
- `routing.decision` event に confidence と route を残す。
- route decision error は `routing decision failed` として返す。

### idle

`message_orchestrator_idle.go` に busy state guard を分離した。

維持した契約:

- activity notification と chat busy を processing 中だけ立てる。
- worker busy は `CHAT` 以外だけ立てる。
- error path でも defer で解除する。

### TTS lifecycle

`message_orchestrator_tts_lifecycle.go` に TTS session start / end / push / stream hook を分離した。

維持した契約:

- TTS bridge がない場合は何もしない。
- TTS start / end / push error は degraded log として扱う。
- 音声、口パク、Viewer 表示本文を混同しない。

### autonomous execution

`message_orchestrator_autonomous.go` に autonomous executor の request 組み立て、retry、verify helper を分離した。

維持した契約:

- unsupported route は error にする。
- retry 時だけ retry message を作る。
- `verifyByContract` を維持する。
- apply / verify error を fallback success にしない。

### route dispatch

`message_orchestrator_routes.go` に route-specific execution entrypoint を分離した。

維持した契約:

- `CHAT` は autonomous executor を通らない。
- CODE 系 route は `executeCodeViaShiro` を通る。
- `executeCodeViaShiro` は `CodeExecutor.ExecuteCode` に委譲する。
- Phase6 の Shiro relay event order を維持する。

## `MessageOrchestrator` に残した責務

`message_orchestrator.go` には次を残した。

- request / response / agent interface / struct 定義。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。
- dependency の保持。

これにより、`ProcessMessage` は各責務の呼び出し順を表す入口として残し、詳細実装は専用ファイルへ分離した。

## 検証結果

baseline と after で次を実行した。

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

route chain 契約確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

TTS lifecycle 確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

差分チェックとして次を実行し、成功した。

```bash
git diff --check
git diff --stat
```

## 完了条件との対応

Phase7 の完了条件に対する判定は次の通り。

- `MessageOrchestrator` が top-level orchestration として説明できる。
- session、response、command、routing、task、TTS、idle、autonomous、route dispatch、event の責務を分離した。
- Phase6 route chain 契約を維持した。
- CodeExecutor / WorkerExecutionService 内部は変更していない。
- handler / DTO / SSE event / Viewer JS / CSS / IdleChat / STT / TTS provider / LLM provider / runtime config は変更していない。
- production behavior の意図的な変更は行っていない。

## Phase8 前の確認事項

Phase8 以降で進める候補は次の通り。

- `MessageOrchestrator` の collaborator 化。
- autonomous execution coordinator の interface 化。
- route dispatch の adapter / application 境界整理。
- distributed orchestrator への同様の責務整理を別 Phase として検討する。

Phase8 に進む前に、Phase7 の分離が単なるファイル移動に留まっていないか、入力、出力、副作用、永続化、ログ、エラー契約を再確認する。
