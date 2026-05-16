# Phase5 CodeExecutor ファイル分離実装仕様

## Phase5 の目的

Phase5 は、Phase4 で整理した `CodeExecutor` の責務境界をファイル構成にも反映する段階である。

目的は次の通り。

- `internal/application/orchestrator/code_executor.go` に残っている責務を、selection / proposal path / Generate path / event / response contract の単位で分離する。
- `code_executor.go` を CODE 系 route の orchestration が読める薄い入口にする。
- 挙動変更を行わず、関数本体、error message、log message、event content、route 条件を維持する。
- モジュール化と疎結合を最重要方針として、将来 Coder selection、proposal execution、event emission を差し替えやすい構造にする。
- 単にファイルを分けるだけではなく、各ファイルが持つ責務、入力、出力、副作用、エラー契約を説明できる状態にする。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

正本仕様では、Coder は plan / patch / risk / cost hint を生成し、Proposal の適用は `WorkerExecutionService.ExecuteProposal()` が担当する。Phase5 ではこの責務境界を変更しない。

`docs/codebase-map/` は一次解析資料として使う。CodeExecutor 周辺の結合点、ユースケース、潜在バグを確認するための補助資料であり、正本仕様ではない。判断が矛盾する場合は `docs/01_正本仕様/実装仕様.md` と現在コードを優先する。`docs/archive/` は一次参照にしない。

## docs/codebase-map から見た注意点

`docs/codebase-map/` では、CodeExecutor 周辺は次の結合点として扱われている。

- CODE 系 route は `MessageOrchestrator` から `CodeExecutor.ExecuteCode` に渡る。
- Coder proposal は `proposal.Proposal` / `patch.PatchCommand` を経由して `WorkerExecutionService` に渡る。
- Worker execution の安全境界は `WorkerExecutionService`、`ToolRunner`、`PolicyEngine` にまたがるため、CodeExecutor 側へ実行責務を戻してはいけない。
- fallback / degraded / error は成功の代替ではなく、notice / log / error として扱う。
- route と Coder slot の対応を変えると、Orchestrator、Viewer 表示、分散実行、prompts の整合性に影響する。

Phase5 はファイル分離のみを扱い、この結合点の意味は変えない。

## 対象範囲

Phase5 の対象は次に限定する。

- `internal/application/orchestrator/code_executor.go`
- `CodeExecutor`
- `CodeExecutionRequest`
- `CodeExecutionResponse`
- `DefaultCodeExecutor`
- `codeTarget`
- selection helper
- proposal path helper
- Generate path helper
- event emission helper
- response assembly helper
- `shouldUseProposalPath`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`

関連する確認対象:

- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/orchestrator/*code*_test.go`
- `internal/application/orchestrator/coder_status.go`
- `internal/domain/capability/`
- `internal/domain/proposal/`
- `internal/domain/patch/`
- `internal/application/service/worker_execution_service.go`

## 対象外

Phase5 では次を変更しない。

- `MessageOrchestrator` の route chain。
- `WorkerExecutionService` の内部実行方式。
- `ToolRunner` / `PolicyEngine`。
- Coder provider。
- proposal / patch format の意味。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS。
- runtime config。
- 未追跡の `tests/`。

## 現在の code_executor.go の責務棚卸し

### CodeExecutor interface

現在の責務:

- CODE 系 route 実行の抽象インターフェースを定義する。
- `ExecuteCode(ctx, req)` の入出力契約を持つ。

置いてはいけない責務:

- selection、proposal、Generate、event の具体処理。

### request / response DTO

現在の責務:

- `CodeExecutionRequest` は task、route、session、channel、chat、job を CodeExecutor に渡す。
- `CodeExecutionResponse` は response 本文と `Handled` を返す。
- `Handled` は proposal path が処理したかを表し、success / failure ではない。

置いてはいけない責務:

- Worker 実行結果の詳細分類。
- Viewer 表示契約の意味決定。

