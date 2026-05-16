# Phase8 MessageOrchestrator collaborator 化実装仕様

## 1. Phase8 の目的

Phase8 は、Phase7 でファイル分離した `MessageOrchestrator` 周辺責務を、意味のある collaborator 境界へ段階的に整理する段階である。

目的は次の通り。

- `MessageOrchestrator.ProcessMessage` は top-level orchestration として残す。
- `MessageOrchestrator` が全ての helper method を直接抱える状態を減らす。
- session、response、routing、idle、autonomous execution など、入力、出力、副作用、永続化、ログ、エラー契約を説明できる単位だけ collaborator 化する。
- Phase6 で固定した Chat / Worker / Coder route chain 契約を変更しない。
- Phase7 で分離したファイル責務を崩さず、単なる巨大 service / manager / helper / util に寄せない。
- モジュール化と疎結合を最重要方針とし、将来 `internal/application` 内の workflow / coordinator / adapter 境界へ移しやすくする。

Phase8 は挙動変更ではなく、構造整理と契約固定を目的にする。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/message_orchestrator.go`
  - `MessageOrchestrator.ProcessMessage`
  - constructor
  - dependency holding
- `internal/application/orchestrator/message_orchestrator_session.go`
- `internal/application/orchestrator/message_orchestrator_response.go`
- `internal/application/orchestrator/message_orchestrator_commands.go`
- `internal/application/orchestrator/message_orchestrator_task.go`
- `internal/application/orchestrator/message_orchestrator_routing.go`
- `internal/application/orchestrator/message_orchestrator_idle.go`
- `internal/application/orchestrator/message_orchestrator_tts_lifecycle.go`
- `internal/application/orchestrator/message_orchestrator_autonomous.go`
- `internal/application/orchestrator/message_orchestrator_routes.go`
- `internal/application/orchestrator/message_orchestrator_events.go`
- `internal/application/orchestrator/message_orchestrator_*test.go`
- ProcessMessage、route chain、TTS、session、response の既存テスト

## 3. 対象外

Phase8 では次を対象外にする。

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

fallback、degraded、error、handled success を混同しない。Viewer 表示、音声、口パク、ログも混同しない。

## 4. Phase7 後の現在構造

### `message_orchestrator.go`

責務:

- `ProcessMessageRequest` / `ProcessMessageResponse` / agent interface / repository interface の定義。
- `MessageOrchestrator` struct による依存保持。
- `NewMessageOrchestrator` による初期組み立て。
- public setter。
- `ProcessMessage` の top-level orchestration。

`ProcessMessage` は、chat busy 開始、session load、message received event、pre-routing command、task 作成、route decision、TTS session start、worker busy、route execution、TTS session end、session save、response assembly の順序を表す入口である。

### `message_orchestrator_session.go`

責務:

- request から session を load / create する。
- `session.ErrSessionNotFound` の場合だけ新規 session を作る。
- route execution 成功後に completed task を session に保存する。
- load / save error を既存の wrap 文言で返す。

永続化は `SessionRepository` 経由で発生する。

### `message_orchestrator_response.go`

責務:

- route decision と job ID から `ProcessMessageResponse` を組み立てる。
- pre-routing chat command の handled response を `CHAT` / confidence `1.0` として組み立てる。

永続化と外部副作用は持たない。

### `message_orchestrator_commands.go`

責務:

- `MioAgent.HandleChatCommand` に pre-routing command を問い合わせる。
- handled command の場合、route decision を bypass する。
- handled command の `agent.response` event を emit する。
- command error を `chat command failed` として返す。

### `message_orchestrator_task.go`

責務:

- request から `task.Task` と `task.JobID` を作る。
- attachments を task に引き継ぐ。
- attachment がある場合に Viewer event を emit する。
- TTS bridge がある場合だけ TTS session ID を作る。

task 作成自体は pure に近いが、attachment event と TTS bridge 判定が混ざっている。

### `message_orchestrator_routing.go`

責務:

- `MioAgent.DecideAction` に route 判断を委譲する。
- `routing.decision` event を emit する。
- route decision error を `routing decision failed` として返す。

### `message_orchestrator_idle.go`

責務:

- `IdleNotifier` がある場合だけ activity / chat busy / worker busy を操作する。
- chat busy は `ProcessMessage` 処理中だけ立てる。
- worker busy は `CHAT` 以外の route でだけ立てる。
- 呼び出し側が `defer` で解除できる closure を返す。

### `message_orchestrator_tts_lifecycle.go`

責務:

- route に応じた TTS session start request を作り、TTS bridge に渡す。
- TTS session end を bridge に渡す。
- stream callback を TTS / VTuber / `agent.thinking` event に接続する。
- final response を TTS / VTuber に push する。
- bridge error は degraded log として記録する。

音声、口パク、Viewer 表示本文を混同しない契約を持つため、Phase8 では慎重に扱う。

### `message_orchestrator_autonomous.go`

責務:

- autonomous route の対象判定。
- `contractapp.NormalizeRequestWithRoute` による contract 作成。
- `autonomousapp.RunExecutor` request の組み立て。
- stage event の emit。
- retry 時の user message 再構成。
- `executeRouteDirect` への実行委譲。
- contract に基づく verify。
- failure kind / reason の分類。

route dispatch と retry / verify が密接に絡むため、切り出し時は route dispatcher と同時に動かさない。

### `message_orchestrator_routes.go`

責務:

- `executeTask` で `CHAT` と autonomous route を分岐する。
- `executeChatRoute` で Mio chat、stream hook、TTS finalize、event emission を行う。
- `executeRouteDirect` で OPS / CODE / WILD / PLAN / ANALYZE / RESEARCH を route 別に分岐する。
- CODE 系 route は `executeCodeViaShiro` から `CodeExecutor.ExecuteCode` へ委譲する。
- route-specific response event と TTS push を行う。

Phase6 の Shiro relay event order と WorkerExecutionService handoff 契約に直結する。

### `message_orchestrator_events.go`

責務:

- listener が nil の場合は panic せず skipped log を出す。
- event emit の共通処理を行う。
- `message.received` event を user -> mio として emit する。

Viewer event は execution log ではない。ここを抽象化すると event order と観測契約に影響する。

### まだ `MessageOrchestrator` method として残る理由

Phase7 の各関数は、`MessageOrchestrator` が持つ依存に直接アクセスしている。

- `mio`
- `shiro`
- `wild`
- `heavy`
- `codeExecutor`
- `sessionRepo`
- `listener`
- `reporter`
- `idleNotifier`
- `ttsBridge`
- `vtuberBridge`
- `maxRepair`

また、Phase6 / Phase7 で固定した event order、route chain、TTS degraded log、session save timing を壊さないため、Phase7 では method のままファイル分離に留めている。Phase8 では、この結合を一度に剥がさず、契約を説明できる単位から collaborator 化する。

## 5. collaborator 化候補の比較

### session lifecycle collaborator

- collaborator 化する価値: 高い。session load / create / save は永続化境界が明確で、`SessionRepository` だけを依存として渡せる。
- collaborator 化しない場合の理由: `ProcessMessage` の流れを読むだけなら現状でも理解できるが、session 永続化の責務が `MessageOrchestrator` method に残り続ける。
- 入力: `context.Context`、`ProcessMessageRequest`、`session.Session`、`task.Task`。
- 出力: `*session.Session` または error。
- 副作用: session に task を追加する。
- 永続化: `SessionRepository.Load` / `SessionRepository.Save`。
- ログ: load / save 失敗、load / create 成功。
- エラー契約: load / create 失敗は `failed to load or create session`、save 失敗は `failed to save session`。
- 差し替え可能性: repository 実装や session lifecycle policy を差し替えやすい。
- Phase6 / Phase7 契約との関係: route chain には直接触れない。save timing は route execution 成功後のまま維持する。

### response assembler

- collaborator 化する価値: 非常に高い。pure に近く、最初に切り出す対象として安全。
- collaborator 化しない場合の理由: 現状でも free function で依存は少ないが、response contract の置き場が曖昧になる。
- 入力: response text、`routing.Decision`、`task.JobID`、chat command response text。
- 出力: `ProcessMessageResponse`。
- 副作用: なし。
- 永続化: なし。
- ログ: なし。
- エラー契約: error は返さない。fallback response は作らない。
- 差し替え可能性: route / confidence / jobID の response contract を単体で固定できる。
- Phase6 / Phase7 契約との関係: error を success response に変換しない契約を維持する。

### pre-routing command handler

- collaborator 化する価値: 中から高。route decision 前にだけ動く command bypass 境界を明確化できる。
- collaborator 化しない場合の理由: `MioAgent` と event emitter と response assembler に依存し、小さく切り出すには collaborator 間の接続が必要。
- 入力: `context.Context`、`ProcessMessageRequest`。
- 出力: `ProcessMessageResponse`、handled bool、error。
- 副作用: handled command の `agent.response` event emit。
- 永続化: なし。
- ログ: 現状は明示ログなし。
- エラー契約: `MioAgent.HandleChatCommand` error は `chat command failed`。
- 差し替え可能性: command handling の有無や実装を Mio route decision から分けられる。
- Phase6 / Phase7 契約との関係: handled command は route decision を bypass する。ここを変えると Phase6 route chain 前段に影響する。

### task context builder

- collaborator 化する価値: 中。task / job / TTS session ID の生成規則をまとめられる。
- collaborator 化しない場合の理由: attachment event emit と TTS bridge 有無判定が含まれ、pure な builder と副作用持ち collaborator が混ざりやすい。
- 入力: `ProcessMessageRequest`、TTS bridge 有無、event emitter。
- 出力: `task.Task`、`task.JobID`、TTS session ID。
- 副作用: attachment received event emit。
- 永続化: なし。
- ログ: 現状はなし。
- エラー契約: 現状 error は返さない。
- 差し替え可能性: attachment handling や TTS session ID policy を後で分離しやすい。
- Phase6 / Phase7 契約との関係: job ID を task / response / event で共有する契約を維持する必要がある。

### route decision coordinator

- collaborator 化する価値: 高い。Mio route decision と `routing.decision` event の契約を明確化できる。
- collaborator 化しない場合の理由: event emitter と MioAgent だけなので現状 method でも小さい。
- 入力: `context.Context`、`task.Task`、`ProcessMessageRequest`、`task.JobID`。
- 出力: `routing.Decision`。
- 副作用: `routing.decision` event emit。
- 永続化: なし。
- ログ: 現状は明示ログなし。
- エラー契約: `routing decision failed`。
- 差し替え可能性: Mio 以外の routing coordinator に置き換えやすい。
- Phase6 / Phase7 契約との関係: route decision は dispatch より前に必ず emit される。

### idle busy guard

- collaborator 化する価値: 高い。idle notifier の busy state を `ProcessMessage` から薄くできる。
- collaborator 化しない場合の理由: method は短いが、defer 解除漏れの検証対象として独立させる価値がある。
- 入力: `IdleNotifier`、`routing.Route`。
- 出力: 終了用 closure。
- 副作用: `NotifyActivity`、`SetChatBusy`、`SetWorkerBusy`。
- 永続化: なし。
- ログ: 現状はなし。
- エラー契約: error は返さない。nil notifier は no-op。
- 差し替え可能性: IdleChat 連携や busy policy を差し替えやすい。
- Phase6 / Phase7 契約との関係: route chain には触れないが、error path でも解除されることが必要。

### TTS lifecycle collaborator

- collaborator 化する価値: 中。TTS / VTuber / stream hook の境界を明確にできる。
- collaborator 化しない場合の理由: 音声、口パク、Viewer event、stream callback が密接で、Phase8 で急に切ると観測契約を壊しやすい。
- 入力: `context.Context`、request、route、job ID、session ID、response text、TTS / VTuber bridge。
- 出力: stream hook context、stream bundle、またはなし。
- 副作用: TTS start / push / end、VTuber push、`agent.thinking` event emit。
- 永続化: なし。
- ログ: TTS / VTuber degraded log。
- エラー契約: bridge error は degraded log で、success response とは混同しない。
- 差し替え可能性: TTS / VTuber 境界は差し替え候補だが、まず event boundary を明確化してから行う。
- Phase6 / Phase7 契約との関係: route execution event と stream hook に接続しているため、Phase8 前半では method のまま残す。

### autonomous execution coordinator

- collaborator 化する価値: 高いが後半向け。RunExecutor request、retry、verify を独立境界にできる。
- collaborator 化しない場合の理由: `executeRouteDirect`、event emitter、report store、maxRepair に依存し、route dispatcher と混ぜると危険。
- 入力: `context.Context`、`task.Task`、route、session/channel/chat IDs、TTS session ID、report store、maxRepair、route direct executor。
- 出力: response text、error。
- 副作用: `entry.stage` event emit、route direct execution、report store への記録委譲。
- 永続化: report store が設定されている場合、autonomous executor 側で記録が発生し得る。
- ログ: 主に RunExecutor 側と downstream route execution に依存。
- エラー契約: unsupported route、contract normalize error、executor error、verify failure を fallback success にしない。
- 差し替え可能性: autonomous execution の retry / verify policy を差し替えやすくする。
- Phase6 / Phase7 契約との関係: CODE 系 route の Worker handoff と error propagation を維持する。

### route dispatcher

- collaborator 化する価値: 中。route-specific execution の責務を明確にできる。
- collaborator 化しない場合の理由: Phase6 route chain event order、CodeExecutor handoff、TTS push、stream hook に強く結合しており、Phase8 では変更リスクが高い。
- 入力: `context.Context`、`task.Task`、route、session/channel/chat IDs、TTS session ID、agents、CodeExecutor、TTS lifecycle、event emitter。
- 出力: response text、error。
- 副作用: agent call、event emit、TTS / VTuber push、CodeExecutor execution。
- 永続化: WorkerExecutionService 経由で patch execution report などが発生し得る。
- ログ: downstream executor / agent / event に依存。
- エラー契約: Worker error / Generate error / unsupported route を success にしない。
- 差し替え可能性: 将来の adapter / application 境界整理候補。
- Phase6 / Phase7 契約との関係: Phase6 の主契約そのものなので Phase8 では method のまま残す。

### event emitter

- collaborator 化する価値: 中。event emission を明示的な port にできる。
- collaborator 化しない場合の理由: nil listener skipped log、event order、Viewer 観測契約に直結するため、Phase8 で切ると影響範囲が大きい。
- 入力: event type、from、to、content、route、jobID、sessionID、channel、chatID。
- 出力: なし。
- 副作用: listener callback、log。
- 永続化: なし。
- ログ: emit / skipped log。
- エラー契約: listener nil は error にしない。
- 差し替え可能性: event port 化は有効だが、Viewer event と execution log を混同しない設計が先。
- Phase6 / Phase7 契約との関係: route chain event order を守る必要があるため Phase8 では method のまま残す。

## 6. Phase8 で collaborator 化してよいもの / method のまま残すもの

### collaborator 化してよいもの

Phase8 では次の順で collaborator 化してよい。

1. `messageResponseAssembler`
   - pure に近く、最も安全。
   - error path で fallback response を作らない契約を固定しやすい。
2. `messageSessionLifecycle`
   - `SessionRepository` への永続化境界が明確。
   - save timing を route execution 成功後に固定する。
3. `routeDecisionCoordinator`
   - Mio route decision と `routing.decision` event の境界を明確化できる。
4. `preRoutingCommandHandler`
   - route decision bypass 契約を明確化できる。
   - response assembler と event emitter を明示的に使う。
5. `idleBusyGuardFactory`
   - nil notifier no-op と defer 解除契約を固定しやすい。
6. `autonomousExecutionCoordinator`
   - Phase8 後半で、route dispatcher を触らずに RunExecutor request 組み立てだけを切り出す。

### まだ method のまま残すもの

Phase8 では次を method のまま残す。

- route dispatcher
  - Phase6 route chain 契約そのものに近い。
  - CODE 系 route、Shiro relay event order、CodeExecutor handoff を同時に動かさない。
- TTS lifecycle
  - 音声、口パク、Viewer event、stream callback が絡む。
  - degraded log を success と混同しない設計を別 Phase で扱う。
- event emitter
  - event order と nil listener skipped log に直結する。
  - Viewer event と execution log の境界を別 Phase で整理する。
- task context builder
  - task / job ID 生成は切り出し可能だが、attachment event と TTS session ID 生成が混ざる。
  - Phase8 では collaborator 化を急がず、必要なら Phase8-6 で Phase9 候補に回す。

## 7. 提案する collaborator 構成

### `messageResponseAssembler`

- 種別: private struct。
- interface 化: 初期段階ではしない。
- `MessageOrchestrator` field: 持たせる。
- constructor: `NewMessageOrchestrator` 内で zero dependency として組み立てる。
- dependency: なし。
- test double: 不要。直接 unit test で十分。
- 主な method:
  - `Build(response string, decision routing.Decision, jobID task.JobID) ProcessMessageResponse`
  - `BuildChatCommand(response string) ProcessMessageResponse`

### `messageSessionLifecycle`

- 種別: private struct。
- interface 化: 初期段階ではしない。外部差し替えが必要になった時点で interface 化する。
- `MessageOrchestrator` field: 持たせる。
- constructor: `newMessageSessionLifecycle(sessionRepo SessionRepository)`。
- dependency: `SessionRepository`。
- test double: 既存の mock session repository を使う。
- 主な method:
  - `LoadForRequest(ctx context.Context, req ProcessMessageRequest) (*session.Session, error)`
  - `SaveCompletedTask(ctx context.Context, sess *session.Session, t task.Task) error`

### `preRoutingCommandHandler`

- 種別: private struct。
- interface 化: 初期段階ではしない。
- `MessageOrchestrator` field: 持たせる。
- constructor: `newPreRoutingCommandHandler(mio MioAgent, emitter messageEventEmitter, responses messageResponseAssembler)`。
- dependency: `MioAgent`、event emit function または小さな emitter interface、`messageResponseAssembler`。
- test double: MioAgent と emitter の recording double が有用。
- 主な method:
  - `Handle(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, bool, error)`

### `routeDecisionCoordinator`

- 種別: private struct。
- interface 化: 初期段階ではしない。
- `MessageOrchestrator` field: 持たせる。
- constructor: `newRouteDecisionCoordinator(mio MioAgent, emitter messageEventEmitter)`。
- dependency: `MioAgent`、event emit function または小さな emitter interface。
- test double: MioAgent と emitter の recording double が有用。
- 主な method:
  - `Decide(ctx context.Context, t task.Task, req ProcessMessageRequest, jobID task.JobID) (routing.Decision, error)`

### `idleBusyGuardFactory`

- 種別: private struct。
- interface 化: しない。
- `MessageOrchestrator` field: 持たせる。
- constructor: `newIdleBusyGuardFactory(idleNotifier IdleNotifier)`。
- dependency: `IdleNotifier`。
- test double: IdleNotifier の recording double が有用。
- 主な method:
  - `BeginChat() func()`
  - `BeginWorker(route routing.Route) func()`

### `autonomousExecutionCoordinator`

- 種別: private struct。
- interface 化: 初期段階ではしない。ただし route direct executor は function type で注入する。
- `MessageOrchestrator` field: 持たせる。
- constructor: `newAutonomousExecutionCoordinator(reporter ReportStore, maxRepair func() int, emitter messageEventEmitter, executeDirect autonomousRouteExecutor)`。
- dependency: `ReportStore`、maxRepair provider、event emitter、route direct executor。
- test double: route direct executor と emitter の recording double が必要。
- 主な method:
  - `Execute(ctx context.Context, t task.Task, route routing.Route, sessionID, channel, chatID, ttsSessionID string) (string, error)`

### field と setter の扱い

`MessageOrchestrator` の setter が後から依存を差し替えるため、collaborator が保持する依存も setter で更新される必要がある。

- `SetEventListener` 後、event emitter を使う collaborator が最新 listener を使えること。
- `SetIdleNotifier` 後、`idleBusyGuardFactory` が最新 notifier を使えること。
- `SetReportStore` 後、`autonomousExecutionCoordinator` が最新 reporter を使えること。
- `SetTTSBridge` / `SetVTuberBridge` は Phase8 では TTS lifecycle を method のまま残すため、現状維持する。

実装時は、constructor で collaborator を作るだけでなく、setter 後の dependency 更新経路を明記してから変更する。

## 8. 小 Phase 案

### Phase8-0: collaborator 化対象の現在契約固定

- 目的: 変更前の契約をテストと文書で固定する。
- 対象範囲: response、session、pre-routing command、route decision、idle busy、autonomous execution の既存テスト確認。
- 対象外: production code の構造変更。
- 実装手順:
  - baseline test を実行する。
  - 既存テストで不足する観点を確認する。
  - 不足が小さい場合のみ、collaborator 化前の契約テストを追加する。
- 検証手順:
  - `go test` 対象パッケージ一式。
  - session / response focused test。
  - route chain focused test。
- 完了条件:
  - response / session / command / routing / idle の既存契約が確認できている。
  - Phase8-1 へ進む判断材料がある。

### Phase8-1: response assembler collaborator 化

- 目的: response assembly を `messageResponseAssembler` へ移す。
- 対象範囲: `message_orchestrator_response.go`、`ProcessMessage` の response assembly 呼び出し。
- 対象外: route execution、session、event、TTS。
- 実装手順:
  - `messageResponseAssembler` を private struct として追加する。
  - `Build` と `BuildChatCommand` に既存 free function の処理を移す。
  - `MessageOrchestrator` に field を追加し、constructor で初期化する。
  - 呼び出しを collaborator 経由に置き換える。
- 検証手順:
  - session / response focused test。
  - 全体 after test。
- 完了条件:
  - response の route / confidence / jobID 契約が変わっていない。
  - error path で fallback response を作っていない。

### Phase8-2: session lifecycle collaborator 化

- 目的: session load / create / save を `messageSessionLifecycle` へ移す。
- 対象範囲: `message_orchestrator_session.go`、`ProcessMessage` の session lifecycle 呼び出し。
- 対象外: route decision、route execution、response assembly の追加変更。
- 実装手順:
  - `messageSessionLifecycle` を private struct として追加する。
  - `SessionRepository` を collaborator に渡す。
  - load / create / save の既存 log と error wrap を維持する。
  - `MessageOrchestrator` から collaborator を呼ぶ。
- 検証手順:
  - session / response focused test。
  - 全体 after test。
- 完了条件:
  - `ErrSessionNotFound` のみ新規 session になる。
  - save timing が route execution 成功後のまま。
  - load / save error の wrap 文言が維持されている。

### Phase8-3: route decision coordinator / pre-routing command handler collaborator 化

- 目的: route decision 前後の境界を明確にする。
- 対象範囲: `message_orchestrator_commands.go`、`message_orchestrator_routing.go`。
- 対象外: route dispatcher、CodeExecutor、WorkerExecutionService。
- 実装手順:
  - `preRoutingCommandHandler` を private struct として追加する。
  - `routeDecisionCoordinator` を private struct として追加する。
  - event emit は既存 `emit` を経由する小さな interface または function type で渡す。
  - handled command bypass と `routing.decision` event order を維持する。
- 検証手順:
  - route chain focused test。
  - session / response focused test。
  - 全体 after test。
- 完了条件:
  - handled command が route decision を bypass する。
  - route decision event が dispatch より前に出る。
  - routing error は success response にならない。

### Phase8-4: idle busy guard collaborator 化

- 目的: IdleChat busy state の guard を `idleBusyGuardFactory` へ移す。
- 対象範囲: `message_orchestrator_idle.go`、`ProcessMessage` の busy guard 呼び出し。
- 対象外: IdleChat 本体、Viewer、TTS。
- 実装手順:
  - `idleBusyGuardFactory` を private struct として追加する。
  - nil notifier は no-op closure を返す。
  - `CHAT` 以外でだけ worker busy を立てる契約を維持する。
  - `SetIdleNotifier` が collaborator に反映されるようにする。
- 検証手順:
  - idle notifier 周辺の既存テスト。
  - 全体 after test。
- 完了条件:
  - error path でも `defer` 解除できる。
  - `CHAT` route で worker busy が立たない。
  - nil notifier で panic しない。

### Phase8-5: autonomous execution coordinator collaborator 化

- 目的: RunExecutor request、retry、verify の組み立てを `autonomousExecutionCoordinator` へ移す。
- 対象範囲: `message_orchestrator_autonomous.go` の `executeAutonomousTask` 相当。
- 対象外: `executeRouteDirect` の分割、route-specific execution の変更、CodeExecutor の変更。
- 実装手順:
  - route direct executor を function type として collaborator に渡す。
  - event emitter と reporter を collaborator に渡す。
  - maxRepair は value 固定ではなく provider function で参照する。
  - retry message、verify、failure classification の既存挙動を維持する。
- 検証手順:
  - route chain focused test。
  - autonomous execution に関係する既存テスト。
  - 全体 after test。
- 完了条件:
  - unsupported route は error になる。
  - Worker error / Generate error / verify failure が success response にならない。
  - route dispatcher は method のまま維持されている。

### Phase8-6: 完了判定と Phase9 判断

- 目的: Phase8 の collaborator 化が過剰分割になっていないか判定し、Phase9 の対象を決める。
- 対象範囲: Phase8 で追加した collaborator、`MessageOrchestrator` の field / constructor / setter、テスト結果、差分。
- 対象外: Phase9 実装。
- 実装手順:
  - collaborator ごとに入力、出力、副作用、永続化、ログ、エラー契約を再確認する。
  - `MessageOrchestrator` constructor が巨大化していないか確認する。
  - route dispatcher、TTS lifecycle、event emitter、task context builder を Phase9 に回すか判断する。
  - `docs/refactor/Phase8_完了判定.md` を作成する。
- 検証手順:
  - 全体 after test。
  - route chain focused test。
  - TTS focused test。
  - `git diff --check`。
- 完了条件:
  - `MessageOrchestrator` が top-level orchestration として説明できる。
  - collaborator 化した単位が巨大 helper になっていない。
  - Phase6 / Phase7 契約が維持されている。
  - Phase9 に進むか判断できる。

## 9. テスト方針

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

route chain に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

session / response に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
```

