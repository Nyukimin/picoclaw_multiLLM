# Phase8 完了判定

## 目的

Phase8 は、Phase7 でファイル分離した `MessageOrchestrator` 周辺責務を、意味のある collaborator 境界へ整理する段階である。

この完了判定では、`ProcessMessage` の top-level orchestration を維持したまま、response、session lifecycle、pre-routing command、route decision、idle busy guard、autonomous execution を private collaborator として分離した結果を記録する。

## 実施範囲

対象にした範囲は次の通り。

- `messageResponseAssembler`
- `messageSessionLifecycle`
- `preRoutingCommandHandler`
- `routeDecisionCoordinator`
- `idleBusyGuardFactory`
- `autonomousExecutionCoordinator`
- `MessageOrchestrator.ProcessMessage` から各 collaborator への委譲
- collaborator 化に伴う setter 反映
- collaborator 境界の契約テスト

対象外にした範囲は次の通り。

- Phase6 で固定した route chain 契約の変更。
- route dispatcher の collaborator 化。
- TTS lifecycle の collaborator 化。
- event emitter の collaborator 化。
- task context builder の collaborator 化。
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

## 追加・変更した collaborator

### `messageResponseAssembler`

`message_orchestrator_response.go` に追加した。

責務:

- normal route response の `Response` / `Route` / `Confidence` / `JobID` を組み立てる。
- pre-routing chat command response を `CHAT` / confidence `1.0` として組み立てる。

契約:

- 入力は response text、`routing.Decision`、`task.JobID`、chat command response text。
- 出力は `ProcessMessageResponse`。
- 副作用、永続化、ログは持たない。
- error path で fallback response を生成しない。

### `messageSessionLifecycle`

`message_orchestrator_session.go` に追加した。

責務:

- request から session を load / create する。
- route execution 成功後に completed task を session に保存する。

契約:

- 入力は `context.Context`、`ProcessMessageRequest`、`*session.Session`、`task.Task`。
- 出力は `*session.Session` または error。
- `SessionRepository.Load` / `SessionRepository.Save` を通じて永続化する。
- `session.ErrSessionNotFound` の場合だけ新規 session を作る。
- load / create 失敗は `failed to load or create session`、save 失敗は `failed to save session` として返す。
- save timing は route execution 成功後のまま維持する。

### `preRoutingCommandHandler`

`message_orchestrator_commands.go` に追加した。

責務:

- `MioAgent.HandleChatCommand` に pre-routing command を問い合わせる。
- handled command の場合、route decision を bypass する。
- handled command の response event を emit する。

契約:

- 入力は `context.Context` と `ProcessMessageRequest`。
- 出力は `ProcessMessageResponse`、handled bool、error。
- 副作用は handled command 時の `agent.response` event emission。
- 永続化は持たない。
- command error は `chat command failed` として返す。

### `routeDecisionCoordinator`

`message_orchestrator_routing.go` に追加した。

責務:

- `MioAgent.DecideAction` に route decision を委譲する。
- `routing.decision` event を emit する。

契約:

- 入力は `context.Context`、`task.Task`、`ProcessMessageRequest`、`task.JobID`。
- 出力は `routing.Decision`。
- 副作用は `routing.decision` event emission。
- 永続化は持たない。
- route decision error は `routing decision failed` として返す。
- route decision event は dispatch より前に emit される。

### `idleBusyGuardFactory`

`message_orchestrator_idle.go` に追加した。

責務:

- chat busy と worker busy の開始・解除 closure を作る。
- `IdleNotifier` が nil の場合は no-op closure を返す。

契約:

- 入力は `IdleNotifier` と `routing.Route`。
- 出力は終了用 closure。
- 副作用は `NotifyActivity`、`SetChatBusy`、`SetWorkerBusy`。
- 永続化とログは持たない。
- `CHAT` route では worker busy を立てない。
- error path でも呼び出し側の `defer` で解除できる。
- `SetIdleNotifier` 後は collaborator が最新 notifier を使う。

### `autonomousExecutionCoordinator`

`message_orchestrator_autonomous.go` に追加した。

