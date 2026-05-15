# Phase4 CodeExecutor 境界整理実装仕様

## Phase 4 の目的

Phase 4 は、CODE 系 route の中核である `CodeExecutor` の責務境界を整理する。

目的は次の通り。

- `DefaultCodeExecutor` の責務境界を整理する。
- Coder selection、proposal path、Generate path、Worker handoff を分けて追えるようにする。
- Coder が実行責務を持たず、Worker が実行主体である境界を維持する。
- degraded route notice を fallback success と混同しない。
- モジュール化と疎結合を最重要方針として、Coder selection や proposal execution を将来差し替えやすい構造にする。
- 挙動変更ではなく、現在契約を固定しながら段階的に構造を整理する。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

正本仕様では、Coder は plan / patch / risk / cost hint を生成し、Proposal の適用は `WorkerExecutionService.ExecuteProposal()` が担当する責務分離が明記されている。Phase 4 ではこの境界を崩さない。

`docs/codebase-map/` は CodeExecutor 周辺の結合点、ユースケース、潜在バグ確認に使うが、正本仕様ではない。判断が矛盾する場合は正本仕様と現在コードを優先し、`docs/archive/` は一次参照にしない。

## docs/codebase-map から見た注意点

`docs/codebase-map/` と Phase 2 / Phase 3 文書から、CodeExecutor 周辺には次の結合点がある。

- CODE 系 route は `MessageOrchestrator` から `executeCodeViaShiro` を経由して `CodeExecutor.ExecuteCode` に渡る。
- `DefaultCodeExecutor.ExecuteCode` は Coder を選択し、proposal path が可能な場合は `tryExecuteProposalPath` を使う。
- `tryExecuteProposalPath` は valid proposal の場合だけ `WorkerExecutionService.ExecuteProposal` に渡す。
- proposal path を使えない場合は Coder の Generate path に落ちる。
- fallback / degraded / error は成功表示の代替ではなく、notice / log / error として扱う。
- Worker execution の安全境界は Phase 3 で整理済みであり、Phase 4 で CodeExecutor 側へ戻さない。

## 対象範囲

Phase 4 の対象は次に限定する。

- `internal/application/orchestrator/code_executor.go`
- `internal/application/orchestrator/code_helpers.go`
- `internal/application/orchestrator/code_executor_test.go`
- `internal/application/orchestrator/message_orchestrator_code_path_test.go`
- `internal/application/orchestrator/message_orchestrator_route_chain_contract_test.go`
- `internal/application/orchestrator/coder_status.go`
- `internal/domain/capability/`
- `internal/domain/proposal/`
- `internal/domain/patch/`
- `internal/application/service/worker_execution_service.go` への接続点

扱う責務は次の通り。

