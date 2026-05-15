# Phase2 route chain 明確化実装仕様

## Phase 2 の目的

Phase 2 では、`MessageOrchestrator` を中心に Chat / Worker / Coder の route 判断、dispatch、response assembly の流れを明確にする。

目的は次の通り。

- `MessageOrchestrator.ProcessMessage` の責務を段階的に読みやすくする。
- route 判断、route dispatch、response assembly、event emission、reporting の境界を明確にする。
- Chat / Worker / Coder の責務境界をコード上で追いやすくする。
- Coder proposal 生成と Worker 実行の境界を守る。
- fallback を正常成功として扱わず、安全側経路または error 経路として可視化する。
- Viewer event、session、report、TTS hook を落とさない。
- 表示、音声、口パク、ログを混同しない。
- モジュール化と疎結合を最重要方針として、route ごとの入力、出力、event、error contract を固定する。

## 現在の前提

- Phase 1「`cmd/picoclaw` の composition root 整理」は完了済み。
- Phase 1 完了判定は `docs/refactor/Phase1_完了判定.md` に記録済み。
- `cmd/picoclaw/main.go` は composition root として薄くなっている。
- Phase 2 では `internal/application/orchestrator/message_orchestrator.go` と周辺契約を主対象にする。
- 未追跡の `tests/` は対象外として触らない。

## 参照資料

- `AGENTS.md`
- `CLAUDE.md`
- `docs/01_正本仕様/実装仕様.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/段階移行計画.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase1_完了判定.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- `docs/codebase-map/modules/application.md`
- `docs/codebase-map/modules/domain.md`
- `docs/codebase-map/modules/adapter.md`
- `docs/codebase-map/modules/infrastructure.md`
- `docs/codebase-map/modules/潜在バグ一覧.md`
- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/code_executor.go`
- `internal/application/service/worker_execution_service.go`
- `internal/domain/routing/route.go`
- `internal/domain/task/`
- `internal/domain/proposal/`
- `internal/domain/execution/`
- `internal/adapter/viewer/`
- `internal/application/orchestrator/*_test.go`
- `internal/application/service/*_test.go`
- `internal/domain/routing/*_test.go`

## docs/codebase-map/ の使い方

`docs/codebase-map/` は実装前の一次解析資料として使う。

- `docs/codebase-map/アーキテクチャ総合.md` は Chat / Worker / Coder の主要データフロー確認に使う。
- `docs/codebase-map/結合ポイントマップ.md` は `MessageOrchestrator`、`WorkerExecutionService`、Viewer observation chain の結合点確認に使う。
- `docs/codebase-map/modules/application.md` は `MessageOrchestrator.ProcessMessage`、`WorkerExecutionService.ExecuteProposal`、IdleChat / TTS / event の責務把握に使う。
- `docs/codebase-map/modules/潜在バグ一覧.md` は Worker 安全境界、Viewer / IdleChat 表示契約、runtime config 混同リスクの確認に使う。

ただし `docs/codebase-map/` は正本仕様ではない。実装判断で矛盾がある場合は `docs/01_正本仕様/実装仕様.md` と現在コードを優先する。`docs/archive/` は一次参照にしない。

現在コードとの差分リスク:

- codebase-map は `MessageOrchestrator.ProcessMessage` の全 route 分岐を body まで精査していないと記録しているため、Phase 2-0 で現在コードから契約を再固定する。
- Phase 1 により `cmd/picoclaw/main.go` の集中は解消済みであるため、Phase 2 では Application 層の route chain を対象にする。
- live runtime config は codebase-map の静的解析対象外であるため、runtime config の意味変更は Phase 2 の対象外にする。

## 対象範囲

Phase 2 の対象は次である。

- `MessageOrchestrator.ProcessMessage`
- `loadOrCreateSession`
- route 判断前の chat command handling
- `MioAgent.DecideAction`
- task / job ID / session ID の生成と引き継ぎ
- `executeTask`
- `executeAutonomousTask`
- `executeRouteDirect`
- `executeCodeViaShiro`
- `DefaultCodeExecutor.ExecuteCode`
- Coder proposal 生成から `WorkerExecutionService.ExecuteProposal` への handoff
- Viewer event emission
- TTS / VTuber stream hook
- report store への接続
- route ごとの入力、出力、event、error contract