### DefaultCodeExecutor struct / constructor

現在の責務:

- Coder slot、WorkerExecutionService、CoderStatus、eventEmitter、coderCaps を保持する。
- `NewDefaultCodeExecutor` で依存を注入する。
- `WithCapabilities` で dynamic selection 用の capability を設定する。

置いてはいけない責務:

- provider 初期化。
- Worker 実行方式の決定。
- runtime config の意味解決。

### ExecuteCode orchestration

現在の責務:

- route に応じて Coder を選択する。
- CoderStatus release を defer する。
- code handoff log を出す。
- degraded route notice と handoff start event を出す。
- proposal path を試行する。
- proposal path が処理しない場合、Generate path を実行する。

置いてはいけない責務:

- proposal validation の詳細。
- Worker handoff の詳細。
- Generate event の詳細。
- selection の詳細。

### Coder selection

現在の責務:

- `coderCaps != nil` の場合は `capability.SelectCoder` に委譲する。
- explicit route `CODE1` / `CODE2` / `CODE3` / `CODE4` を Coder slot に対応付ける。
- generic `CODE` では `coder1` -> `coder2` -> `coder3` -> `coder4` の順で選ぶ。
- CoderStatus がある場合、busy state を acquire し、release 関数を `codeTarget` に保持する。

置いてはいけない責務:

- proposal generation。
- Worker handoff。
- response assembly。
- event response 組み立て。

### proposal path

現在の責務:

- selected Coder が `CoderAgentWithProposal` を満たすか確認する。
- proposal generation を呼ぶ。
- nil / invalid proposal を Worker に渡さず error にする。
- valid proposal の plan event を出す。
- WorkerExecutionService へ handoff する。
- Worker result を formatting して response にする。

置いてはいけない責務:

- Coder selection。
- patch command 実行。
- protected file 判定。
- ToolRunner / PolicyEngine の policy 判断。

### Generate path

現在の責務:

- selected Coder の `Generate` を呼ぶ。
- Generate error event を出し、error を返す。
- Generate success event を Coder -> Shiro、Shiro -> Mio の順で出す。
- Generate path response を返す。

置いてはいけない責務:

- proposal validation。
- Worker handoff。
- Worker result formatting。

### event emission

現在の責務:

- degraded route notice を出す。
- Code handoff start event を出す。
- eventEmitter がある場合だけ event を配送する。
- eventEmitter を後から設定できる。

置いてはいけない責務:

- Viewer 表示本文の決定。
- Worker log の代替。
- 音声、口パク、ログの混同。

### response assembly

現在の責務:

- proposal path response は `Handled: true` とする。
- Generate path response は `Handled: false` とする。

置いてはいけない責務:

- success / failure 判定。
- error の握りつぶし。
- event emission。

### route prompt / coder lookup helper

現在の責務:

- `explicitCodeRouteTarget` で explicit route と Coder slot / prompt を対応付ける。
- `systemPromptForRoute` で route に対応する prompt を返す。
- `coderByName` で Coder slot 名から CoderAgent を取得する。

置いてはいけない責務:

- dynamic capability selection の意味変更。
- Coder provider 初期化。

## 提案するファイル分割

### 採用: internal/application/orchestrator/code_executor.go

残すもの:

- `CodeExecutor` interface
- `CodeExecutionRequest`
- `DefaultCodeExecutor`
- `NewDefaultCodeExecutor`
- `WithCapabilities`
- `ExecuteCode`

採用理由:

- CODE 系 route の入口と依存注入を 1 ファイルで追える。
- `ExecuteCode` の composition flow を残すことで、selection -> notice/start event -> proposal path -> Generate path の大枠が読める。
- 詳細 helper は別ファイルへ移し、`code_executor.go` を巨大な実装置き場にしない。

不採用にする配置:

- proposal / Generate / event helper を `code_executor.go` に残し続ける配置は不採用。Phase4 で整理した境界をファイル構成に反映できないため。

