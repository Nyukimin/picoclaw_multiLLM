# Phase6 route chain 統合確認実装仕様

## Phase6 の目的

Phase6 は、Phase2 から Phase5 で整理した責務境界が、Chat / Worker / Coder route chain 全体として矛盾していないかを確認する段階である。

目的は次の通り。

- `MessageOrchestrator -> CodeExecutor -> WorkerExecutionService` の流れを現在契約として固定する。
- Chat / Worker / Coder の責務分離を docs と契約テストで再確認する。
- 次の大きな `MessageOrchestrator` 分割へ入る前に、route chain の安全確認を行う。
- 挙動変更ではなく、現在契約確認と不足している契約テストの計画を行う。
- fallback、degraded、error、`Handled`、Viewer event、execution log、response text を混同しない。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

正本仕様では、Chat はユーザー対話と route 判断、Worker は実行主体、Coder は plan / patch / proposal / Generate を担当する。Coder は破壊的操作を直接実行せず、valid proposal の適用は `WorkerExecutionService.ExecuteProposal()` が担当する。

Phase6 ではこの責務境界を変更しない。`docs/codebase-map/` は route chain、結合点、ユースケース、潜在バグを確認する一次解析資料として使うが、正本仕様ではない。判断が矛盾する場合は正本仕様と現在コードを優先し、`docs/archive/` は一次参照にしない。

## docs/codebase-map から見た注意点

`docs/codebase-map/` では、route chain 周辺は次の結合点として整理されている。

- `MessageOrchestrator` は Mio route、Shiro / Worker / Coder / Wild / Heavy、session、events、TTS / VTuber を統合する。
- code execution chain は Coder proposal から `PatchCommand`、`WorkerExecutionService`、protected checks、execution result へ進む。
- Worker execution は Coder の plan / patch を実行する場所であり、Coder に実行責務を戻してはいけない。
- route 追加 / 変更は `routing.Route`、`MessageOrchestrator`、prompts、Viewer 表示、tests に影響する。
- `MessageOrchestrator.ProcessMessage` の全 route 分岐は影響範囲が広いため、Phase6 では大規模分割を開始しない。

## 対象範囲

Phase6 の対象は次に限定する。

- `MessageOrchestrator.ProcessMessage`
- route decision / route dispatch
- CODE / CODE1 / CODE2 / CODE3 / CODE4 path
- `executeCodeViaShiro`
- `CodeExecutor.ExecuteCode`
- Coder selection
- proposal path
- Generate path
- WorkerExecutionService handoff
- response assembly
- event emission
- existing route chain tests

確認対象ファイル:

- `internal/application/orchestrator/message_orchestrator.go`
- `internal/application/orchestrator/message_orchestrator_*test.go`
- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_executor_*.go`
- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/service/worker_execution_service.go`
- `internal/application/service/worker_execution_service_test.go`
- `internal/domain/routing/`
- `internal/domain/task/`
- `internal/domain/proposal/`
- `internal/domain/patch/`

## 対象外

Phase6 では次を変更しない。

- `MessageOrchestrator` の大規模分割。
- `WorkerExecutionService` 内部の再分割。
- `ToolRunner` / `PolicyEngine`。
- Coder provider。
- proposal / patch format の意味。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS。
- runtime config。
- 未追跡の `tests/`。

## 現在の route chain 棚卸し

### ユーザー入力から ProcessMessage へ入る流れ

`ProcessMessage` は `ProcessMessageRequest` を受け取り、次の順で処理する。

1. session load / create。
2. `message.received` event。
3. pre-routing chat command。
4. task / job / TTS session ID assembly。
5. Mio による route decision。
6. `routing.decision` event。
7. TTS session start。
8. route execution。
9. TTS session end。
10. session save。
11. `ProcessMessageResponse` assembly。

pre-routing chat command が handled の場合は route decision を通らず、`CHAT` response として返す。

### Mio / routing decision の責務

Mio は Chat と route 判断を担当する。

- ユーザー入力を task として受け取る。
- `DecideAction` で route を決める。
- route decision は `routing.Decision` として `MessageOrchestrator` に返す。
- Mio は Worker の実行詳細、Coder proposal validation、file edit / shell / git 実行を持たない。