責務:

- autonomous route の request を `autonomousapp.RunExecutor` 用に組み立てる。
- stage event を emit する。
- retry message を組み立てる。
- route direct executor へ実行を委譲する。
- contract verify を実行する。

契約:

- 入力は `context.Context`、`task.Task`、route、session/channel/chat IDs、TTS session ID。
- 出力は response text と error。
- 副作用は `entry.stage` event emission、route direct execution、report store への記録委譲。
- report store が設定されている場合、`autonomousapp.RunExecutor` 経由で実行レポートが保存される。
- unsupported route、contract normalize error、executor error、verify failure を fallback success にしない。
- route dispatcher は method のまま維持する。
- `SetReportStore` 後は collaborator が最新 reporter を使う。
- maxRepair は `MessageOrchestrator.maxRepairOrDefault` を provider function として参照する。

## `MessageOrchestrator` に残した責務

`MessageOrchestrator` には次を残した。

- request / response / agent interface / repository interface の定義。
- dependency holding。
- constructor。
- public setter。
- `ProcessMessage` の top-level orchestration。
- route dispatcher。
- TTS lifecycle。
- event emitter。
- task context builder。

これにより、`ProcessMessage` は処理順序を示す入口として維持し、collaborator は説明可能な責務境界だけを担当する。

## 維持した契約

Phase8 では次を維持した。

- Phase6 route chain event order。
- handled chat command が route decision を bypass する契約。
- route decision が dispatch より前に行われる契約。
- CODE 系 route が Shiro 経由で CodeExecutor に委譲される契約。
- Worker error / Generate error を success response に変換しない契約。
- session save は route execution 成功後だけ行う契約。
- TTS degraded log を success と混同しない契約。
- Viewer event、execution log、response text、音声、口パクを混同しない契約。
- handler / DTO / SSE event / Viewer JS / CSS / IdleChat / STT / TTS provider / LLM provider / runtime config は変更しない契約。

## 追加した契約テスト

`internal/application/orchestrator/message_orchestrator_phase8_collaborators_test.go` を追加した。

追加した確認は次の通り。

- `messageResponseAssembler` が normal response の route / confidence / jobID を維持すること。
- `messageResponseAssembler` が chat command response を `CHAT` / confidence `1.0` として組み立てること。
- `idleBusyGuardFactory` が nil notifier で no-op になること。
- `idleBusyGuardFactory` が chat busy を開始・解除すること。
- `idleBusyGuardFactory` が `CHAT` route で worker busy を立てないこと。
- `idleBusyGuardFactory` が non-CHAT route で worker busy を開始・解除すること。
- `autonomousExecutionCoordinator` が route direct executor を呼ぶこと。
- `autonomousExecutionCoordinator` が `SetReportStore` 後の reporter に execution report を保存すること。
- `autonomousExecutionCoordinator` が stage event を emit すること。

## 検証結果

baseline として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

実装途中の focused test として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase8|TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

最終確認として次を実行し、成功した。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

## 完了条件との対応

Phase8 の完了条件に対する判定は次の通り。

- `MessageOrchestrator` が top-level orchestration として説明できる状態を維持した。
- response、session lifecycle、pre-routing command、route decision、idle busy guard、autonomous execution を collaborator 化した。
- 各 collaborator の入力、出力、副作用、永続化、ログ、エラー契約を本書に記録した。
- route dispatcher、TTS lifecycle、event emitter、task context builder は Phase8 では method のまま残した。
- Phase6 / Phase7 契約を変更していない。
- handler / DTO / SSE event / Viewer JS / CSS / IdleChat / STT / TTS provider / LLM provider / runtime config は変更していない。
- 未追跡の `tests/` は触っていない。

## Phase9 前の確認事項

Phase9 に進む場合は、次のどれを対象にするかを先に決める。

- route dispatcher の境界整理。
- TTS lifecycle の境界整理。
- event emitter の port 化。
- task context builder の attachment / TTS session ID 境界整理。

Phase9 でも、route chain、TTS degraded、Viewer event、execution log、response text の契約を混同しない。