### 採用: internal/application/orchestrator/code_executor_selection.go

置くもの:

- `codeTarget`
- `selectCoderForRoute`
- `selectDynamicCoderForRoute`
- `selectExplicitCoderForRoute`
- `selectAvailableCoderForGenericRoute`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`

採用理由:

- selection の入力、出力、副作用は `codeTarget` に集約されるため、`codeTarget` は selection ファイルに置くのが最も読みやすい。
- route / capability / CoderStatus / prompt / Coder slot lookup の境界を 1 ファイルで確認できる。
- proposal path と Worker handoff を selection から分離できる。

注意:

- `codeTarget.release` は CoderStatus の release 関数を保持するだけで、release 実行は `ExecuteCode` の defer に残す。
- `coderByName` は selection のための lookup として扱い、provider setup へ広げない。

### 採用: internal/application/orchestrator/code_executor_proposal.go

置くもの:

- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `proposalCoderForTarget`
- `generateProposalForTarget`
- `validateGeneratedProposal`
- `emitProposalPlan`
- `executeProposalWithWorker`
- `emitProposalExecutionResult`

採用理由:

- proposal generation、validation、Worker handoff、result formatting の流れを 1 ファイルで追える。
- invalid proposal を Worker に渡さない境界を明確にできる。
- WorkerExecutionService 内部へ責務を広げず、`ExecuteProposal` 契約だけに依存する構造を保てる。

注意:

- `emitProposalPlan` と `emitProposalExecutionResult` は proposal path 専用 event なので、このファイルに置く。
- 汎用 event helper へ寄せすぎない。event を「似ているからまとめる」共通化は行わない。

### 採用: internal/application/orchestrator/code_executor_generate.go

置くもの:

- `executeCoderGeneratePath`
- `emitCoderGenerateError`
- `emitCoderGenerateResponse`

採用理由:

- proposal path に進まない通常 Generate の契約を独立して確認できる。
- Generate error を fallback success にしないこと、Generate success は `Handled: false` であることを追いやすい。
- proposal validation や Worker handoff と混ざらない。

注意:

- `emitCoderGenerateError` / `emitCoderGenerateResponse` は Generate path 専用 event として扱う。
- Viewer 表示契約や SSE schema には踏み込まない。

### 採用: internal/application/orchestrator/code_executor_events.go

置くもの:

- `emit`
- `SetEventEmitter`
- `emitDegradedRouteNotice`
- `emitCodeHandoffStart`

採用理由:

- CodeExecutor 全体の共通 event 配送と、ExecuteCode orchestration に近い start / notice event をまとめられる。
- degraded route notice が fallback success ではないことを明確にできる。
- proposal path 専用 event、Generate path 専用 event はそれぞれのファイルに残し、汎用 event ファイルを巨大化させない。

注意:

- `emit` は nil eventEmitter guard のみを持つ。
- event type / from / to / content / route は変更しない。

### 採用: internal/application/orchestrator/code_executor_response.go

置くもの:

- `CodeExecutionResponse`
- `buildProposalHandledResponse`
- `buildCoderGenerateResponse`

採用理由:

- `Handled` の意味を response contract として 1 ファイルで確認できる。
- `CodeExecutionResponse` は `CodeExecutionRequest` と違い、proposal path / Generate path の戻り契約に強く関係するため、response helper と同じファイルに置く。
- `Handled` が success / failure ではないことを明確にできる。

注意:

- `CodeExecutionRequest` は `ExecuteCode` の入力 DTO として `code_executor.go` に残す。
- response helper は副作用を持たない。

## 分割単位ごとの契約

### code_executor.go

責務:

- CodeExecutor の入口。
- request DTO。
- DefaultCodeExecutor の依存保持。
- constructor / capability setup。
- ExecuteCode の大枠 orchestration。

入力:

- `context.Context`
- `CodeExecutionRequest`
- Coder slots
- WorkerExecutionService
- CoderStatus
- eventEmitter
- coder capabilities

出力:

- `CodeExecutionResponse`
- error

副作用:

- CoderStatus release の defer 実行。
- code handoff log。
- selection / proposal / Generate / event helper の呼び出し。

永続化:

- なし。

ログ:

- code handoff log のみを残す。

エラー契約:

- selection error はそのまま返す。
- proposal path が handled true の場合は、その response / error を返す。
- proposal path が handled false の場合は Generate path に進む。

置いてはいけない責務:

- selection 詳細。
- proposal validation 詳細。
- Worker handoff 詳細。
- Generate event 詳細。
- response assembly 詳細。

将来差し替える場合の境界:

- `CodeExecutor` interface を維持したまま、Default 実装を別実装へ差し替える。

### code_executor_selection.go

責務:

- route、capability、CoderStatus から `codeTarget` を選ぶ。
- route と prompt / Coder slot の対応を管理する。

入力:

- `routing.Route`
- `[]capability.CoderCapability`
- Coder slots
- CoderStatus

出力:

- `codeTarget`
- error

副作用:

- CoderStatus acquire。
- selected / skipped / degraded log。

永続化:

- なし。

ログ:

- `mode=dynamic`
- `mode=explicit`
- `mode=auto`
- unavailable / busy skip

エラー契約:

- dynamic selection error は `%s route: %w` で wrap する。
- selected Coder missing は `%s route: selected coder %s is not initialized`。
- explicit Coder missing は `%s route requested but no %s available`。
- generic `CODE` 全 unavailable は `CODE route requested but all coders are unavailable`。
- CoderStatus ありの全 busy / unavailable は `CODE route requested but all coders are busy or unavailable`。
- unknown route は `unknown code route: %s`。

置いてはいけない責務:

- proposal generation。
- Worker handoff。
- response formatting。
- event response assembly。

将来差し替える場合の境界:

- selection strategy を差し替える場合も、出力は `codeTarget` に保つ。

### code_executor_proposal.go

責務:

- proposal path の可否判定。
- proposal generation。
- proposal validation。
- valid proposal の Worker handoff。
- Worker result formatting との接続。

入力:

- `CodeExecutionRequest`
- `codeTarget`
- `CoderAgentWithProposal`
- `proposal.Proposal`

出力:

- `CodeExecutionResponse`
- handled bool
- error
- `patch.PatchExecutionResult`

副作用:

- Coder `GenerateProposal` 呼び出し。
- proposal path event emission。
- valid proposal の場合だけ WorkerExecutionService handoff。

永続化:

- CodeExecutor 自体は永続化しない。
- WorkerExecutionService に渡った後の副作用は Phase3 の契約に従う。

ログ:

- Phase5 では新規ログを追加しない。

エラー契約:

- proposal interface 非対応 Coder は handled false。
- proposal generation error は handled true + `%s proposal generation failed: %w`。
- nil / invalid proposal は handled true + `%s proposal generation failed: invalid proposal`。
- Worker execution error は handled true + `worker execution failed: %w`。

置いてはいけない責務:

- Coder selection。
- patch command 実行。
- protected file 判定。
- ToolRunner / PolicyEngine の policy 判断。

将来差し替える場合の境界:

- proposal executor を差し替える場合も、Worker handoff 条件は valid proposal のみに固定する。

### code_executor_generate.go

責務:

- proposal path に進まない通常 Generate を実行する。
- Generate error / success event を出す。

入力:

- `context.Context`
- `CodeExecutionRequest`
- `codeTarget`
- generated response
- Generate error

出力:

- `CodeExecutionResponse`
- error

副作用:

- Coder `Generate` 呼び出し。
- Generate path event emission。

永続化:

- なし。

ログ:

- なし。Generate path では既存 event を維持する。

エラー契約:

- Generate error は event と error にする。
- Generate error を fallback success にしない。
- Generate success は `Handled: false`。

置いてはいけない責務:

- proposal validation。
- Worker handoff。
- Worker result formatting。

将来差し替える場合の境界:

- Generate path を差し替える場合も、`CoderAgent.Generate` と `CodeExecutionResponse{Handled:false}` の意味を維持する。

### code_executor_events.go

責務:

- eventEmitter への共通配送。
- degraded route notice。
- code handoff start event。

入力:

- event type
- from / to
- content
- route
- job / session / channel / chat
- `CodeExecutionRequest`
- `codeTarget`

出力:

- なし。

副作用:

- event emission。
- degraded route log。

永続化:

- なし。

ログ:

- degraded route log。

エラー契約:

- eventEmitter が nil の場合は何もしない。
- event emission failure は現在契約上扱わない。

置いてはいけない責務:

- Viewer 表示本文の決定。
- Worker log の代替。
- proposal path 専用 event の過剰集約。
- Generate path 専用 event の過剰集約。

将来差し替える場合の境界:

- eventEmitter の差し替えは `SetEventEmitter` と `emit` の契約で吸収する。

### code_executor_response.go

責務:

- `CodeExecutionResponse` の戻り契約。
- proposal path / Generate path の response assembly。

入力:

- response string

出力:

- `CodeExecutionResponse`

副作用:

- なし。

永続化:

- なし。

ログ:

- なし。

エラー契約:

- `Handled` は proposal path 処理有無を表す。
- `Handled` を success / failure として扱わない。
- response helper は error を握りつぶさない。

置いてはいけない責務:

- event emission。
- Worker result formatting。
- success / failure 判定。

将来差し替える場合の境界:

- response 表現を変更する場合は、`Handled` の意味を先に仕様変更として扱う。

## 分割方針

- package は `orchestrator` のままにする。
- exported API は原則増やさない。
- 関数名、error message、log message、event content を変えない。
- route path、route value、Coder slot 対応を変えない。
- テストから必要な既存 package-private helper は維持してよい。
- import のためだけに責務を広げない。
- ファイル名は責務を表す。
- 巨大な `helper` / `util` / `manager` ファイルを作らない。
- 「便利だから共有する」「似ているからまとめる」だけの共通化は禁止する。
- `code_executor.go` を薄くするが、`ExecuteCode` の composition flow は追えるようにする。
- proposal path 専用 event は proposal ファイル、Generate path 専用 event は Generate ファイル、共通配送と start / notice は event ファイルに置く。

## 小 Phase 案

### Phase5-0: 現在ファイル構成と baseline test 記録

目的:

- Phase5 の開始状態を固定する。

対象範囲:

- `code_executor.go`
- `code_executor_test.go`
- Phase4 docs

対象外:

- 関数移動。
- 実装変更。

実装手順:

1. `git status --short` を確認する。
2. 未追跡の `tests/` を対象外として確認する。
3. baseline test を実行する。
4. 現在の関数配置を `rg` で記録する。
5. Phase5-0 文書を作成する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
rg "^(type|func) |^func \\(e \\*DefaultCodeExecutor\\)" internal/application/orchestrator/code_executor.go
```

