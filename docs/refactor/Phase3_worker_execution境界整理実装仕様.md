# Phase3 worker execution 境界整理実装仕様

## Phase 3 の目的

Phase 3 は、Phase 2 で明確化した Chat / Worker / Coder route chain の次段階として、Worker execution chain の責務境界を整理する。

目的は次の通り。

- WorkerExecutionService / ToolRunner / PolicyEngine の役割を明確にする。
- Coder proposal を Worker が実行する境界を壊さない。
- patch / proposal 実行、実行ログ、実行結果、error contract の責務を分ける。
- モジュール化と疎結合を最重要方針として、将来差し替え可能な Worker execution 境界を作る。
- 挙動変更ではなく、現在契約を固定しながら段階的に構造を整理する。

## 正本仕様との関係

実装判断の一次参照は `docs/01_正本仕様/実装仕様.md` とする。

`docs/codebase-map/` は Worker execution 周辺の結合点、潜在バグ、ユースケース確認に使うが、正本仕様ではない。codebase-map と現在コードが違う場合は現在コードを確認し、判断が正本仕様と矛盾しそうな場合は作業を止める。

`docs/archive/` は一次参照にしない。

## docs/codebase-map から見た注意点

`docs/codebase-map/` では Worker execution 周辺について次の結合点が示されている。

- `CodeExecutor` から `WorkerExecutionService.ExecuteProposal` へ Coder proposal を渡す。
- `WorkerExecutionService` は `PatchCommand` を実行し、file edit / shell / git operation、protected file check、実行結果を扱う。
- `ToolRunner` と `PolicyEngine` は tool execution と security policy の境界として存在する。
- `WorkerExecutionService` と `ToolRunner` の安全境界は重なって見えるが、前者は Coder proposal の patch 実行、後者は tool request の実行で文脈が異なる。
- 潜在バグとして、Worker 実行の file / shell / git 操作が集中し、保護判定やログ変更時に安全境界が弱まるリスクがある。

Phase 3 では、この重なりを「便利だから統合する」のではなく、入力、出力、副作用、ログ、エラー契約の違いで分けて扱う。

## 対象範囲

Phase 3 の対象は次に限定する。

- `internal/application/orchestrator/code_executor.go`
- `internal/application/service/worker_execution_service.go`
- `internal/domain/proposal/`
- `internal/domain/patch/`
- `internal/infrastructure/tools/`
- `internal/infrastructure/security/policy_engine.go`
- `internal/infrastructure/security/policy_runner.go`
- `internal/domain/execution/`
- 関連する `*_test.go`

扱う責務は次の通り。

- WorkerExecutionService
- ToolRunner
- PolicyEngine
- proposal / patch execution
- 実行ログ
- 実行結果
- error contract
- CodeExecutor から WorkerExecutionService への接続点

## 対象外

Phase 3 では次を変更しない。

- MessageOrchestrator の route chain。
- Coder の proposal 生成ロジック。
- Tool の実処理内容。
- Policy の意味。
- Viewer JS / CSS。
- STT / TTS provider。
- LLM provider。
- runtime config の意味。
- handler、DTO、SSE event。
- IdleChat 契約。
- `docs/archive/`。
- 未追跡の `tests/`。

## 現在の責務整理

### CodeExecutor

`DefaultCodeExecutor` は CODE 系 route の実行入口である。

現在の責務:

- route に応じて Coder を選択する。
- CODE1 / CODE2 / CODE3 / CODE4 または CODE3 品質へ縮退した route で proposal path を試す。
- Coder が `CoderAgentWithProposal` を満たす場合、`GenerateProposal` を呼ぶ。
- proposal が nil または invalid の場合、WorkerExecutionService に渡さず error にする。
- valid proposal の場合だけ `WorkerExecutionService.ExecuteProposal` を呼ぶ。
- Worker 実行結果を `formatExecutionResult` で user-facing response に整形する。
- `agent.start` / `agent.response` の event を発火する。

CodeExecutor が持ってはいけない責務:

- file edit / shell / git operation の直接実行。
- protected file 判定。
- policy 判定。
- patch command の実行順序制御。

### WorkerExecutionService

`WorkerExecutionService` は Coder proposal の patch を実行する Application service である。

現在の責務:

- `proposal.Proposal` から `p.Patch()` を取り出す。
- `patch.ParsePatch` で `[]patch.PatchCommand` に変換する。
- 実行前サマリを表示する。
- config に応じて pre / post auto-commit を行う。
- sequential / parallel execution を選択する。
- `PatchCommand.Type` に応じて file edit / shell command / git operation を dispatch する。
- protected file check を行う。
- command result を `patch.PatchExecutionResult` に集約する。
- `FailureKind` / `FailureReason` / `Retryable` を設定する。

WorkerExecutionService が持ってはいけない責務:

- Coder の選択。
- Coder proposal の生成。
- Viewer 表示用 event の設計。
- ToolRunner の tool metadata 管理。
- PolicyEngine の policy 意味解決。

### ToolRunner

`ToolRunner` は tool request を実行する Infrastructure 側の runner である。

現在の責務:

- shell / file_read / file_write / file_list / web_search / subagent / register_tool を登録する。
- V1 / V2 tool execution を提供する。
- tool metadata を返す。
- 引数 validation、path validation、timeout、retry、dry-run metadata を扱う。
- web_search cache や ToolRegistry など tool 実行に必要な adapter 詳細を扱う。

ToolRunner が持ってはいけない責務:

- Coder proposal の解釈。
- WorkerExecutionService の patch command 実行順序制御。
- policy 判定の意味そのもの。
- Worker execution result の summary / failure metadata 生成。

### PolicyEngine / PolicyRunner

`PolicyEngine` は tool execution の policy 判定を行う。

現在の責務:

- `execution.Action` を受け取り、`execution.PolicyDecision` を返す。
- deny command、workspace 外 file_write、network scope を判定する。
- allow / deny と reason / matched rule を返す。

`PolicyRunner` は `RunnerV2` を policy 適用付きで包む。

現在の責務:

- inner `RunnerV2` の tool metadata を確認する。
- `execution.Action` を作る。
- `execution.Service.RequestToolExecution` を通じて policy と実行をつなぐ。
- denied を `tool.ToolResponse` の permission error として返す。

PolicyEngine / PolicyRunner が持ってはいけない責務:

- file / shell / git の実行そのもの。
- Coder proposal の valid / invalid 判定。
- PatchExecutionResult の構築。
- Viewer 表示契約。

### proposal / patch domain object

`proposal.Proposal` は Coder からの plan / patch / risk / cost hint を表す値オブジェクトである。

`patch.PatchCommand` は Worker が実行する command の値オブジェクトである。

`patch.PatchExecutionResult` は Worker 実行結果を表す値オブジェクトである。

Domain object が持ってよい責務:

- 値の保持。
- basic validation。
- patch parse。
- result aggregation。

Domain object が持ってはいけない責務:

- filesystem / shell / git の副作用。
- policy 判定。
- Viewer event 発火。
- runtime config 参照。

### ログと実行結果

現在は WorkerExecutionService が `fmt.Printf` による実行ログを出し、CodeExecutor が orchestrator event を emit する。

Phase 3 では、ログ、event、result を混同しない。

- ログ: Worker 内部の実行観測。
- event: Viewer / orchestration 向けの状態通知。
- result: `PatchExecutionResult` として上位へ返す契約。
- report: autonomous execution の evidence。Worker patch result と混同しない。

## 提案する Phase 3 の小 Phase

### Phase 3-0: 現在契約の固定

目的:
- Worker execution chain の現在契約をテストと文書で固定する。

対象範囲:
- `CodeExecutor` から `WorkerExecutionService.ExecuteProposal` への handoff。
- invalid proposal が Worker に到達しない契約。
- `WorkerExecutionService.ExecuteProposal` の parse / execute / result 契約。
- protected file、shell failure、git operation、parallel execution の既存テスト。

対象外:
- 実装分割。
- policy 意味変更。
- ToolRunner 実行内容変更。

入力:
- `CodeExecutionRequest`
- `task.Task`
- `proposal.Proposal`
- `config.WorkerConfig`

出力:
- `CodeExecutionResponse`
- `patch.PatchExecutionResult`
- error

副作用:
- WorkerExecutionService 内の file edit / shell / git operation。
- auto-commit が有効な場合の git commit。
- CodeExecutor の event emission。

永続化:
- WorkerExecutionService 自体は専用 DB 永続化を持たない。
- auto-commit 有効時のみ git repository へ commit が作られる。

ログ:
- CodeExecutor の `log.Printf`。
- WorkerExecutionService の `fmt.Printf`。