## 対象外

Phase 2 では次を変更しない。

- `WorkerExecutionService` の内部実行方式。
- file edit / shell / git の実行ロジック。
- `ToolRunner` / `PolicyEngine` の実装。
- LLM provider の挙動。
- Viewer JS / CSS。
- STT / TTS provider。
- IdleChat raw / view / audio 契約。
- persistence schema。
- route 名や外部 API 契約の意味。
- runtime config の意味。
- fallback を成功表示にする変更。
- Coder に file edit / command / git 実行責務を戻す変更。

## 現在コードの把握

### `ProcessMessage` の現在の流れ

現在の `MessageOrchestrator.ProcessMessage` は次の順で動く。

1. request を log に記録する。
2. `IdleNotifier` に activity と chat busy を通知する。
3. session を load または create する。
4. `message.received` event を emit する。
5. `MioAgent.HandleChatCommand` を route 判断前に実行する。
6. chat command が処理済みなら `agent.response` を emit し、`RouteCHAT` の response を返す。
7. `task.NewJobID()` で job ID を作る。
8. `task.NewTask(...).WithAttachments(...)` で task を作る。
9. attachment があれば `viewer.attachment.received` を emit する。
10. TTS bridge がある場合は `sessionID-jobID` の TTS session ID を作る。
11. `MioAgent.DecideAction` で route decision を得る。
12. `routing.decision` event を emit する。
13. task に route を設定する。
14. TTS session を開始する。
15. CHAT 以外では `IdleNotifier.SetWorkerBusy(true)` を設定する。
16. `executeTask` へ dispatch する。
17. TTS session を終了する。
18. session に task を追加する。
19. session を保存する。
20. `ProcessMessageResponse` を返す。

### route dispatch の現在の構造

- `executeTask` は `RouteCHAT` 以外を `executeAutonomousTask` へ送る。
- `RouteCHAT` は `MioAgent.Chat` を呼ぶ。
- `executeAutonomousTask` は `contractapp.NormalizeRequestWithRoute` と `autonomousapp.RunExecutor` を使い、`executeRouteDirect` を試行する。
- `executeRouteDirect` は OPS / CODE / WILD / PLAN / ANALYZE / RESEARCH を route ごとに実行する。
- CODE / CODE1 / CODE2 / CODE3 / CODE4 は `executeCodeViaShiro` から `CodeExecutor.ExecuteCode` に委譲する。
- `DefaultCodeExecutor.ExecuteCode` は Coder を選択し、proposal path が可能な場合は `tryExecuteProposalPath` を使う。
- `tryExecuteProposalPath` は Coder proposal を生成し、valid proposal の場合のみ `WorkerExecutionService.ExecuteProposal` に渡す。
- proposal path を使えない場合は Coder の generate path に落ちる。

### 現在の route 定義

`internal/domain/routing/route.go` には次が定義されている。

- `CHAT`
- `PLAN`
- `ANALYZE`
- `OPS`
- `RESEARCH`
- `WILD`
- `CODE`
- `CODE1`
- `CODE2`
- `CODE3`
- `CODE4`

この仕様書ではユーザー指定に合わせて CODE3 までを主要対象にしつつ、現在コードに存在する CODE4 / WILD も破壊しない契約として扱う。

### 既存テストで固定されている契約

既存テストには次の観点がある。

- `internal/application/orchestrator/message_orchestrator_test.go`
  - new / existing session。
  - CHAT / OPS / CODE。
  - TTS start / stream / degraded。
  - routing error / chat error / shiro error。
  - no coder。
  - fallback chain。
  - unknown route。
  - WILD / ANALYZE。
- `internal/application/orchestrator/message_orchestrator_code3_test.go`
  - CODE3 proposal JSON patch / markdown patch。
  - invalid proposal。
  - no coder3。
  - max repair 到達時の error。
- `internal/application/orchestrator/message_orchestrator_code_path_test.go`
  - CODE1 / CODE2 / CODE4 の event 順序。
- `internal/application/orchestrator/code_executor_test.go`
  - explicit coder route。
  - CODE fallback chain。
  - proposal path。