完了条件:

- baseline test が成功している。
- 現在の関数配置が記録されている。
- コード変更を行っていない。

### Phase5-1: selection helper を code_executor_selection.go へ移動

目的:

- Coder selection の責務をファイル単位で独立させる。

対象範囲:

- `codeTarget`
- `selectCoderForRoute`
- `selectDynamicCoderForRoute`
- `selectExplicitCoderForRoute`
- `selectAvailableCoderForGenericRoute`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`

対象外:

- proposal path。
- Generate path。
- event helper。
- response helper。

実装手順:

1. `code_executor_selection.go` を作成する。
2. selection 関数と `codeTarget` を移動する。
3. 関数本体は原則そのままにする。
4. `code_executor.go` から不要 import を削除する。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/capability ./cmd/picoclaw
git diff --check
rg "selectCoderForRoute|selectDynamicCoderForRoute|selectExplicitCoderForRoute|selectAvailableCoderForGenericRoute|explicitCodeRouteTarget|systemPromptForRoute|coderByName|type codeTarget" internal/application/orchestrator
```

完了条件:

- selection helper が `code_executor_selection.go` にある。
- CoderStatus release 契約テストが成功している。
- generic `CODE` fallback order が変わっていない。

### Phase5-2: proposal path helper を code_executor_proposal.go へ移動

