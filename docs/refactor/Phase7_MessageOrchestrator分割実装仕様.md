# Phase7 MessageOrchestrator 分割実装仕様

## Phase7 の目的

Phase7 は、Phase6 で固定した route chain 契約を守ったまま、`MessageOrchestrator` を段階的に薄くするための実装計画である。

目的は次の通り。

- `MessageOrchestrator` を段階的に薄くする。
- `ProcessMessage` の責務を棚卸しし、分割順を決める。
- Phase6 の route chain 契約を維持する。
- 挙動変更ではなく、責務分離と移行計画を目的にする。
- モジュール化と疎結合を最重要方針として、単なる helper 化や巨大 manager 化を避ける。

Phase7 では、`ProcessMessage` を一度に大きく分割しない。session、event、pre-routing command、route decision、TTS、idle busy guard、autonomous execution、response assembly の境界を小 Phase に分けて整理する。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

正本仕様で守るべき責務境界は次の通り。

- Chat はユーザー対話、route 判断、結果返却を担当する。
- Worker は実行主体として file edit / shell / git / test execution を担当する。
- Coder は plan / patch / proposal / Generate を担当し、破壊的操作を直接実行しない。

Phase7 では、この責務境界を変更しない。`docs/codebase-map/` は `MessageOrchestrator` の結合点、ユースケース、潜在バグを確認する一次解析資料として使うが、正本仕様ではない。矛盾がある場合は、正本仕様と現在コードを優先する。`docs/archive/` は一次参照にしない。

## Phase6 契約との関係

Phase6 で固定した契約は Phase7 の前提である。

- CODE 系 route は `executeCodeViaShiro` から `CodeExecutor.ExecuteCode` へ委譲される。
- generic `CODE` と explicit `CODE1` / `CODE2` / `CODE3` / `CODE4` の Shiro 経由 event order を維持する。
- `CODE3` proposal path の event order を維持する。
- proposal interface 非対応 Coder は Generate path に戻る。
- nil / invalid proposal は Worker に渡らない。
- valid proposal だけ WorkerExecutionService に渡る。
- Worker error は success に変換しない。
- Generate error は fallback success に変換しない。
- degraded notice と `Handled` を success と混同しない。
- Viewer event は execution log の代替にしない。

Phase7 で route dispatch や autonomous execution の境界を整理する場合も、これらの契約を壊してはいけない。

## docs/codebase-map から見た注意点

`docs/codebase-map/` では、`MessageOrchestrator` は主要結合点として整理されている。

- Mio route、Shiro / Worker / Coder / Wild / Heavy、session、events、TTS / VTuber を統合する。
- route 追加や event 追加の影響範囲が広い。
- Code execution chain は Coder proposal から `WorkerExecutionService` へ進む。
- Viewer observation chain は application event、EventHub / SSE、JS rendering、event log / history が連動する。
- fallback 応答は成功ではなく、エラー経路の可視化として扱う必要がある。

このため Phase7 は「分割して薄くする」ことが目的だが、Viewer event、execution log、response text、TTS / VTuber の流れを混同しないように進める。

## 対象範囲

Phase7 の対象は次に限定する。

- `MessageOrchestrator.ProcessMessage`
- session load / create / save
- `message.received` event
- pre-routing chat command
- task / job / TTS session ID assembly
- route decision
- TTS session start / end
- idle notifier busy state
- `executeTask`
- `executeAutonomousTask`
- route-specific execution entrypoint
- response assembly
- event emission helper
- current tests around ProcessMessage and route chain

確認対象ファイル:

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/message_orchestrator_*test.go`
- `internal/application/orchestrator/code_executor*.go`
- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/service/worker_execution_service.go`
- `internal/application/service/worker_execution_service_test.go`
- `internal/domain/routing/`
- `internal/domain/task/`
- `internal/domain/session/`

## 対象外

Phase7 では次を変更しない。

- Phase6 で固定した route chain 契約。
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

## 現在の ProcessMessage 責務棚卸し

### session load / create

`ProcessMessage` は最初に `loadSessionForRequest` を呼ぶ。内部では `loadOrCreateSession` が session repository から session を読む。`session.ErrSessionNotFound` の場合は新規 session を作る。

責務:

- request の `SessionID` / `Channel` / `ChatID` に対応する session を取得する。
- session がない場合は作成する。
- load 失敗を `failed to load or create session` として wrap する。
- session ID を log に残す。