- `internal/application/service/worker_execution_service_test.go`
  - proposal execution。
  - parse error。
  - missing command / protected file / failure classification。
  - sequential / parallel / auto commit。

Phase 2 ではこの既存テストを壊さず、route chain の責務分離に必要な不足テストだけを追加する。

## Phase 2 の分割

Phase 2 は次の小 Phase に分けて進める。

- Phase 2-0: 現状 route chain の契約固定。
- Phase 2-1: `ProcessMessage` の内部ステップ命名。
- Phase 2-2: route ごとの dispatch 関数分割。
- Phase 2-3: response assembly の分離。
- Phase 2-4: Viewer event / report / TTS hook の契約確認。
- Phase 2-5: fallback / error route の扱い固定。
- Phase 2-6: Phase 2 完了判定。

各小 Phase は、計画文書作成、Push、実装、検証、Push の順に進める。

## Phase 2-0: 現状 route chain の契約固定

目的:

- 実装を変える前に、現在の route chain の入力、出力、event、error contract を固定する。
- `ProcessMessage` と `DefaultCodeExecutor` の現状をテストと文書で確認する。

対象範囲:

- `MessageOrchestrator.ProcessMessage`
- `executeTask`
- `executeAutonomousTask`
- `executeRouteDirect`
- `DefaultCodeExecutor.ExecuteCode`
- 既存 `*_test.go`

対象外:

- 実装の分割。
- route 名の変更。
- Worker 実行方式の変更。

入力:

- `ProcessMessageRequest`
- session ID / channel / chat ID / user message / attachments
- `routing.Decision`

出力:

- `ProcessMessageResponse`
- route
- confidence
- job ID
- emitted events

副作用:

- session load / save。
- event emission。
- TTS session start / push / end。
- idle notifier busy state。

永続化:

- session repository。
- autonomous executor 経由の report store。

ログ:

- `[MessageOrch] ProcessMessage START`
- `[MessageOrch] Session loaded/created`
- `[MessageOrch] emit`
- `[MessageOrch] ProcessMessage COMPLETE`
- `[CodeExecutor] code handoff`

event 契約:

- `message.received`
- `viewer.attachment.received`
- `routing.decision`
- `agent.start`
- `agent.thinking`
- `agent.response`
- `agent.notice`
- `entry.stage`

error 契約:

- session load error は `failed to load or create session`。
- chat command error は `chat command failed`。
- route decision error は `routing decision failed`。
- task execution error は `task execution failed`。
- unknown route は error として返す。

変更してはいけない既存挙動:

- chat command は routing 前に処理する。
- route decision 後に `routing.decision` event を出す。
- CODE 系は `CodeExecutor` に委譲する。
- Coder proposal が valid な場合だけ Worker に渡す。

実装手順:

1. 既存テストを確認する。
2. route ごとの現在契約を test table または文書で固定する。
3. 不足する場合は最小テストを追加する。
4. 実装分割はしない。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

完了条件:

- route chain の現状契約が文書化されている。
- 既存テストが成功している。
- 不足する契約テストが追加されている。

## Phase 2-1: `ProcessMessage` の内部ステップ命名

目的:

- `ProcessMessage` の処理順を小さな private function に分け、route chain の読みやすさを上げる。
- 挙動は変えず、手順名をコード上で追えるようにする。

対象範囲:

- `ProcessMessage`
- session load / create
- chat command handling
- task creation
- route decision
- TTS session start / end
- session save

対象外:

- route dispatch の中身。
- `CodeExecutor`。
- `WorkerExecutionService`。

入力:

- `context.Context`
- `ProcessMessageRequest`
- session repository
- Mio agent

出力:

- `ProcessMessageResponse`
- intermediate route context

副作用:

- session load / save。
- event emission。
- idle notifier。
- TTS bridge。

永続化:

- session repository のみ。

ログ:

- 既存ログ文言を維持する。

event 契約:

- event type、from、to、route、jobID、sessionID、channel、chatID を変えない。

error 契約:

- error wrapping の文言を変えない。

変更してはいけない既存挙動:

- `HandleChatCommand` を routing 前に呼ぶ。
- task job ID と response job ID の対応。
- TTS session ID が `sessionID-jobID` であること。
- session save が task execution 後であること。