目的:

- proposal path と Worker handoff 境界をファイル単位で独立させる。

対象範囲:

- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `proposalCoderForTarget`
- `generateProposalForTarget`
- `validateGeneratedProposal`
- `emitProposalPlan`
- `executeProposalWithWorker`
- `emitProposalExecutionResult`

対象外:

- WorkerExecutionService 内部。
- patch parser。
- selection。
- Generate path。

実装手順:

1. `code_executor_proposal.go` を作成する。
2. proposal path helper を移動する。
3. `proposal` / `patch` import を移動先に寄せる。
4. event content と error message を変えない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/patch ./cmd/picoclaw
git diff --check
rg "shouldUseProposalPath|tryExecuteProposalPath|proposalCoderForTarget|generateProposalForTarget|validateGeneratedProposal|executeProposalWithWorker" internal/application/orchestrator
```

完了条件:

- proposal path helper が `code_executor_proposal.go` にある。
- invalid proposal が Worker に渡らない契約が維持されている。
- valid proposal の Worker handoff 契約が維持されている。

### Phase5-3: Generate path helper を code_executor_generate.go へ移動

目的:

- Generate path を proposal path からファイル単位で分ける。

対象範囲:

- `executeCoderGeneratePath`
- `emitCoderGenerateError`
- `emitCoderGenerateResponse`

対象外:

- proposal path。
- selection。
- common event emitter。
- response helper。

実装手順:

1. `code_executor_generate.go` を作成する。
2. Generate path helper を移動する。
3. event type / from / to / content / route を変えない。
4. gofmt を実行する。
5. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
rg "executeCoderGeneratePath|emitCoderGenerateError|emitCoderGenerateResponse" internal/application/orchestrator
```