分割時の注意:

- session 作成と save の責務を混ぜない。
- session repository の interface や domain session の意味を変えない。
- load error を fallback response に変換しない。

### message received event

`emitMessageReceived` は `message.received` event を user -> mio として emit する。

責務:

- request 入力を Viewer-facing event として通知する。
- route はまだ未決定なので空文字のままにする。

分割時の注意:

- raw input event と response event を混同しない。
- event がないことを session save や route decision の代替にしない。

### pre-routing chat command

`handlePreRoutingChatCommand` は Mio の `HandleChatCommand` を route decision より前に実行する。

責務:

- 明示的な chat command を route decision 前に処理する。
- handled の場合は `CHAT` response として返す。
- handled の場合は route decision を bypass する。
- command error は `chat command failed` として返す。

分割時の注意:

- command handled path で `routing.decision` event を出さない。
- command handled path を通常 chat route execution と混同しない。
- `Handled` は chat command 用の別契約であり、CodeExecutionResponse の `Handled` と混同しない。

### task / job / TTS session ID assembly

`buildTaskForRequest` は `task.JobID` を生成し、`task.Task` を作り、attachments を引き継ぎ、TTS session ID を組み立てる。

責務:

- job ID を生成する。
- user message、channel、chat ID から task を作る。
- attachments を task に持たせる。
- attachment event を emit する。
- TTS bridge がある場合のみ TTS session ID を作る。

分割時の注意:

- job ID と response JobID をずらさない。
- attachment event と message received event を混同しない。
- TTS session ID は TTS bridge がある場合だけ作る。

### Mio route decision

`decideRouteForTask` は Mio の `DecideAction` を呼び、`routing.decision` event を emit する。

責務:

- Mio に route 判断を委譲する。
- confidence と route を event に残す。
- routing decision error を `routing decision failed` として返す。

分割時の注意:

- route decision と route execution を同じ collaborator に混ぜない。
- routing decision event の順序を変えない。
- route decision 失敗を fallback success にしない。

### TTS session start / end

`startTTSSessionForRoute` は route 決定後に TTS session を開始し、`endTTSSession` は route execution 後に終了する。

責務:

- route に応じた TTS context、speaker、voice、speech mode を決める。
- TTS bridge がない場合は何もしない。
- start / end error は degraded log として扱い、route execution 自体は継続する。

分割時の注意:

- TTS degraded log を route success と混同しない。
- TTS start 条件と end 条件を変えない。
- 音声、口パク、Viewer 表示本文を混同しない。

### idle notifier busy state

`ProcessMessage` は開始時に activity と chat busy を通知する。route が `CHAT` 以外の場合は worker busy を立て、defer で戻す。

責務:

- user activity を IdleChat 側へ通知する。
- chat processing 中の busy state を立てる。
- autonomous route の worker busy state を立てる。
- defer によって解除漏れを防ぐ。

分割時の注意:

- worker busy は `CHAT` route では立てない。
- error path でも busy state を解除する。
- IdleChat の契約や endpoint は変更しない。

### route execution への委譲

`executeTask` は `CHAT` と autonomous route を分ける。`CHAT` は `executeChatRoute`、それ以外は `executeAutonomousTask` に進む。

責務:

- top-level route execution を切り替える。
- `CHAT` と autonomous route の入口を分ける。

分割時の注意:

- `CHAT` を autonomous executor に通さない。
- autonomous route を `CHAT` fallback として成功扱いしない。

### autonomous executor retry / verify flow との境界

`executeAutonomousTask` は route が autonomous route か確認し、contract を normalize し、`autonomous.RunExecutor` に `Execute` / `Verify` callback を渡す。

責務:

- unsupported autonomous route を error にする。
- route-specific execution を `executeRouteDirect` に委譲する。
- retry 時は user message を retry message に差し替える。
- `verifyByContract` で route に応じた検証を行う。
- report store と entry stage event をつなぐ。

分割時の注意:

- retry により route-specific execution が複数回呼ばれる可能性を success / failure 判定と混同しない。
- `executeRouteDirect` の route chain 契約を壊さない。
- `RunExecutor` の apply / verify error を fallback success にしない。

### response assembly

`buildProcessMessageResponse` は response、route、confidence、job ID を `ProcessMessageResponse` にまとめる。`buildChatCommandResponse` は command handled path 用の `CHAT` response を作る。

責務:

- route decision の route / confidence と response text を組み合わせる。
- job ID を response に入れる。
- chat command handled path は別の response assembly を使う。

分割時の注意:

- normal route response と chat command response の JobID 契約を混同しない。
- response text と Viewer event content を同じものとして扱いすぎない。
- error path で fallback response を組み立てない。

### session save

`saveCompletedTask` は completed task を session history に追加し、repository に保存する。

責務:

- task を session history に追加する。
- session repository に保存する。
- save error を `failed to save session` として返す。

分割時の注意:

- route execution が成功した後に save する順序を変えない。
- error path で completed task として保存しない。
- session save と execution log / evidence を混同しない。

### error wrapping

`ProcessMessage` は各段階の error を wrap して返す。

責務:

- session load / route decision / command / task execution / session save の error context を残す。
- task execution error は `task execution failed` として返す。

分割時の注意:

- Worker error / Generate error を success response に変換しない。
- fallback を正常系として扱わない。
- error context を削りすぎない。

## 分割候補と順序

Phase7 では次の順で扱う。

| 順序 | 候補 | 理由 | 初期移動先案 |
|---|---|---|---|
| 1 | session lifecycle helper | load / create / save は副作用が明確で、route chain から切り離しやすい | `message_orchestrator_session.go` |
| 2 | response assembler | pure に近く、session lifecycle と合わせて完了条件を固定しやすい | `message_orchestrator_response.go` |
| 3 | pre-routing command handler | route decision を bypass する重要契約なので先に境界を明示する | `message_orchestrator_commands.go` |
| 4 | route decision coordinator | Mio route 判断と `routing.decision` event の境界を固定する | `message_orchestrator_routing.go` |
| 5 | task context builder | job ID、task、attachment event、TTS session ID の入力出力を明確にする | `message_orchestrator_task.go` |
| 6 | TTS lifecycle helper | 音声、口パク、Viewer 表示を混同しないため専用境界にする | `message_orchestrator_tts_lifecycle.go` |
| 7 | idle busy guard | defer による解除漏れ防止の契約を明示する | `message_orchestrator_idle.go` |
| 8 | autonomous execution coordinator | `RunExecutor` callback と route-specific execution の境界を明確にする | `message_orchestrator_autonomous.go` |
| 9 | route dispatch file / coordinator | Phase6 route chain 契約を守りながら最後に移す | `message_orchestrator_routes.go` |
| 10 | event emission helper | event は全体に関わるため、最後に薄く整理する | `message_orchestrator_events.go` |

event emission helper は先に分けると全差分に波及しやすい。Phase7 では、まず `emit` の挙動を変えず、必要に応じて最後にファイル分離だけを検討する。

## MessageOrchestrator に残すもの / 移すもの

### 残してよいもの

- top-level orchestration。
- 主要 dependency の保持。
- route chain の大枠。
- 分割した collaborator / helper の呼び出し。
- public setter と constructor。
- Phase6 で固定した CodeExecutor handoff の入口。

### 減らしたいもの

- 長い session lifecycle 詳細。
- 長い TTS lifecycle 詳細。
- autonomous executor の request 組み立て詳細。
- route-specific execution の細部。
- response assembly の細部。
- error / event / log の局所的な重複。
- retry / verify flow の callback 詳細。

## 分割単位ごとの契約

### session lifecycle helper

- 入力: `context.Context`、`ProcessMessageRequest`、`session.Session`、`task.Task`。
- 出力: loaded / created session、または error。
- 副作用: session repository read / write、session history 追加。
- 永続化: `SessionRepository.Save`。
- ログ: load / create 成功、load / save error。
- エラー契約: load error は `failed to load or create session`、save error は `failed to save session`。
- 変更してはいけない既存挙動: `session.ErrSessionNotFound` の場合だけ新規 session を作る。route execution 成功後だけ completed task として保存する。
- Phase6 契約との関係: Worker / Generate error 時に success として session save しない。

### response assembler

- 入力: response text、`routing.Decision`、`task.JobID`、chat command response text。
- 出力: `ProcessMessageResponse`。
- 副作用: なし。
- 永続化: なし。
- ログ: なし。
- エラー契約: なし。
- 変更してはいけない既存挙動: normal route response は route decision の route / confidence / jobID を使う。chat command response は route `CHAT`、confidence `1.0` とする。
- Phase6 契約との関係: Worker error / Generate error 時に fallback response を生成しない。