- `DefaultCodeExecutor`
- `CodeExecutionRequest` / `CodeExecutionResponse`
- `codeTarget`
- `selectCoderForRoute`
- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `executeCoderGeneratePath`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`
- CodeExecutor event emission
- CoderStatus acquire / release
- `capability.SelectCoder` との接続点
- WorkerExecutionService への handoff

## 対象外

Phase 4 では次を変更しない。

- `MessageOrchestrator` の route chain。
- `WorkerExecutionService` の内部実行方式。
- `ToolRunner` / `PolicyEngine`。
- CoderAgent の LLM provider 挙動。
- proposal / patch format の意味。
- handler、DTO、SSE event。
- Viewer JS / CSS。
- IdleChat 契約。
- STT / TTS provider。
- runtime config の意味。
- 未追跡の `tests/`。

## 現在の責務整理

### DefaultCodeExecutor

`DefaultCodeExecutor` は CODE 系 route の実行入口である。

現在の責務:

- `CodeExecutionRequest` を受け取る。
- route に応じて Coder を選択する。
- CoderStatus がある場合、汎用 CODE route で Coder busy state を acquire / release する。
- degraded route がある場合、`agent.notice` を emit する。
- `agent.start` event を emit する。
- proposal path を使うべきか判断する。
- proposal path が処理した場合、その response / error を返す。
- proposal path が使えない場合、Generate path を実行する。

DefaultCodeExecutor が持ってはいけない責務:

- file edit / shell / git operation の直接実行。
- WorkerExecutionService の command dispatch。
- ToolRunner / PolicyEngine の policy 判断。
- Coder provider の挙動変更。
- Viewer 表示契約の意味決定。

### Coder selection

現在の Coder selection は `selectCoderForRoute` に集中している。

静的 selection:

- CODE1 は coder1。
- CODE2 は coder2。
- CODE3 は coder3。
- CODE4 は coder4。
- CODE は coder1 -> coder2 -> coder3 -> coder4 の順で利用可能な Coder を選ぶ。
- CoderStatus がある場合、busy な Coder は skip し、acquire 成功時に release 関数を `codeTarget` に保持する。

動的 selection:

- `coderCaps` が nil でない場合、`capability.SelectCoder` を使う。
- selected coder が初期化されていない場合は error。
- degraded route がある場合、`codeTarget.degradedRoute` に保持する。

Coder selection が持ってはいけない責務:

- proposal の生成。
- Worker handoff。
- response formatting。
- event response の組み立て。

### proposal path

現在の proposal path は `tryExecuteProposalPath` に集中している。

現在の責務:

- selected Coder が `CoderAgentWithProposal` を満たすか確認する。
- `GenerateProposal` を呼ぶ。
- proposal generation error を event と error にする。
- nil / invalid proposal を event と error にし、WorkerExecutionService に渡さない。
- valid proposal の plan event を emit する。
- Worker execution start event を emit する。
- `WorkerExecutionService.ExecuteProposal(ctx, req.Task.JobID(), p)` を呼ぶ。
- Worker error を event と error にする。
- `formatExecutionResult` で response を整形する。
- `CodeExecutionResponse{Handled: true}` を返す。

proposal path が持ってはいけない責務:

- Coder selection。
- patch command 実行。
- protected file 判定。
- ToolRunner / PolicyEngine の policy 判断。

### Generate path

現在の Generate path は `executeCoderGeneratePath` に集中している。

現在の責務:

- selected Coder の `Generate` を呼ぶ。
- Coder error を event と error にする。
- Coder response を coder -> shiro と shiro -> mio の event として emit する。
- `CodeExecutionResponse{Handled: false}` を返す。

Generate path が持ってはいけない責務:

- proposal validation。
- Worker handoff。
- Worker result formatting。

### Worker handoff

Worker handoff は valid proposal の場合だけ発生する。

契約:

- `p == nil || !p.IsValid()` の場合、WorkerExecutionService に渡さない。
- Worker に渡す `jobID` は `req.Task.JobID()`。
- Worker に渡す proposal は Coder が生成した proposal instance。
- Worker execution の内部は Phase 3 の境界に従い、CodeExecutor は実行内容を持たない。

### event emission

CodeExecutor は orchestration / Viewer 向け event を emit する。

現在の主な event:

- `agent.notice`: degraded route notice。
- `agent.start`: Shiro 経由実行開始、Coder 呼び出し開始、Worker execution 開始。
- `agent.response`: Coder response、invalid proposal、Worker result、Worker error。

event emission は execution result や Worker log の代替ではない。

### fallback / degraded route notice

CODE route の coder1 -> coder2 -> coder3 -> coder4 は可用性選択であり、成功保証ではない。

degraded route notice は Coder selection の結果通知であり、fallback success ではない。degraded route がある場合でも、proposal generation、Worker execution、Generate path はそれぞれ失敗し得る。

### CodeExecutionResponse.Handled

`Handled` は proposal path で処理されたかを表す。

- `Handled: true`: proposal path が Worker handoff まで含めて処理した。
- `Handled: false`: Generate path の通常 response。

`Handled` は success / failure の一般状態ではない。

## 提案する Phase 4 の小 Phase

### Phase 4-0: 現在契約の固定

目的:
- CodeExecutor の現在契約を文書とテストで固定する。

対象範囲:
- explicit CODE1 / CODE2 / CODE3 / CODE4 selection。
- generic CODE fallback chain。
- dynamic capability selection。
- CoderStatus acquire / release。
- proposal path。
- invalid proposal が Worker に到達しない契約。
- event order。

対象外:
- 実装分割。
- WorkerExecutionService 内部。
- Coder provider。

入力:
- `CodeExecutionRequest`
- `routing.Route`
- `CoderAgent`
- `CoderAgentWithProposal`
- `capability.CoderCapability`
- `CoderStatus`

出力:
- `CodeExecutionResponse`
- error
- CodeExecutor event

副作用:
- CoderStatus acquire / release。
- event emission。
- Worker handoff は valid proposal の場合だけ発生する。

永続化:
- CodeExecutor 自体は永続化しない。
- WorkerExecutionService に渡った後の副作用は Phase 3 の対象。

ログ:
- coder selected / skip。
- code handoff。
- degraded route。

エラー契約:
- explicit route の coder missing は error。
- CODE route で全 Coder unavailable は error。
- CoderStatus ありで全 busy / unavailable は error。
- invalid proposal は Worker に渡さず error。
- Worker execution error は `worker execution failed`。

変更してはいけない既存挙動:
- route と coder の対応。
- CODE fallback chain。
- dynamic selection の `capability.SelectCoder` 使用。
- proposal path の handoff 条件。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

完了条件:
- 現在契約が docs/refactor に記録されている。
- 不足している契約テストが追加されている。
- 既存挙動を変えていない。

### Phase 4-1: Coder selection 境界整理

目的:
- Coder selection を proposal execution から分けて追える状態にする。

対象範囲:
- `selectCoderForRoute`
- `explicitCodeRouteTarget`
- `systemPromptForRoute`
- `coderByName`
- `codeTarget`
- CoderStatus acquire / release
- `capability.SelectCoder` 接続

対象外:
- proposal path。
- Generate path。
- Worker handoff。
- capability selection の意味変更。

入力:
- route。
- coder slots。
- coder capabilities。
- CoderStatus。

出力:
- `codeTarget`
- error

副作用:
- CoderStatus acquire。
- log。

永続化:
- なし。

ログ:
- selected / skipped / degraded。

エラー契約:
- explicit coder missing は error。
- selected coder missing は error。
- all unavailable / busy は error。

変更してはいけない既存挙動:
- static route mapping。
- generic CODE fallback order。
- dynamic selection の degraded route。
- release 関数の設定。

実装手順:
- baseline test を実行する。
- selection helper を必要最小限で分ける。
- proposal path には触れない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:
- selection の入力、出力、副作用が関数名で追える。
- CoderStatus release 漏れを起こしていない。

### Phase 4-2: proposal path 境界整理

目的:
- proposal generation、validation、Worker handoff、result formatting を分けて追える状態にする。

対象範囲:
- `shouldUseProposalPath`
- `tryExecuteProposalPath`
- `CoderAgentWithProposal`
- proposal validation
- Worker handoff
- Worker result formatting

対象外:
- WorkerExecutionService 内部。
- patch parser。
- Coder selection。
- Generate path。

入力:
- `CodeExecutionRequest`
- `codeTarget`
- `proposal.Proposal`

出力:
- `CodeExecutionResponse`
- handled bool
- error

副作用:
- Coder `GenerateProposal` 呼び出し。
- WorkerExecutionService handoff。
- event emission。

永続化:
- CodeExecutor 自体は永続化しない。
- WorkerExecutionService 側の副作用は Phase 3 の契約に従う。

ログ:
- proposal path の handoff に必要な既存 log を維持する。

エラー契約:
- Coder が proposal interface を持たない場合は handled false。
- proposal generation error は handled true + error。
- nil / invalid proposal は handled true + error。
- Worker execution error は handled true + wrapped error。

変更してはいけない既存挙動:
- invalid proposal を Worker に渡さない。
- valid proposal だけ Worker に渡す。
- proposal path response は `Handled: true`。

実装手順:
- baseline test を実行する。
- proposal path 内部を generate / validate / handoff / format helper に分ける。
- WorkerExecutionService 内部に踏み込まない。
- event 内容を変えない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
git diff --check
```