完了条件:

- Generate path helper が `code_executor_generate.go` にある。
- Generate error が error として返る。
- Generate path response が `Handled: false` のままである。

### Phase5-4: event helper を code_executor_events.go へ移動

目的:

- 共通 event emission と ExecuteCode 直下の start / notice event をファイル単位で分ける。

対象範囲:

- `emit`
- `SetEventEmitter`
- `emitDegradedRouteNotice`
- `emitCodeHandoffStart`

対象外:

- proposal path 専用 event。
- Generate path 専用 event。
- Viewer JS / CSS。
- SSE event schema。

実装手順:

1. `code_executor_events.go` を作成する。
2. event helper を移動する。
3. `fmt` / `log` import を必要なファイルに寄せる。
4. event content と log message を変えない。
5. gofmt を実行する。
6. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
rg "SetEventEmitter|emitDegradedRouteNotice|emitCodeHandoffStart|func \\(e \\*DefaultCodeExecutor\\) emit" internal/application/orchestrator
```

完了条件:

- event helper が `code_executor_events.go` にある。
- degraded route notice を fallback success として扱っていない。
- Viewer-facing event の type / from / to / route を変えていない。

### Phase5-5: response helper を code_executor_response.go へ移動

目的:

- `CodeExecutionResponse` と `Handled` 契約をファイル単位で独立させる。

対象範囲:

- `CodeExecutionResponse`
- `buildProposalHandledResponse`
- `buildCoderGenerateResponse`

対象外:

- `CodeExecutionRequest`
- Worker result formatting。
- event emission。
- success / failure 判定。

実装手順:

1. `code_executor_response.go` を作成する。
2. `CodeExecutionResponse` と response helper を移動する。
3. `CodeExecutionRequest` は `code_executor.go` に残す。
4. gofmt を実行する。
5. after test を実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/patch ./cmd/picoclaw
git diff --check
rg "type CodeExecutionResponse|buildProposalHandledResponse|buildCoderGenerateResponse" internal/application/orchestrator
```