TTS lifecycle に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

live health は原則不要。ただし server startup / runtime config に触った場合のみ実行する。

```bash
curl -fsS http://127.0.0.1:18790/health
```

Phase8 では handler、Viewer、IdleChat、STT / TTS provider、LLM provider、runtime config の挙動を変更しないため、通常は live health やブラウザ確認は要求しない。

## 10. リスク

Phase8 の主なリスクは次の通り。

- collaborator 化で依存注入が増えすぎる。
- 小さい collaborator が多すぎて逆に読みにくくなる。
- collaborator 名が抽象的すぎて責務が曖昧になる。
- `MessageOrchestrator` constructor が巨大化する。
- setter 後の dependency 更新漏れで collaborator が古い依存を参照する。
- Phase6 route chain event order を壊す。
- session save / response assembly / route decision の順序を変える。
- TTS degraded を success と混同する。
- idle busy guard の defer 解除が漏れる。
- autonomous executor の retry / verify flow と route dispatch を混同する。
- Worker error / Generate error を success として扱う。
- Viewer event / execution log / response text を混同する。
- distributed orchestrator へ同時に広げて Phase8 の範囲を超える。
- task context builder を急いで切り出し、job ID、attachment event、TTS session ID の対応を崩す。

## 11. 完了条件

Phase8 の完了条件は次の通り。

- `docs/refactor/Phase8_MessageOrchestrator_collaborator化実装仕様.md` が作成されている。
- Phase7 後の現在構造が棚卸しされている。
- collaborator 化候補が比較されている。
- collaborator 化するものと、まだ method のまま残すものが明記されている。
- 各 collaborator の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 小 Phase 案が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- 未追跡の `tests/` は触っていない。
- ユーザーが次に Phase8 を実装してよいか判断できる。