実装手順:

1. baseline test を実行する。
2. `ProcessMessage` 内の各段階に private helper を作る。
3. helper は同じ file 内に置き、package 外 API を増やさない。
4. route dispatch には踏み込まない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
git diff --stat
```

完了条件:

- `ProcessMessage` が手順を追える長さになっている。
- route dispatch の挙動が変わっていない。
- 既存テストが成功している。

## Phase 2-2: route ごとの dispatch 関数分割

目的:

- route dispatch を責務単位で分ける。
- Chat / Worker / Coder の差を隠さず、各 route の実行先を明確にする。

対象範囲:

- `executeTask`
- `executeAutonomousTask`
- `executeRouteDirect`
- route-specific handler private function

対象外:

- `WorkerExecutionService` の内部。
- Coder selection の品質判定。
- Viewer JS / CSS。

入力:

- `task.Task`
- `routing.Route`
- session ID / channel / chat ID
- TTS session ID

出力:

- route response string
- error

副作用:

- event emission。
- TTS push / stream。
- autonomous report。
- Worker busy state は `ProcessMessage` 側に残す。

永続化:

- autonomous executor report store のみ。

ログ:

- 既存ログを維持する。
- 新規ログを入れる場合は route、jobID、sessionID を含める。

event 契約:

- route ごとの `agent.start` / `agent.response` / `entry.stage` を維持する。

error 契約:

- unknown route は成功扱いしない。
- unsupported autonomous route は error として返す。

変更してはいけない既存挙動:

- CHAT は Mio.Chat。
- OPS は Shiro.Execute。
- CODE 系は `executeCodeViaShiro` から CodeExecutor。
- PLAN / RESEARCH は Mio.Chat。
- ANALYZE は Heavy があれば Heavy、なければ Mio。
- WILD は Wild がなければ error。

実装手順:

1. baseline test を実行する。
2. route group ごとに private function を作る。
3. duplicate している PLAN / ANALYZE / RESEARCH の stream hook は、契約を説明できる場合だけ小さく共通化する。
4. Chat / Worker / Coder の差を隠す generic handler は作らない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

完了条件:

- route ごとの実行先が private function 名で追える。
- Coder proposal と Worker execution の境界が維持されている。
- fallback / unknown route が成功扱いされていない。

## Phase 2-3: response assembly の分離

目的:

- `ProcessMessageResponse` の組み立てを明確にする。
- response、route、confidence、job ID の由来を追いやすくする。

対象範囲:

- `ProcessMessageResponse` construction。
- chat command response。
- normal route response。
- error ではない degraded TTS path の扱い。

対象外:

- response text の内容変更。
- LLM provider response shaping。
- Viewer rendering。

入力:

- route result。
- routing decision。
- job ID。
- chat command result。

出力:

- `ProcessMessageResponse`

副作用:

- なし。response assembly は純粋関数に近づける。

永続化:

- なし。

ログ:

- response assembly では新規ログを増やさない。

event 契約:

- response assembly 自体は event を emit しない。

error 契約:

- error を response に偽装しない。

変更してはいけない既存挙動:

- chat command handled は `RouteCHAT` / confidence `1.0`。
- normal response は decision route / confidence を返す。
- job ID を空にしない。

実装手順:

1. baseline test を実行する。
2. response construction helper を作る。
3. helper に event emission や session save を混ぜない。
4. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:

- response assembly の入力と出力が明確である。
- error が成功 response に混ざっていない。

## Phase 2-4: Viewer event / report / TTS hook の契約確認

目的:

- route chain 分割後も Viewer event、report、TTS / VTuber hook が落ちていないことを固定する。
- 表示、音声、口パク、ログを別契約として扱う。

対象範囲:

- `emit`
- `withStreamHooks`
- `pushTTS`
- `executeAutonomousTask` の `Observe` / `ReportStore`
- route-specific event emission

対象外:

- Viewer JS / CSS。
- TTS provider 実装。
- VTuber bridge 実装。
- report schema。

入力:

- event type
- route
- job ID
- session ID
- channel / chat ID
- response text

出力:

- `OrchestratorEvent`
- TTS push request
- VTuber request
- report record

副作用:

- listener への event emission。
- TTS / VTuber bridge call。
- report store write。

永続化:

- report store。

ログ:

- emit skipped / emit log。
- TTS degraded log。
- VTuber degraded log。

event 契約:

- `message.received` は user input。
- `routing.decision` は route decision。
- `agent.start` は execution start。
- `agent.thinking` は stream token。
- `agent.response` は response text。
- `entry.stage` は autonomous executor stage。
- `agent.notice` は degraded coder route などの通知。

error 契約:

- TTS / VTuber bridge failure は degraded log として扱い、route execution error と混同しない。
- report write failure の扱いは autonomous executor の既存契約に従う。

変更してはいけない既存挙動:

- 音声 chunk を本文表示の唯一根拠にしない。
- Viewer event と TTS / VTuber hook を同じ状態として扱わない。

実装手順:

1. event 名を `rg` で確認する。
2. route 別の event sequence test を必要最小限で追加する。
3. TTS degraded が ProcessMessage failure にならない既存契約を維持する。
4. after test を実行する。

検証手順:

```bash
rg -n "\"(message.received|routing.decision|agent.start|agent.thinking|agent.response|entry.stage|agent.notice)\"" internal/application/orchestrator internal/adapter/viewer
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