完了条件:

- response contract が `code_executor_response.go` にある。
- `Handled` の意味が変わっていない。
- response helper が副作用を持っていない。

### Phase5-6: 完了判定

目的:

- Phase5 が CodeExecutor 周辺ファイル分離として完了したか判定する。

対象範囲:

- Phase5 docs。
- Phase5 実装差分。
- Phase5 test result。

対象外:

- Phase6 以降。

実装手順:

1. 全体 test を実行する。
2. `git diff --check` を実行する。
3. `git status --short` を確認する。
4. 関数配置を `rg` で確認する。
5. `docs/refactor/Phase5_完了判定.md` を作成する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
rg "func \\(e \\*DefaultCodeExecutor\\) select|func shouldUseProposalPath|func \\(e \\*DefaultCodeExecutor\\) tryExecuteProposalPath|func \\(e \\*DefaultCodeExecutor\\) executeCoderGeneratePath|func \\(e \\*DefaultCodeExecutor\\) emit|func buildProposalHandledResponse" internal/application/orchestrator
```

完了条件:

- `code_executor.go` が入口と orchestration に絞られている。
- selection / proposal / Generate / event / response helper が責務別ファイルへ移動している。
- 挙動変更がない。
- 対象パッケージのテストが成功している。
- すべての Phase5 docs / 実装差分が push 済みである。

## 実装手順

Phase5 の実装は次の順で進める。

1. baseline test を実行する。
2. Phase5-0 文書を作成し、commit / push する。
3. 1 回の commit では 1 種類の責務だけを移動する。
4. 関数本体は原則そのまま移動する。
5. 関数名、error message、log message、event content を変えない。
6. import を最小化する。
7. gofmt を実行する。
8. after test を実行する。
9. 各小 Phase ごとに docs / 実装を commit / push する。
10. 最後に Phase5 完了判定を作成し、commit / push する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
git diff --check
git diff --stat
```

ファイル移動確認:

```bash
rg "func \\(e \\*DefaultCodeExecutor\\) select|func shouldUseProposalPath|func \\(e \\*DefaultCodeExecutor\\) tryExecuteProposalPath|func \\(e \\*DefaultCodeExecutor\\) executeCoderGeneratePath|func \\(e \\*DefaultCodeExecutor\\) emit|func buildProposalHandledResponse" internal/application/orchestrator
```

## リスク

- 関数移動時に import を壊す。
- package-private helper の配置を誤る。
- event type / from / to / content / route を変える。
- error message を変える。
- log message を変える。
- CoderStatus release 契約を壊す。
- invalid proposal が Worker に渡る。
- `Handled` の意味を変える。
- `code_executor.go` を薄くしすぎて `ExecuteCode` の流れが読めなくなる。
- 分割ファイルが新しい巨大 helper になる。
- ファイル分割だけで責務境界を説明できない状態になる。
- `CodeExecutionResponse` と `CodeExecutionRequest` を同時に移して、入口 DTO と response contract の意味が曖昧になる。

## 完了条件

- `docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md` が作成されている。
- 提案ファイル分割が明記されている。
- 各ファイルの責務、入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 小 Phase の順序が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- ユーザーが次に「Phase5 を実装してよいか」を判断できる。

## 次に確認すべきこと

Phase5 実装に入る前に、次を確認する。

1. `docs/refactor/Phase5_CodeExecutorファイル分離実装仕様.md` の分割方針でよいか。
2. `CodeExecutionResponse` を `code_executor_response.go` へ移し、`CodeExecutionRequest` を `code_executor.go` に残す判断でよいか。
3. 各小 Phase で docs commit / push と実装 commit / push を分ける運用を継続するか。