完了条件:
- proposal validation と Worker handoff の境界が関数名で追える。
- Phase 3 の Worker handoff 契約を壊していない。

### Phase 4-3: Generate path / event emission 境界整理

目的:
- Generate path と CodeExecutor event emission の責務を明確にする。

対象範囲:
- `executeCoderGeneratePath`
- `emit`
- degraded route notice
- `agent.start` / `agent.response` emission

対象外:
- Viewer JS / CSS。
- SSE event schema。
- MessageOrchestrator route chain。
- proposal path。

入力:
- `CodeExecutionRequest`
- `codeTarget`
- generated text
- error

出力:
- `CodeExecutionResponse`
- error
- CodeExecutor event

副作用:
- Coder `Generate` 呼び出し。
- event emission。

永続化:
- なし。

ログ:
- degraded route log。
- selected coder log は selection 側に置く。

エラー契約:
- Coder Generate error は event と error。
- Generate path response は `Handled: false`。

変更してはいけない既存挙動:
- `agent.start` の from / to / route。
- `agent.response` の from / to / route。
- degraded route notice の意味。
- Generate path response。

実装手順:
- baseline test を実行する。
- event helper を必要最小限に分ける。
- event type / from / to / content / route を変えない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:
- Generate path と event emission の責務が読める。
- Viewer-facing event 契約を変えていない。

### Phase 4-4: CodeExecutionResponse / result contract 整理

目的:
- CodeExecutionResponse と result formatting の意味を明確にする。

対象範囲:
- `CodeExecutionResponse`
- `formatExecutionResult`
- `executeCoderGeneratePath`
- `tryExecuteProposalPath`

対象外:
- WorkerExecutionService result 内部。
- Viewer display rendering。
- proposal / patch format。

入力:
- generated response。
- proposal execution formatted response。
- `PatchExecutionResult`

出力:
- `CodeExecutionResponse`

副作用:
- なし。response assembly helper は副作用を持たない。

永続化:
- なし。

ログ:
- response assembly helper はログを出さない。

エラー契約:
- `Handled` は proposal path 処理有無であり、成功可否ではない。
- error は error として返し、fallback success にしない。