### route dispatch の責務

`executeTask` は `CHAT` 以外を autonomous route として扱い、`executeAutonomousTask` から `executeRouteDirect` へ進む。

`executeRouteDirect` は route-specific function へ分岐する。

- `CHAT`: `executeChatRoute`
- `OPS`: `executeOPSRoute`
- `CODE` / `CODE1` / `CODE2` / `CODE3` / `CODE4`: `executeCodeRoute`
- `WILD`: `executeWildRoute`
- `PLAN`: `executePlanRoute`
- `ANALYZE`: `executeAnalyzeRoute`
- `RESEARCH`: `executeResearchRoute`

Phase6 ではこの分岐構造を変更しない。

### CODE 系 route が Shiro 経由で CodeExecutor に入る流れ

CODE 系 route は `executeCodeRoute` から `executeCodeViaShiro` に進む。

`executeCodeViaShiro` は `CodeExecutionRequest` を組み立て、`CodeExecutor.ExecuteCode` へ委譲する。

`CodeExecutor` は次の Shiro 経由 event を維持する。

- `agent.start`: mio -> shiro
- `agent.start`: shiro -> coder
- Generate path success では `agent.response`: coder -> shiro
- Generate path success では `agent.response`: shiro -> mio
- proposal path では plan、Worker start、Worker result / error の event を出す。

### CodeExecutor が Coder を選ぶ責務

CodeExecutor は route / capability / CoderStatus に基づいて Coder を選ぶ。

- explicit `CODE1` は coder1。
- explicit `CODE2` は coder2。
- explicit `CODE3` は coder3。
- explicit `CODE4` は coder4。
- generic `CODE` は coder1 -> coder2 -> coder3 -> coder4 の順で選ぶ。
- `coderCaps != nil` の場合は dynamic capability selection を使う。
- CoderStatus がある場合、generic `CODE` で acquire / release を行う。

CodeExecutor は provider 初期化や Worker 実行詳細を持たない。

### Coder が proposal / Generate を担当する責務

Coder は次を担当する。

- proposal path では `GenerateProposal` により proposal を生成する。
- Generate path では `Generate` により通常 response を生成する。
- plan / patch / proposal / Generate は担当するが、file edit / shell / git 実行は担当しない。

proposal interface 非対応 Coder は Generate path に戻る。

### WorkerExecutionService が valid proposal を実行する責務

WorkerExecutionService は valid proposal だけを受け取る。

- nil / invalid proposal は CodeExecutor で止め、WorkerExecutionService に渡さない。
- valid proposal の場合だけ `ExecuteProposal(ctx, req.Task.JobID(), proposal)` を呼ぶ。
- WorkerExecutionService は proposal の patch を parse し、file edit / shell / git execution、protected check、execution result assembly を担当する。

### response が Mio / user 向けに戻る流れ

CodeExecutor の response は `executeCodeViaShiro` から `executeCodeRoute` に戻り、`ProcessMessageResponse` として組み立てられる。

`CodeExecutionResponse.Handled` は proposal path が処理したかを表す内部契約であり、success / failure の一般状態ではない。`executeCodeViaShiro` は現在 `resp.Response` と error を返し、`Handled` を final success 判定には使わない。

### event が Viewer / log と混同されないこと

`agent.start` / `agent.response` / `agent.notice` は Viewer-facing event であり、Worker execution log や execution evidence の代替ではない。

degraded route notice は品質縮退の通知であり、fallback success ではない。Viewer event、execution log、response text はそれぞれ別契約として扱う。

## Chat / Worker / Coder 責務境界

### Chat

責務:

- ユーザー対話。
- route 判断。
- pre-routing chat command。
- 結果返却。
- response assembly。

持たない責務:

- file edit / shell / git operation。
- WorkerExecutionService の command dispatch。
- Coder proposal validation。
- 破壊的操作。

### Worker

責務:

- 実行主体。
- file edit / shell / git / test execution。
- Coder proposal の適用。
- protected file / workspace restriction の確認。
- execution result / evidence / log。

持たない責務:

- route decision。
- Coder selection。
- proposal generation。

### Coder

責務:

- plan / patch / proposal / Generate。
- CODE 系 route に対する設計・実装案生成。