Viewer 実表示に触った場合のみ、Viewer で最低 1 セッションを追う。

完了条件:

- route chain 分割後も event / report / TTS hook が維持されている。
- Viewer 表示と音声 / 口パク / ログの契約が混同されていない。

## Phase 2-5: fallback / error route の扱い固定

目的:

- fallback と error を成功扱いしない。
- invalid route、empty response、provider error、worker error、coder proposal error を区別する。

対象範囲:

- unknown route。
- unsupported autonomous route。
- Coder unavailable。
- proposal generation error。
- invalid proposal。
- Worker execution error。
- TTS result verification fallback wording。

対象外:

- provider retry policy。
- Worker execution internals。
- Viewer JS / CSS。

入力:

- route。
- error。
- attempt result。
- response text。

出力:

- error。
- failure kind。
- event / log。
- optional partial response。

副作用:

- event emission。
- autonomous retry。
- report store write。

永続化:

- report store。

ログ:

- fallback / degraded / error を区別できる文言を維持する。

event 契約:

- success response と error / degraded notice を混同しない。
- fallback chain は `agent.notice` または log で判別可能にする。

error 契約:

- unknown route は error。
- invalid proposal は error。
- no coder は error。
- Worker execution error は error。
- TTS / VTuber degraded は route execution success / failure と別扱い。

変更してはいけない既存挙動:

- fallback を正常成功として表示しない。
- Coder が実行責務を持たない。
- Worker 安全境界を薄めない。

実装手順:

1. 既存 fallback / error tests を確認する。
2. 成功扱いされている fallback がないか調査する。
3. 必要なら error kind / event assertion を追加する。
4. 実装変更が必要な場合は最小限にする。
5. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

完了条件:

- fallback / error の扱いが route ごとに文書化されている。
- fallback が成功扱いされていないことをテストで確認している。

## Phase 2-6: Phase 2 完了判定

目的:

- Phase 2 の実装結果を `docs/refactor/` に記録する。
- Phase 3「Worker execution 安全境界の分離」に進めるか判断できる状態にする。

対象範囲:

- Phase 2 で作成した文書。
- Phase 2 で変更した実装ファイル。
- 実行した検証。
- 残リスク。

対象外:

- Phase 3 の実装。
- Worker execution 内部の分解。

入力:

- git diff / commit log。
- test results。
- route contract。

出力:

- `docs/refactor/Phase2_完了判定.md`

副作用:

- docs 追加。

永続化:

- docs/refactor の Markdown。

ログ:

- なし。

event 契約:

- なし。

error 契約:

- 未解決の error / risk を残リスクとして書く。

変更してはいけない既存挙動:

- 完了判定で未検証事項を完了扱いしない。

実装手順:

1. 最終 test を実行する。
2. Phase 2 実施内容を整理する。
3. 残リスクと Phase 3 への確認事項を書く。
4. docs を追加して Push する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
git status --short
```

完了条件:

- Phase 2 完了判定が docs/refactor にある。
- 実装と docs が Push 済み。
- Phase 3 に進む判断材料がある。

## Chat / Worker / Coder の責務境界

### Chat

責務:

- ユーザー対話。
- route 判断。
- 結果返却。
- chat command handling。
- session と response の統合。

持たない責務:

- file edit。
- shell / git 実行。
- Coder proposal の直接適用。
- Worker safety policy の実装。

### Worker

責務:

- 実行。
- file edit / command / test / git など。
- Coder が生成した plan / patch の適用。
- 実行結果、failure kind、summary の記録。

持たない責務:

- route 判断。
- proposal 生成。
- Viewer 表示本文の生成。

### Coder

責務:

- 設計。
- plan / patch 生成。
- proposal 生成。

持たない責務:

- 破壊的操作の直接実行。
- file edit / shell / git の直接実行。
- Worker safety policy の迂回。

## route ごとの契約

| route | 入力 | 出力 | dispatch 先 | Viewer event | session / job ID | report | TTS hook | error handling | fallback handling |
|---|---|---|---|---|---|---|---|---|---|
| CHAT | user message / attachments | Mio response | `MioAgent.Chat` | `message.received`, `routing.decision`, `agent.start`, `agent.thinking`, `agent.response` | session は request、job は task | なし | stream + final | Chat error は `task execution failed` | fallback なし |
| PLAN | user message | plan response | autonomous executor -> `MioAgent.Chat` | `entry.stage`, `agent.start`, `agent.response` | 同上 | autonomous report | stream + final | verification failure は retry / error | 成功代替にしない |
| ANALYZE | user message | analysis response | Heavy があれば `HeavyAgent.Generate`、なければ `MioAgent.Chat` | `entry.stage`, `agent.start`, `agent.response` | 同上 | autonomous report | stream + final | Heavy / Mio error は error | Heavy absence は既存 fallback だが正常成功の代替として隠さない |
| OPS | user message | Shiro execution response | `ShiroAgent.Execute` | `entry.stage`, `agent.start`, `agent.response` | 同上 | autonomous report | final push | Shiro error は error | fallback なし |
| RESEARCH | user message | research response | `MioAgent.Chat` | `entry.stage`, `agent.start`, `agent.response` | 同上 | autonomous report | stream + final | provider error は error | fallback なし |
| CODE | user message | coder / worker result | `CodeExecutor.ExecuteCode` | `entry.stage`, `agent.start`, `agent.response`, optional `agent.notice` | 同上 | autonomous report + worker result | final push | no coder / invalid proposal / worker error は error | coder1 -> coder2 -> coder3 -> coder4 の選択は成功代替ではなく可用性選択として扱う |
| CODE1 | user message | coder1 result or proposal execution result | `CodeExecutor` explicit coder1 | 同上 | 同上 | autonomous report + worker result | final push | coder1 unavailable は error | 品質縮退がある場合は notice / log |
| CODE2 | user message | coder2 result or proposal execution result | `CodeExecutor` explicit coder2 | 同上 | 同上 | autonomous report + worker result | final push | coder2 unavailable は error | 品質縮退がある場合は notice / log |
| CODE3 | user message | coder3 proposal execution result | `CodeExecutor` explicit coder3 -> Worker proposal path | 同上 | 同上 | autonomous report + worker result | final push | invalid proposal / worker error は error | max repair 到達時は error |
| CODE4 | user message | coder4 result or proposal execution result | `CodeExecutor` explicit coder4 | 同上 | 同上 | autonomous report + worker result | final push | coder4 unavailable は error | 現在コードに存在するため維持 |
| WILD | user message | creative response | `WildAgent.Generate` | `entry.stage`, `agent.start`, `agent.response` | 同上 | autonomous report | stream + final | wild unavailable は error | fallback なし |
| unknown | invalid route | error | なし | 原則 response event なし | job は作られる場合あり | なし | なし | unknown route error | 正常成功にしない |

## fallback / error 方針

- fallback を正常成功として扱わない。
- route fallback は安全側経路または可用性選択であり、成功表示の代替ではない。
- invalid route、empty response、provider error、worker error、coder proposal error を区別する。
- Viewer には成功、失敗、保留、安全側遷移が区別できる event / log を残す。
- TTS / VTuber degraded は route execution error と混同しない。
- `verifyTTSResult` にある暫定フォールバック文言は、Phase 2-5 で契約として再確認する。

## モジュール化と疎結合の方針

- 単に関数を分けるだけではモジュール化ではない。
- route 判断、dispatch、response assembly、event emission、reporting を責務単位で分ける。
- 共通化は意味のある契約がある場合だけ行う。
- 「便利だから共有する」「似ているからまとめる」だけの共通化は禁止する。
- 巨大 service / manager / helper / util を新設しない。
- interface、contract、event、DTO、adapter の境界を明確にする。
- Chat / Worker / Coder の差を隠す抽象化を避ける。
- 既存 `CodeExecutor` は Coder selection / proposal handoff の境界として尊重する。
- `WorkerExecutionService` は Phase 2 では内部変更せず、Phase 3 の安全境界整理に送る。

## テスト方針

基本方針:

- まず既存テストを通す。
- route ごとの unit test / integration test を優先する。
- `ProcessMessage` の分岐を変更する場合は、route 別に入力と期待結果を固定する。
- Coder proposal と Worker execution の境界が崩れていないことを検証する。
- fallback / error route が成功扱いになっていないことを検証する。
- Viewer event / session / report / TTS hook が落ちていないことを検証する。
- Viewer が関係する場合は最低 1 セッションを追う確認を入れる。

追加または強化する観点:

- `ProcessMessage` が chat command を routing 前に処理する。
- route decision 後に `routing.decision` event が出る。
- unknown route は error になる。
- no coder / invalid proposal / worker error は success response にならない。
- TTS start failure は degraded として扱い、route response を壊さない。
- `agent.response` と TTS chunk を本文表示根拠として混同しない。

## 検証手順

通常検証:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
git diff --stat
```