エラー契約:
- invalid proposal は Worker に渡さず error。
- patch parse error は `patch parse error` として返す。
- command failure は `PatchExecutionResult` に失敗として記録する。
- WorkerExecutionService の command failure は原則 result に集約し、parse / pre-commit など入口失敗は error とする。

変更してはいけない既存挙動:
- Coder が file edit / shell / git を直接実行しない。
- CODE 系の proposal path は valid proposal のみ Worker に渡す。
- fallback を成功扱いしない。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/infrastructure/tools ./internal/infrastructure/security ./cmd/picoclaw
git diff --check
```

完了条件:
- 現在契約が docs/refactor に記録されている。
- 既存テストが成功している。
- 必要なら契約テストが追加されている。

### Phase 3-1: WorkerExecutionService の入力・出力・副作用整理

目的:
- WorkerExecutionService の入口、結果、実行副作用を関数名で追える状態にする。

対象範囲:
- `ExecuteProposal`
- `executeSequential`
- `executeParallel`
- `executeCommand`
- `executeFileEdit`
- `executeShellCommand`
- `executeGitOperation`
- `autoCommitChanges`
- `classifyExecutionFailure`

対象外:
- patch format 変更。
- file / shell / git の挙動変更。
- ToolRunner への統合。

入力:
- `context.Context`
- `task.JobID`
- `*proposal.Proposal`
- `config.WorkerConfig`
- `[]patch.PatchCommand`

出力:
- `*patch.PatchExecutionResult`
- command output
- error

副作用:
- filesystem mutation。
- shell command 実行。
- git operation。
- auto-commit。

永続化:
- filesystem。
- git repository。

ログ:
- Worker execution summary。
- command success / failure。
- phase progress。
- auto-commit result。

エラー契約:
- parse error は result ではなく error。
- command execution error は `CommandResult` に入る。
- StopOnError は以後の実行を止める。
- parallel mode は file_edit -> shell_command -> git_operation の phase order を維持する。

変更してはいけない既存挙動:
- protected file check。
- workspace 外書き込み拒否。
- StopOnError の意味。
- AutoCommit の pre / post commit timing。

実装手順:
- baseline test を実行する。
- `ExecuteProposal` の中から parse、pre-commit、execution strategy selection、post-commit、summary / failure classification を小関数へ分ける。
- `executeCommand` の dispatch は file / shell / git の差を隠さない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/service ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:
- `ExecuteProposal` が Worker execution の composition root として読める。
- 入力、出力、副作用、エラー契約が関数名で追える。
- WorkerExecutionService が巨大 helper へ置き換わっていない。

### Phase 3-2: ToolRunner 境界整理

目的:
- ToolRunner を tool execution adapter として読みやすくし、WorkerExecutionService と混同しない境界を明確にする。

対象範囲:
- `internal/infrastructure/tools/runner.go`
- `ToolRunnerConfig`
- tool registration。
- V1 / V2 execution。
- tool metadata。
- validation / timeout / retry middleware。

対象外:
- tool の外部仕様変更。
- ToolRegistry の永続化仕様変更。
- WorkerExecutionService への統合。

入力:
- tool name。
- `map[string]any` arguments。
- `ToolRunnerConfig`。

出力:
- V1: string / error。
- V2: `*tool.ToolResponse` / error。
- tool metadata list。

副作用:
- shell / file / web_search / subagent / register_tool の実行。
- web_search cache write。
- ToolRegistry write。

永続化:
- web_search cache。
- ToolRegistry。
- file_write の対象ファイル。

ログ:
- 既存 tool execution のログがある場合は維持する。
- Phase 3 では新しい監査ログ形式を導入しない。

エラー契約:
- unknown tool は unknown tool error。
- V2 は `ToolResponse` の error code を優先する。
- validation / timeout / not found / permission denied の分類を維持する。

変更してはいけない既存挙動:
- registered tool name。
- ToolResponse error code。
- DisableWebSearch の意味。
- AllowedShellCommands / AllowedWritePaths の意味。

実装手順:
- baseline test を実行する。
- registration と metadata の整理対象を文書化する。
- 挙動変更なしで必要最小限の関数分割を行う。
- WorkerExecutionService へ tool 実行を移さない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tools ./internal/infrastructure/security ./internal/application/orchestrator ./cmd/picoclaw
git diff --check
```