持たない責務:

- 破壊的操作の直接実行。
- WorkerExecutionService の実行責務。
- ToolRunner / PolicyEngine の policy 判断。

## 統合確認する契約

Phase6 で固定する契約は次の通り。

- CODE 系 route は `executeCodeViaShiro` から `CodeExecutor` へ委譲される。
- CODE 系 route は Shiro 経由 event を維持する。
- explicit `CODE1` / `CODE2` / `CODE3` / `CODE4` の Coder slot 対応を維持する。
- generic `CODE` fallback order は coder1 -> coder2 -> coder3 -> coder4 である。
- dynamic capability selection が route chain を壊さない。
- proposal interface 非対応 Coder は Generate path に戻る。
- nil / invalid proposal は Worker に渡らない。
- valid proposal だけ WorkerExecutionService に渡る。
- WorkerExecutionService に渡す jobID は `req.Task.JobID()`。
- Worker error は success に変換しない。
- Generate error は fallback success にしない。
- `CodeExecutionResponse.Handled` は success / failure ではなく proposal path 処理有無である。
- degraded route notice は fallback success ではない。
- Viewer event / execution log / response text を混同しない。
- route chain の event order を壊さない。
- unknown route は success response event を出さず error にする。
- pre-routing chat command は route decision を bypass する。

## 既存テストの棚卸し

### `message_orchestrator_code_path_test.go`

保証していること:

- `CODE1` は Shiro 経由 event を通り、coder1 response が shiro -> mio に返る。
- `CODE2` は Shiro 経由 event を通り、coder2 response が shiro -> mio に返る。
- `CODE4` は Shiro 経由 event を通り、coder4 response が shiro -> mio に返る。
- event order は mio -> shiro start、shiro -> coder start、coder -> shiro response、shiro -> mio response の順である。

不足している確認:

- `CODE` generic route の Shiro 経由 event。
- `CODE3` の Shiro 経由 event order は proposal path 側の event と合わせて追加確認が必要。

### `message_orchestrator_route_chain_contract_test.go`

保証していること:

- `message.received`、`routing.decision`、route execution start、response event の順序。
- pre-routing chat command が route decision を bypass すること。
- invalid proposal が WorkerExecutionService に到達しないこと。
- unknown route が success response event を出さないこと。

不足している確認:

- Worker execution error が success response に変換されないこと。
- `Handled` が final success 判定に使われていないこと。
- degraded route notice が success event として扱われないこと。

### `message_orchestrator_code3_test.go`

保証していること:

- `CODE3` proposal path で JSON patch が WorkerExecutionService により実行され、response に Plan / Execution Result / Risk が含まれること。
- Markdown patch の proposal path が実行されること。
- invalid proposal が error になること。
- coder3 がない場合に error になること。
- `formatExecutionResult` の success / partial failure 表示。

不足している確認:

- WorkerExecutionService が error を返す場合の route chain response / event。
- `CODE3` proposal path の event order を route chain レベルで固定すること。

### `code_executor_test.go`

保証していること:

- explicit `CODE1` が coder1 を選ぶこと。
- `CODE2` / `CODE3` proposal path が WorkerExecutionService に proposal を渡すこと。
- dynamic selection で `CODE3` 品質が選ばれた generic `CODE` が proposal path を使うこと。
- generic `CODE` が coder1 nil の場合に coder2 へ進むこと。
- CoderStatus が Generate success / Generate error の両方で release されること。
- degraded route notice が proposal handled success を意味しないこと。
- Worker に渡す jobID と proposal instance が Coder 生成のものと同じであること。

不足している確認:

- proposal interface 非対応 Coder が Generate path に戻ることの明示テスト。
- Worker execution error が wrapped error として返ること。
- Generate path error が fallback success に変換されないことの route chain レベル確認。

### `worker_execution_service_test.go`

保証していること:

- JSON patch / Markdown patch の successful execution。
- parse error。
- missing command failure classification。
- file edit create / update / delete / append / mkdir / rename / copy。
- shell command execution。
- protected file action。
- workspace restriction。
- StopOnError / ContinueOnError。
- parallel execution。
- git operation。
- AutoCommit。

不足している確認:

- CodeExecutor からの handoff が WorkerExecutionService 内部契約を壊さないことは CodeExecutor 側テストで確認する必要がある。
- route chain の success / error event とは別契約として扱う必要がある。

### domain routing tests

保証していること:

- route string。
- `Route.IsCoderRoute()`。
- `routing.NewDecision`。

不足している確認:

- `CODE4` が `IsCoderRoute` に含まれるかは現在の test table に含まれていないため、Phase6 以降で追加候補とする。

### domain proposal / patch tests

保証していること:

- proposal の plan / patch / risk / cost hint。
- proposal validity。
- proposal reconstruct。
- patch command value object。
- JSON / Markdown patch parse。
- unknown patch format error。
- execution result success / failure aggregation。

不足している確認:

- route chain ではなく domain 契約のため、Phase6 では必要最小限の参照に留める。

## 不足している契約テスト案

Phase6 で追加する候補は次の通り。

- generic `CODE` route が Shiro 経由 event を維持し、generic fallback selection 後も route が `CODE` のまま event に出ること。
- `CODE3` proposal path の event order が、mio -> shiro start、shiro -> coder start、coder -> shiro plan、shiro -> mio Worker start、shiro -> mio result の順であること。
- proposal interface 非対応 Coder は Generate path に戻り、WorkerExecutionService に到達しないこと。
- WorkerExecutionService が error を返した場合、`ProcessMessage` は error を返し、success response event に変換しないこと。
- Generate path error は `ProcessMessage` error になり、fallback success に変換されないこと。
- `Handled` は final success 判定に使われず、proposal path / Generate path の内部 contract として扱われること。
- degraded route notice は `agent.notice` であり、`agent.response` success として扱われないこと。
- `CODE4` が `Route.IsCoderRoute()` に含まれることを domain routing test に追加すること。
- route chain の response assembly が CODE / WORKER / CHAT の責務を混同しないこと。

## 小 Phase 案

### Phase6-0: 現在 route chain と既存テストの棚卸し

目的:

- route chain の現在構造と既存テストの保証範囲を文書化する。

対象範囲:

- `MessageOrchestrator.ProcessMessage`
- `executeTask`
- `executeRouteDirect`
- `executeCodeRoute`
- `executeCodeViaShiro`
- 既存 route chain tests

対象外:

- production code 変更。
- 新規契約テスト追加。

実装手順:

1. baseline test を実行する。
2. route chain の現在関数を `rg` で確認する。
3. 既存テストの保証範囲を `docs/refactor/` に記録する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
rg "ProcessMessage|executeTask|executeRouteDirect|executeCodeRoute|executeCodeViaShiro|ExecuteCode|ExecuteProposal" internal/application/orchestrator internal/application/service
```

完了条件:

- baseline test が成功している。
- 既存テストの保証範囲が記録されている。
- コード変更をしていない。

### Phase6-1: CODE route chain 契約テスト追加

目的:

- CODE 系 route が Shiro 経由 event を維持する契約を追加固定する。

対象範囲:

- `message_orchestrator_code_path_test.go`
- CODE / CODE3 / generic CODE route event order

対象外:

- route dispatch 実装変更。
- CodeExecutor selection 実装変更。

実装手順:

1. generic `CODE` route の Shiro relay event test を追加する。
2. `CODE3` proposal path の event order test を追加する。
3. gofmt を実行する。
4. 対象テストを実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:

- CODE 系 route の Shiro relay event が test で確認できる。
- route chain の event order が固定されている。
- production code を変更していない。

### Phase6-2: proposal handoff / invalid proposal 契約テスト追加

目的:

- proposal handoff と invalid proposal の境界を route chain レベルで追加固定する。

対象範囲:

- `message_orchestrator_route_chain_contract_test.go`
- `code_executor_test.go`

対象外:

- WorkerExecutionService 内部。
- proposal / patch format の意味変更。

実装手順:

1. proposal interface 非対応 Coder が Generate path に戻る test を追加する。
2. invalid proposal が WorkerExecutionService に到達しない既存 test を確認し、必要なら event / response 観点を追加する。
3. Worker に渡る jobID と proposal instance は既存 CodeExecutor test で維持する。
4. gofmt を実行する。
5. 対象テストを実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

完了条件:

- proposal interface 非対応 Coder の Generate fallback が確認できる。
- invalid proposal が Worker に渡らない契約が維持されている。
- valid proposal handoff 契約が維持されている。

### Phase6-3: error / degraded / Handled 契約テスト追加

目的:

- error、degraded、`Handled` を success と混同しない契約を追加固定する。

対象範囲:

- `message_orchestrator_route_chain_contract_test.go`
- `code_executor_test.go`
- `internal/domain/routing/route_test.go`

対象外:

- production code の挙動変更。
- Viewer JS / CSS。
- WorkerExecutionService 内部。

実装手順:

1. WorkerExecutionService error が success response に変換されない test を追加する。
2. Generate path error が fallback success に変換されない test を追加する。
3. degraded route notice が `agent.response` success ではない test を必要に応じて追加する。
4. `CODE4` が `Route.IsCoderRoute()` に含まれる test を追加する。
5. gofmt を実行する。
6. 対象テストを実行する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/routing ./cmd/picoclaw
git diff --check
```