route / Viewer event に触った場合:

```bash
rg -n "\"(message.received|routing.decision|agent.start|agent.thinking|agent.response|entry.stage|agent.notice)\"" internal/application/orchestrator internal/adapter/viewer
```

Viewer 表示に影響する場合:

- Viewer で最低 1 セッションを追う。
- 表示本文、event log、終了状態を照合する。
- TTS / 口パクに触った場合は、音声 trigger と表示本文を別に確認する。

runtime config に触った場合:

```bash
test -f ~/.picoclaw/config.yaml
curl -fsS http://127.0.0.1:18790/health
```

Phase 2 では runtime config の意味変更をしないため、原則として live health は不要である。

## リスク

- route 分岐を共通化しすぎて責務差が見えなくなる。
- Coder に実行責務が戻る。
- Worker の安全境界が薄くなる。
- fallback が成功扱いになる。
- Viewer event / report / TTS hook が落ちる。
- session ID / job ID の単位が混ざる。
- route 名と実行責務がずれる。
- TTS / VTuber degraded を route execution error と混同する。
- テストだけ通って実セッションが壊れる。
- `executeAutonomousTask` の retry / verification と route dispatch 分割を同時に変えすぎる。

## Phase 2 の最終完了条件

- `MessageOrchestrator` の route chain が読みやすくなっている。
- Chat / Worker / Coder の責務境界がコード上で追える。
- route ごとの入力、出力、event、error contract が文書化されている。
- Coder proposal と Worker execution の境界が崩れていない。
- fallback が正常成功として扱われていない。
- Viewer event / session / report / TTS hook が維持されている。
- 対象テストが成功している。
- 必要な場合、Viewer で最低 1 セッション確認している。
- 各小 Phase の文書と実装差分が Push 済み。
- ユーザーが Phase 3「Worker execution 安全境界の分離」に進むか判断できる。

## 実装時の停止条件

- テストが失敗し、Phase 内で原因を安全に切り分けられない場合。
- Coder proposal と Worker execution の境界が崩れそうな場合。
- WorkerExecutionService、ToolRunner、PolicyEngine の内部変更が必要になった場合。
- Viewer JS / CSS の変更が必要になった場合。
- fallback を成功表示にする必要が出た場合。
- runtime config、LLM provider、STT/TTS provider の挙動変更が必要になった場合。

停止条件に当たった場合は、実装を止め、状況、原因、選択肢を報告する。