### pre-routing command handler

- 入力: `context.Context`、`ProcessMessageRequest`、Mio agent。
- 出力: handled response、handled flag、error。
- 副作用: handled 時に `agent.response` event を emit。
- 永続化: なし。
- ログ: command error は caller で error context を保持する。
- エラー契約: `chat command failed` として返す。
- 変更してはいけない既存挙動: handled command は route decision を bypass する。
- Phase6 契約との関係: route chain event order を壊さない。`routing.decision` event を出さない。

### task context builder

- 入力: `ProcessMessageRequest`、TTS bridge の有無。
- 出力: `task.Task`、`task.JobID`、TTS session ID。
- 副作用: attachment event の emit。
- 永続化: なし。
- ログ: 原則なし。
- エラー契約: 現状 error は返さない。
- 変更してはいけない既存挙動: attachments を task に引き継ぐ。TTS bridge がある場合だけ TTS session ID を作る。
- Phase6 契約との関係: CodeExecutor に渡す `req.Task.JobID()` と response JobID をずらさない。

### route decision coordinator

- 入力: `context.Context`、`task.Task`、`ProcessMessageRequest`、`task.JobID`、Mio agent。
- 出力: `routing.Decision`。
- 副作用: `routing.decision` event の emit。
- 永続化: なし。
- ログ: route decision の event content に confidence を含める。
- エラー契約: `routing decision failed` として返す。
- 変更してはいけない既存挙動: pre-routing command 後にだけ実行する。event の route は decision route にする。
- Phase6 契約との関係: CODE 系 route の dispatch 先を変えない。

### TTS lifecycle helper

- 入力: `context.Context`、`ProcessMessageRequest`、`task.JobID`、`routing.Decision`、TTS session ID、route、text。
- 出力: なし。
- 副作用: TTS bridge start / end / push、VTuber push。
- 永続化: なし。
- ログ: TTS / VTuber degraded log。
- エラー契約: TTS start / end / push error は degraded log として扱い、route execution error にしない。
- 変更してはいけない既存挙動: TTS bridge がない場合は何もしない。route に応じた speaker / voice / speech mode を維持する。
- Phase6 契約との関係: degraded を success と混同しない。音声 chunk を本文表示の唯一根拠にしない。

### idle busy guard

- 入力: idle notifier、route。
- 出力: cleanup function または guard object。
- 副作用: activity notification、chat busy / worker busy state 更新。
- 永続化: なし。
- ログ: 原則なし。
- エラー契約: 現状 error は返さない。
- 変更してはいけない既存挙動: `NotifyActivity`、`SetChatBusy(true)`、defer 解除を維持する。worker busy は `CHAT` 以外だけ。
- Phase6 契約との関係: error path でも busy state を解除する。

### autonomous execution coordinator

- 入力: `context.Context`、`task.Task`、route、session / channel / chat ID、TTS session ID、report store、max repair。
- 出力: response text、error。
- 副作用: entry stage event、report store save、route-specific execution callback。
- 永続化: execution report store。
- ログ: autonomous executor 側の stage / failure log。
- エラー契約: unsupported route、contract normalize error、apply / verify error を返す。fallback success にしない。
- 変更してはいけない既存挙動: retry 時だけ retry message を作る。`verifyByContract` を通す。`executeRouteDirect` に route-specific execution を委譲する。
- Phase6 契約との関係: Worker error / Generate error を success に変換しない。retry による複数回実行を success 判定と混同しない。

### route dispatch file / coordinator

- 入力: `context.Context`、`task.Task`、route、session / channel / chat ID、TTS session ID。
- 出力: response text、error。
- 副作用: agent event、TTS / VTuber stream / push。
- 永続化: なし。
- ログ: route-specific execution log は既存のまま維持する。
- エラー契約: unsupported route、missing agent、agent error を返す。
- 変更してはいけない既存挙動: CODE 系 route は `executeCodeViaShiro` を通る。CHAT は autonomous executor を通らない。
- Phase6 契約との関係: Shiro relay event order と CodeExecutor handoff を維持する。

### event emission helper

- 入力: event type、from、to、content、route、job ID、session ID、channel、chat ID。
- 出力: なし。
- 副作用: listener への event delivery、log。
- 永続化: なし。
- ログ: listener なしの場合の skipped log、emit log。
- エラー契約: 現状 error は返さない。
- 変更してはいけない既存挙動: listener が nil の場合は panic せず log だけ出す。
- Phase6 契約との関係: Viewer event、execution log、response text を混同しない。