完了条件:

- Worker error が success に変換されない。
- Generate error が fallback success に変換されない。
- degraded route notice と success response が混同されていない。
- `Handled` が success / failure の一般状態ではないことが test 名と assertion で追える。

### Phase6-4: route chain 統合確認文書と完了判定

目的:

- Phase6 の契約テスト追加と統合確認結果を文書化する。

対象範囲:

- Phase6 docs。
- Phase6 test diff。
- final verification。

対象外:

- Phase7 以降。

実装手順:

1. 全体 test を実行する。
2. `git diff --check` を実行する。
3. `git status --short` を確認する。
4. `docs/refactor/Phase6_完了判定.md` を作成する。

検証手順:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
```

完了条件:

- route chain の現在契約が docs と test で確認できる。
- Chat / Worker / Coder 責務境界を崩していない。
- production code の挙動変更をしていない。
- Phase6 docs / test diff が push 済みである。

## 実装手順

Phase6 の実装は次の順で進める。

1. baseline test を実行する。
2. 既存テストの保証範囲を Phase6-0 文書に記録する。
3. 不足契約テストを小さく追加する。
4. 原則として production code は変更しない。
5. production code 変更が必要な場合は、Phase6 の範囲を超える可能性として停止し報告する。
6. gofmt を実行する。
7. after test を実行する。
8. 各小 Phase ごとに docs / test diff を commit / push する。
9. 最後に Phase6 完了判定を作成し、commit / push する。

## 検証手順

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

route chain 確認:

```bash
rg "executeCodeViaShiro|ExecuteCode|WorkerExecutionService|ExecuteProposal|Handled|agent.start|agent.response|agent.notice" internal/application/orchestrator internal/application/service
```

## リスク

- 統合確認の名目で `MessageOrchestrator` の大規模分割を始める。
- route chain の挙動変更とテスト追加を混ぜる。
- WorkerExecutionService の内部責務へ踏み込む。
- Coder に実行責務を戻す。
- fallback / degraded を success として扱う。
- `Handled` を success / failure と混同する。
- Viewer event と execution log と response text を混同する。
- 既存テストの保証範囲を過大評価する。
- test double が本物の契約を覆い隠す。
- `executeAutonomousTask` の retry / verify flow と route-specific execution の境界を混同する。

## 完了条件

- `docs/refactor/Phase6_route_chain統合確認実装仕様.md` が作成されている。
- route chain の現在契約が文書化されている。
- Chat / Worker / Coder の責務境界が文書化されている。
- 既存テストの保証範囲が棚卸しされている。
- 不足している契約テスト案が書かれている。
- 小 Phase の順序が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- ユーザーが次に「Phase6 を実装してよいか」を判断できる。

## 次に確認すべきこと

Phase6 実装に入る前に、次を確認する。

1. Phase6 は production code 変更なしの契約テスト追加 Phase として進めてよいか。
2. generic `CODE` と `CODE3` proposal path の event order test を Phase6-1 に含める方針でよいか。
3. Worker error / Generate error / degraded notice / `Handled` の error contract test を Phase6-3 にまとめる方針でよいか。