変更してはいけない既存挙動:
- `Handled: true` は proposal path。
- `Handled: false` は Generate path。
- result Markdown の section 構造。

実装手順:
- baseline test を実行する。
- response assembly helper を必要なら追加する。
- `Handled` の意味を変えない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

完了条件:
- CodeExecutionResponse の意味が docs とコードで追える。
- success / failure と `Handled` を混同していない。

### Phase 4-5: 完了判定

目的:
- Phase 4 が CodeExecutor の境界整理として完了したか判定する。

対象範囲:
- Phase 4 の docs/refactor 文書。
- Phase 4 の実装差分。
- Phase 4 の test 差分。

対象外:
- Phase 5 以降。

入力:
- commit 一覧。
- test result。
- git status。
- Phase 4 文書。

出力:
- `docs/refactor/Phase4_完了判定.md`

副作用:
- docs/refactor への Markdown 追加。
- git commit。

永続化:
- git repository。

ログ:
- 検証コマンドと結果を完了判定文書へ記録する。

エラー契約:
- test failure、仕様矛盾、未 Push 差分がある場合は完了にしない。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/capability ./internal/domain/patch ./cmd/picoclaw
git diff --check
git status --short
```

完了条件:
- CodeExecutor / Coder selection / proposal path / Generate path / Worker handoff の責務差が docs/refactor に記録されている。
- Coder が実行責務を持たない境界が維持されている。
- invalid proposal が Worker に渡らない契約が維持されている。
- fallback / degraded route notice を success と混同していない。
- 対象パッケージのテストが成功している。
- すべての Phase 4 文書と実装差分が Push 済みである。

## モジュール化・疎結合の方針

Phase 4 では、単にファイルを分けるだけではモジュール化とは扱わない。

優先する境界:

- Coder selection: route / capability / busy state から `codeTarget` を選ぶ。
- proposal path: proposal generation / validation / Worker handoff を扱う。
- Generate path: Coder の通常 Generate response を扱う。
- event emission: orchestration / Viewer 向け通知だけを扱う。
- response contract: `CodeExecutionResponse` と `Handled` の意味を固定する。

禁止する整理:

- Coder selection と proposal execution を混ぜる。
- proposal validation と Worker handoff を曖昧にする。
- CodeExecutor を巨大 manager にする。
- WorkerExecutionService の実行責務を CodeExecutor に戻す。
- degraded route notice を fallback success として扱う。
- 「便利だから共有する」「似ているからまとめる」だけの共通化をする。

差し替え可能性の確認観点:

- Coder selection を別実装にしても proposal path の契約が変わらない。
- proposal execution を別実装にしても Worker handoff 条件が変わらない。
- WorkerExecutionService を別実装にしても CodeExecutor は `ExecuteProposal` 契約だけに依存する。
- event emission を整理しても Viewer-facing event の type / from / to / route が変わらない。

## 検証方針

Phase 4 の基本検証は次を使う。

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
```

Coder selection に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/domain/capability ./cmd/picoclaw
```

Worker handoff に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/patch ./cmd/picoclaw
```

共通:

```bash
git diff --check
```

実ブラウザ確認は原則不要。ただし Viewer event、runtime route、Viewer-facing API に触った場合は追加確認する。

## リスク

- Coder selection と proposal path を混ぜる。
- Coder が直接実行する構造に戻る。
- invalid proposal を Worker に渡す。
- CODE route の fallback chain を成功保証として扱う。
- degraded route notice を fallback success と誤解する。
- CoderStatus release 漏れで busy state が残る。
- event emission の順序や from / to を壊す。
- Phase 2 の route chain 契約を壊す。
- Phase 3 の Worker handoff 契約を壊す。
- `Handled` を success / failure と混同する。

## Phase 4 全体の完了条件

- `docs/refactor/Phase4_CodeExecutor境界整理実装仕様.md` が作成されている。
- Phase 4 の目的、対象、対象外が明記されている。
- CodeExecutor / Coder selection / proposal path / Worker handoff の境界が説明されている。
- 小 Phase の移行順が明記されている。
- 各小 Phase の検証条件が書かれている。
- コード変更は行っていない。
- ユーザーが次に「Phase 4 を実装してよいか」を判断できる。

## 次に確認すべきこと

Phase 4 実装に入る前に、最初の小 Phase を次の方針で進める。

1. Phase 4-0 として既存 CodeExecutor 契約テストを確認する。
2. CoderStatus acquire / release、degraded route notice、invalid proposal handoff の不足テストだけを追加する。
3. その後、Coder selection の境界整理に入る。

推奨は、まず CoderStatus release 漏れを防ぐ契約テストから追加することである。selection の副作用が Phase 4 の最も壊れやすい点だからである。