完了条件:
- ToolRunner の責務が tool execution adapter として説明できる。
- Worker patch execution と tool request execution を混同していない。
- V1 / V2 の互換契約が維持されている。

### Phase 3-3: PolicyEngine 境界整理

目的:
- PolicyEngine を policy decision の純粋な境界として扱い、tool 実行の副作用から分離する。

対象範囲:
- `internal/infrastructure/security/policy_engine.go`
- `internal/infrastructure/security/policy_runner.go`
- `internal/domain/execution/`
- policy 関連 tests。

対象外:
- policy rule の意味変更。
- deny / allow の新規ルール追加。
- ToolRunner の実処理変更。
- WorkerExecutionService の protected file check 変更。

入力:
- `execution.Action`
- `PolicyConfig`
- tool name。
- tool args。

出力:
- `execution.PolicyDecision`
- `tool.ToolResponse` error for denied action。
- execution record。

副作用:
- PolicyEngine は副作用を持たない。
- PolicyRunner / execution.Service は record 保存と inner runner 実行を行う。

永続化:
- execution repository への record 保存。

ログ:
- 既存 record / repository に従う。
- Viewer event とは混同しない。

エラー契約:
- deny は permission error response。
- unknown tool は error。
- policy engine nil / runner nil は constructor error。

変更してはいけない既存挙動:
- deny shell signature。
- workspace 外 file_write deny。
- network scope deny / allowlist。
- default profile。

実装手順:
- baseline test を実行する。
- PolicyEngine と PolicyRunner の責務差をコードコメントまたは関数境界で明確にする。
- policy 判定から tool 実行副作用を呼ばない。
- policy rule の意味を変えない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/security ./internal/domain/execution ./internal/infrastructure/tools ./cmd/picoclaw
git diff --check
```

完了条件:
- PolicyEngine は action -> decision の責務として説明できる。
- PolicyRunner は policy + runner + repository の adapter として説明できる。
- ToolRunner が policy 判断を抱え込んでいない。

### Phase 3-4: execution result / error contract 整理

目的:
- Worker execution の result / error / log / event / report の違いを明確にする。

対象範囲:
- `patch.PatchExecutionResult`
- `patch.CommandResult`
- WorkerExecutionService の failure metadata。
- CodeExecutor の `formatExecutionResult`。
- Worker execution tests。

対象外:
- Viewer 表示仕様。
- autonomous execution report schema。
- LLM provider retry。
- fallback 成功化。

入力:
- `PatchExecutionResult`
- `CommandResult`
- WorkerExecutionService の execution result。

出力:
- user-facing formatted response。
- failure metadata。
- command result list。

副作用:
- なし。result formatting は副作用を持たない。

永続化:
- なし。

ログ:
- result とログは混同しない。
- `PatchExecutionResult` はログ文字列ではなく実行結果契約として扱う。

エラー契約:
- parse / pre-commit failure は error。
- command failure は result 内 failure。
- failure kind は retry 判断のために保持する。
- empty / invalid proposal は Worker result ではなく proposal generation error。

変更してはいけない既存挙動:
- `formatExecutionResult` の user-facing summary。
- `FailureKind` / `FailureReason` / `Retryable` の意味。
- fallback を成功扱いしない。

実装手順:
- baseline test を実行する。
- result assembly と error classification の責務を確認する。
- 必要なら formatting helper と failure helper の境界を文書化または小関数化する。
- event / report / log へ意味を移さない。
- gofmt を実行する。
- after test を実行する。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/patch ./cmd/picoclaw
git diff --check
```

完了条件:
- result / error / log / event / report の違いが説明できる。
- failure metadata が retry 判断に使える形で維持されている。
- command failure が成功 response にすり替わっていない。

### Phase 3-5: 完了判定

目的:
- Phase 3 の構造整理が Worker execution chain の責務境界を明確化したか判定する。

対象範囲:
- Phase 3 で作成した docs/refactor 文書。
- Phase 3 で変更した実装差分。
- Phase 3 で追加・更新した tests。

対象外:
- Phase 4 以降の設計。
- Worker execution 以外の refactor。

入力:
- Phase 3 の commit 一覧。
- test result。
- git status。
- docs/refactor の Phase 3 文書。

出力:
- `docs/refactor/Phase3_完了判定.md`
- Phase 4 に進む前の確認事項。

副作用:
- docs/refactor への Markdown 追加。

永続化:
- git commit。