## 小 Phase 案

### Phase7-0: ProcessMessage 現在責務棚卸しと baseline 固定

目的:

- `ProcessMessage` と周辺 helper の現在責務を docs に固定する。
- baseline test を実行し、以降の分割の基準にする。

対象範囲:

- `message_orchestrator.go`
- `message_orchestrator_*test.go`
- Phase6 契約テスト

対象外:

- production code 変更。

実装手順:

1. baseline test を実行する。
2. `ProcessMessage` と helper 一覧を `rg` で確認する。
3. 現在の責務と移動候補を Phase7 docs に記録する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

完了条件:

- baseline test が成功している。
- 現在責務が文書化されている。
- コード変更をしていない。

### Phase7-1: session lifecycle / response assembly 境界整理

目的:

- session load / create / save と response assembly を専用ファイルへ分ける。

対象範囲:

- `loadSessionForRequest`
- `loadOrCreateSession`
- `saveCompletedTask`
- `buildProcessMessageResponse`
- `buildChatCommandResponse`

対象外:

- route decision。
- route dispatch。
- TTS lifecycle。

実装手順:

1. baseline test を実行する。
2. `message_orchestrator_session.go` を作成し、session lifecycle 関数を移す。
3. `message_orchestrator_response.go` を作成し、response assembly 関数を移す。
4. 関数本体と呼び出し順は変えない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:

- session lifecycle と response assembly がファイル責務として分離されている。
- `ProcessMessage` の実行順が変わっていない。
- session save error / load error の既存テストが通っている。

### Phase7-2: pre-routing command / route decision 境界整理

目的:

- route decision 前の command handled path と Mio route decision を別責務として明確にする。

対象範囲:

- `handlePreRoutingChatCommand`
- `decideRouteForTask`

対象外:

- Mio agent の実装。
- route dispatch。
- command 辞書の意味変更。

実装手順:

1. baseline test を実行する。
2. `message_orchestrator_commands.go` を作成し、pre-routing command 関数を移す。
3. `message_orchestrator_routing.go` を作成し、route decision 関数を移す。
4. handled command が route decision を bypass する契約を維持する。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_RouteChainContract_|TestMessageOrchestrator_ProcessMessage_ChatCommand|TestMessageOrchestrator_ProcessMessage_RoutingError'
git diff --check
```

完了条件:

- pre-routing command と route decision が別ファイル責務になっている。
- handled command で `routing.decision` event が出ない契約が維持されている。
- routing decision event order が維持されている。

### Phase7-3: TTS lifecycle / idle busy guard 境界整理

目的:

- TTS session start / end / push と IdleChat busy state の責務を `ProcessMessage` から分ける。

対象範囲:

- `startTTSSessionForRoute`
- `endTTSSession`
- `pushTTS`
- `withStreamHooks`
- idle notifier busy state block

対象外:

- TTS provider。
- VTuber provider。
- IdleChat endpoint。
- Viewer JS / CSS。

実装手順:

1. baseline test を実行する。
2. `message_orchestrator_tts_lifecycle.go` を作成し、TTS lifecycle 関数を移す。
3. `message_orchestrator_idle.go` を作成し、busy guard を関数化または小さい guard として切り出す。
4. TTS degraded log の扱いを変えない。
5. busy state の defer 解除を維持する。
6. gofmt を実行する。
7. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
git diff --check
```

完了条件:

- TTS lifecycle が専用責務として分離されている。
- TTS start error が degraded として継続される。
- busy state の解除漏れがない。

### Phase7-4: autonomous execution coordinator 境界整理

目的:

- `executeAutonomousTask` の `RunExecutor` request 組み立て、retry、verify callback の責務を明確にする。

対象範囲:

- `executeAutonomousTask`
- `buildExecutorRetryMessage`
- `routeExecutionSteps`
- `classifyExecutorFailure`
- `verifyByContract`

対象外:

- autonomous executor の内部実装。
- CodeExecutor。
- WorkerExecutionService。

実装手順:

1. baseline test を実行する。
2. `message_orchestrator_autonomous.go` を作成し、autonomous execution 関数群を移す。
3. callback の入力、出力、副作用を変えない。
4. retry message 生成条件を変えない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_RouteChainContract_|TestVerifyByContract|TestMessageOrchestrator_ProcessMessage_CODE3'
git diff --check
```

完了条件:

- autonomous execution の組み立て責務が専用ファイルに分かれている。
- apply / verify error が fallback success にならない。
- Phase6 の Worker / Generate error 契約が維持されている。

### Phase7-5: route dispatch entrypoint 境界整理

目的:

- route-specific execution entrypoint を専用ファイルへ分け、`ProcessMessage` から route dispatch の細部を減らす。

対象範囲:

- `executeTask`
- `executeRouteDirect`
- `executeChatRoute`
- `executeOPSRoute`
- `executeCodeRoute`
- `executeWildRoute`
- `executePlanRoute`
- `executeAnalyzeRoute`
- `executeResearchRoute`
- `executeCodeViaShiro`

対象外:

- CodeExecutor selection。
- proposal path。
- Generate path。
- WorkerExecutionService。

実装手順:

1. baseline test を実行する。
2. `message_orchestrator_routes.go` を作成し、route dispatch 関数群を移す。
3. route switch の順序と case を変えない。
4. CODE 系 route が `executeCodeViaShiro` を通る契約を維持する。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestMessageOrchestrator_ProcessMessage_|TestProcessMessage_Route'
git diff --check
```

完了条件:

- route dispatch entrypoint が専用ファイルに分離されている。
- CODE 系 route の Shiro event order が維持されている。
- CHAT / OPS / PLAN / ANALYZE / RESEARCH / WILD の既存契約が維持されている。

### Phase7-6: 完了判定と Phase8 判断

目的:

- Phase7 の分割結果を文書化し、Phase8 へ進むか判断できる状態にする。

対象範囲:

- Phase7 docs。
- Phase7 test diff。
- final verification。

対象外:

- Phase8 以降の実装。

実装手順:

1. 全体 test を実行する。
2. `git diff --check` を実行する。
3. `git status --short` を確認する。
4. `docs/refactor/Phase7_完了判定.md` を作成する。
5. Phase8 候補を短く整理する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
```

完了条件:

- `MessageOrchestrator` が top-level orchestration として説明できる。
- session、response、command、routing、TTS、idle、autonomous、route dispatch の責務が分離されている。
- Phase6 route chain 契約が維持されている。
- production behavior の意図的な変更がない。
- Phase7 docs / implementation diff が push 済みである。

## テスト方針

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

TTS lifecycle に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

session / response assembly に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_ProcessMessage_(NewSession|ExistingSession|TaskAddedToHistory|SessionLoadError|SessionSaveError|ChatCommand_Handled)'
```

live health は原則不要。ただし server startup / runtime config に触った場合のみ実行する。

```bash
curl -fsS http://127.0.0.1:18790/health
```

Viewer / IdleChat / STT / TTS の handler 中身を変えた場合は実ブラウザまたは同等の E2E 確認が必要である。ただし Phase7 では原則として handler 中身を変更しない。

## リスク

- `ProcessMessage` 分割で route chain の順序を変える。
- session save / event emission / response assembly の順序を変える。
- TTS session start / end の条件を崩す。
- idle notifier busy state の解除漏れを起こす。
- autonomous executor の retry / verify flow と route-specific execution を混同する。
- CodeExecutor handoff を崩す。
- Worker error / Generate error を success として扱う。
- `Handled` を final success 判定に使ってしまう。
- Viewer event / execution log / response text を混同する。
- helper 化だけで責務境界が曖昧になる。
- 巨大な coordinator / manager を作る。
- distributed orchestrator まで同時に広げて Phase7 の範囲を超える。

## 完了条件

- `docs/refactor/Phase7_MessageOrchestrator分割実装仕様.md` が作成されている。
- `ProcessMessage` の現在責務が棚卸しされている。
- 分割候補と順序が明記されている。
- 各分割候補の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- Phase6 契約を維持する方針が明記されている。
- 小 Phase 案が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- ユーザーが次に「Phase7 を実装してよいか」を判断できる。

## 次に確認すべきこと

Phase7 実装に入る前に、次を確認する。

1. Phase7-1 は session lifecycle / response assembly のファイル分離から始めてよいか。
2. route dispatch の分離は Phase7-5 まで待ち、Phase6 route chain 契約を壊さない順序で進めてよいか。
3. distributed orchestrator は Phase7 の対象外として、必要なら別 Phase で扱う方針でよいか。