ログ:
- 実行した検証コマンドと結果を完了判定文書へ記録する。

エラー契約:
- テスト失敗や仕様矛盾がある場合は Phase 3 完了にしない。

変更してはいけない既存挙動:
- Phase 2 の route chain 契約。
- Worker execution の安全境界。
- policy 判定。

検証手順:
```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/infrastructure/tools ./internal/infrastructure/security ./internal/domain/patch ./internal/domain/execution ./cmd/picoclaw
git diff --check
git status --short
```

完了条件:
- WorkerExecutionService / ToolRunner / PolicyEngine の責務差が docs/refactor に記録されている。
- Coder proposal -> Worker execution の境界が維持されている。
- ToolRunner と PolicyEngine が巨大 service / helper に統合されていない。
- fallback を正常系として扱っていない。
- 対象パッケージのテストが成功している。
- すべての Phase 3 文書と実装差分が Push 済みである。

## モジュール化・疎結合の方針

Phase 3 では、単にファイルを分けるだけではモジュール化とは扱わない。

優先する境界:

- interface: `WorkerExecutionService`、`tool.RunnerV2` など差し替え点。
- contract: proposal valid / invalid、PatchCommand、PolicyDecision、ToolResponse。
- DTO / value object: `CodeExecutionRequest`、`CodeExecutionResponse`、`PatchExecutionResult`、`execution.Action`。
- adapter: ToolRunner、PolicyRunner、filesystem / shell / git / web_search / repository。

禁止する整理:

- WorkerExecutionService を巨大 manager にする。
- ToolRunner が PolicyEngine の判断を抱え込む。
- PolicyEngine が tool 実行の副作用を持つ。
- WorkerExecutionService と ToolRunner を「似ているから」統合する。
- 共通処理を「便利だから共有する」だけで抽出する。
- fallback を成功 response として扱う。

差し替え可能性の確認観点:

- WorkerExecutionService を別実装に差し替えても、CodeExecutor の proposal handoff 契約が変わらない。
- ToolRunner を別 runner に差し替えても、PolicyRunner は `RunnerV2` 契約で接続できる。
- PolicyEngine を別 policy 実装に差し替えても、tool execution の副作用を持ち込まない。
- PatchCommand の command type を増やす場合、WorkerExecutionService の dispatch と test が明示される。

## 検証方針

Phase 3 の基本検証は次を使う。

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./cmd/picoclaw
```

Worker / Tool / Policy に触った場合:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/service ./internal/infrastructure/tools ./internal/infrastructure/security ./internal/domain/patch ./internal/domain/execution
```

共通:

```bash
git diff --check
```

実ブラウザ確認は原則不要。ただし Viewer event、runtime route、Viewer-facing API に触った場合は追加確認する。

## リスク

- Coder が直接実行する構造に戻る。
- WorkerExecutionService が巨大 service になる。
- ToolRunner と PolicyEngine の責務が混ざる。
- policy 判定を bypass する。
- WorkerExecutionService の protected file check が弱まる。
- patch 実行結果とログを混同する。
- command failure を success response に丸める。
- fallback を成功扱いする。
- Phase 2 の route chain 契約を壊す。
- ToolRunner と WorkerExecutionService を誤って同一実行基盤として統合する。

## Phase 3 全体の完了条件

- `docs/refactor/Phase3_worker_execution境界整理実装仕様.md` が作成されている。
- Phase 3 の目的、対象、対象外が明記されている。
- WorkerExecutionService / ToolRunner / PolicyEngine の境界が説明されている。
- 小 Phase の移行順が明記されている。
- 各小 Phase の検証条件が書かれている。
- Coder proposal を Worker が実行する責務境界が維持されている。
- fallback を正常系として扱わない方針が維持されている。
- コード変更は行っていない。
- ユーザーが次に「Phase 3 を実装してよいか」を判断できる。

## 次に確認すべきこと

Phase 3 実装に入る前に、最初の小 Phase を次のどちらにするか確認する。

1. Phase 3-0 として契約テストを先に追加する。
2. 既存テストで契約が十分か確認し、足りない観点だけを Phase 3-0 に追加する。

推奨は 2 である。既存の `worker_execution_service_test.go`、`code_executor_test.go`、`policy_engine_test.go`、`policy_runner_test.go`、`runner_v2_test.go` が広く存在するため、まず足りない契約だけを追加する方が差分を小さく保てる。
