# Phase28 buildDependencies 分割実装仕様

## 目的

Phase28 は、`cmd/picoclaw/runtime_dependencies.go` の `buildDependencies()` 本体に残っている runtime wiring の集中を解消するための構造整理である。

目的は、`cmd/picoclaw` を composition root として維持しつつ、次の責務を説明可能な小さい関数へ分けることである。

- store setup
- provider setup
- conversation runtime
- agent setup
- Viewer handler wiring
- channel handler wiring
- orchestrator wiring
- heartbeat wiring

この Phase では、handler 本体、DTO、SSE event、IdleChat 契約、Viewer 表示契約、STT/TTS provider 挙動、LLM provider 挙動は変更しない。挙動変更なしで、仕様変更時に主に触る実装箇所を説明できる状態へ近づける。

## 対象範囲

対象は次の範囲に限定する。

- `cmd/picoclaw/runtime_dependencies.go` の `buildDependencies()`
- `buildDependencies()` から呼ばれる既存 runtime helper
- `Dependencies` struct へ詰める handler / service / store / runtime の組み立て
- `cmd/picoclaw/runtime_*.go` に置く composition root helper

## 対象外

次は Phase28 の対象外とする。

- HTTP route 登録変更
- handler 本体変更
- provider 実装変更
- runtime config の意味変更
- Viewer JS / CSS 変更
- IdleChat 契約変更
- STT / TTS provider 挙動変更
- LLM provider 挙動変更
- MessageOrchestrator / DistributedOrchestrator の usecase 本体変更
- L1SQLiteStore の persistence 契約変更
- `internal/application` / `internal/domain` / `internal/infrastructure` への責務移動

## 現在の責務分類

`buildDependencies()` は現在、次の責務を 1 つの関数内で順に組み立てている。

| 分類 | 現在の内容 | 変更時に触るべき主担当 |
| --- | --- | --- |
| tool registry | DuckDB ToolRegistry の初期化、CompositeRunner fallback | `runtime_tool_registry.go` 相当 |
| capability probe | LLM / tool / memory / platform の probe とログ | `runtime_capability.go` 相当 |
| LLM providers | Chat / Worker / Heavy / Wild provider、Coder setup | `llm_runtime_factory.go`, `runtime_coders.go` |
| routing | classifier と rule dictionary | `buildDependencies()` または `runtime_orchestrator.go` |
| MCP / ToolRunner | Chat/Worker ToolRunner、Subagent、PolicyRunner、LegacyRunner | `runtime_tool_runtime.go` 相当 |
| conversation runtime / L1 SQLite | RealConversationManager、L1SQLite、embedder、summarizer、profile extractor、web search cache、source sweeper、parquet export | `runtime_conversation.go` 相当 |
| glossary | Glossary DB、feed sync、Mio/IdleChat への recent provider | `runtime_glossary.go` 相当 |
| agents | Mio / Shiro / Heavy / Wild agent、persona editor、conversation injection | `runtime_agents.go` 相当 |
| session / memory store | JSON session repository、CentralMemory、FileStore、session directory | `runtime_sessions.go` 相当 |
| WorkerExecutionService | Worker execution service の生成とログ | `runtime_worker_execution.go` 相当 |
| Viewer handlers | EventHub、EventLog、MonitorStore、Evidence、Memory、SourceRegistry、send handler closure | `runtime_viewer_handlers.go` 相当 |
| IdleChat runtime | IdleChatOrchestrator、speaker provider、forecast provider、topic store、event emitter | `runtime_idlechat.go` 相当 |
| MessageOrchestrator / DistributedOrchestrator | v3/v4 mode 分岐、orchestrator 設定、channel handler、entry/chrome bridge | `runtime_orchestrator.go`, `runtime_channels.go` 相当 |
| heartbeat | HeartbeatService、notification sender、memory/event listener injection | `runtime_heartbeat.go` |

## 提案する分割単位

Phase28 では、`buildDependencies()` の中にある処理を次の関数へ移す。

### `buildRuntimeToolRegistry`

- 入力: `*config.Config`
- 出力: `capdomain.ToolRegistry`
- 副作用: ToolRegistry DB へ接続する
- 永続化: `cfg.Capability.ToolRegistryDB`
- ログ: 初期化成功、初期化失敗 warning
- エラー契約: 初期化失敗は fatal にせず warning のまま継続する
- 変更禁止: `ProbeLLMs` の有無に関係なく runtime ToolRegistry を初期化する挙動

### `buildCapabilityRuntime`

- 入力: `*config.Config`, `capdomain.ToolRegistry`
- 出力: `capdomain.NodeCapabilities`
- 副作用: capability probe を実行する
- 永続化: ToolRegistry がある場合は detector に渡す
- ログ: profile、LLM 数、tool 数、memory、platform、各 LLM の availability
- エラー契約: probe 失敗は warning のまま継続する
- 変更禁止: `cfg.Capability.ProbeLLMs` が false の場合は probe しない

### `buildLLMRuntimeProviders`

- 入力: `*config.Config`
- 出力: Chat / Worker / Heavy / Wild provider、Worker tool calling provider、Coder adapters
- 副作用: local LLM warmup が有効な場合は warmup goroutine を起動する
- 永続化: なし
- ログ: Coder setup、provider error、persona load
- エラー契約: Worker provider が `llm.ToolCallingProvider` でなければ fatal
- 変更禁止: Chat / Worker / Heavy / Wild の provider 選択と timeout / model 解決

### `buildToolRuntime`

- 入力: `*config.Config`, Worker tool provider, ToolRegistry
- 出力: Chat legacy runner、Worker legacy runner、subagent manager
- 副作用: ToolRunner、Subagent、PolicyRunner、CompositeRunner を組み立てる
- 永続化: security audit repository を使う場合がある
- ログ: Subagent enabled / disabled、Security policy enabled、ToolRunner tool count、Google Search 設定
- エラー契約: security audit repository や PolicyRunner の作成失敗は既存通り fatal
- 変更禁止: Chat と Worker の ToolRunner を分けること、persona write path を Chat のみに許可すること

### `buildConversationRuntime`

- 入力: `*config.Config`, primary providers, Chat/Worker ToolRunnerV2
- 出力: conversation engine、RealConversationManager、L1SQLiteStore
- 副作用: L1SQLite directory 作成、source registry sweeper 起動、parquet export job 起動、web search cache injection
- 永続化: Redis、DuckDB、VectorDB、L1SQLite、parquet export
- ログ: ConversationEngine enabled / disabled、L1SQLite、embedder、summarizer、profile extractor、web_search cache
- エラー契約: RealConversationManager / L1SQLite 初期化失敗は既存通り fatal
- 変更禁止: L1SQLite path、web search cache、daily digest summarizer、source registry sweeper の条件

### `buildGlossaryRuntime`

- 入力: `*config.Config`
- 出力: recent glossary context、recent glossary topics、glossary recent handler
- 副作用: glossary DB directory 作成、feed sync、refresh goroutine
- 永続化: `cfg.Glossary.DBPath`
- ログ: glossary directory create warning、sync warning、enabled ログ
- エラー契約: glossary 初期化失敗は warning のまま継続する
- 変更禁止: Mio と IdleChat に渡す recent provider の条件

### `buildAgentRuntime`

- 入力: `*config.Config`, providers, classifier, rule dictionary, runners, MCP client, conversation engine, glossary context, RealConversationManager, subagent manager
- 出力: Mio / Shiro / Heavy / Wild agent
- 副作用: persona editor を Mio に注入する
- 永続化: Mio persona file path
- ログ: Glossary context injection、KBManager injection、PersonaEditor injection、Shiro persona load
- エラー契約: persona file が読めない場合は既存通り無視して継続する
- 変更禁止: subagent disabled 時に typed nil を Shiro へ渡さない契約

### `buildSessionRuntime`

- 入力: `*config.Config`
- 出力: session repository、central memory、heartbeat memory store
- 副作用: session directory 作成
- 永続化: `cfg.Session.StorageDir`, `cfg.WorkspaceDir`
- ログ: MemoryStore initialized
- エラー契約: session directory 作成失敗は fatal
- 変更禁止: ConversationEngine と HeartbeatService の memory store 責務を混同しないこと

### `buildViewerRuntimeHandlers`

- 入力: `*config.Config`, `*Dependencies`, L1SQLiteStore, RealConversationManager, report path
- 出力: EventHub、EventLogStore、EventLogGCService、MonitorStore、Evidence handlers、Memory handlers、SourceRegistry handler
- 副作用: event log store / GC 起動、report store 作成
- 永続化: viewer event log、event GC log、execution report JSONL
- ログ: Viewer event log enabled、GC enabled、evidence API enabled / disabled
- エラー契約: event log / report store 初期化失敗は warning のまま、該当 API を縮退する
- 変更禁止: Viewer 表示、SSE event、event log、evidence、memory API の契約

### `buildViewerBridgeHandlers`

- 入力: `*config.Config`, `*Dependencies`, message processor、report path、TTS entry runtime
- 出力: viewer send handler、entry handler、chrome bridge handlers
- 副作用: attachment store 作成、entry stage event の emit
- 永続化: attachment store
- ログ: viewer send ProcessMessage start / completed / error、entry stage logs
- エラー契約: ProcessMessage error は viewer error event として通知する
- 変更禁止: attachment、entry、chrome bridge の request / response 契約

### `buildIdleChatRuntime`

- 入力: `*config.Config`, `*Dependencies`, Chat/Worker/Heavy/Wild provider、central memory、coder2 adapter、recent glossary topics、TTS bridge
- 出力: IdleChatOrchestrator
- 副作用: IdleChat start、topic store 作成、event emitter 設定、TTS async enqueue
- 永続化: `idlechat_topics.jsonl`, `forecast_topic_stock.json`
- ログ: forecast provider、glossary topics、topic store、enabled
- エラー契約: topic store 初期化失敗は warning のまま継続する
- 変更禁止: `idlechat.tts` を Viewer に送らない、`idlechat.viewer` を TTS に送らない、raw content を Viewer event に保持する

### `buildOrchestratorRuntime`

- 入力: `*config.Config`, `*Dependencies`, session repository、agents、coder adapters、worker execution、node capabilities、central memory、TTS bridge、VTuber bridge、factory closures
- 出力: `Dependencies` に orchestrator と主要 handlers を設定する
- 副作用: v3/v4 mode 分岐、Distributed transports 起動、Local agent goroutine 起動
- 永続化: execution report store を orchestrator に渡す
- ログ: v3/v4 mode、dynamic coder selection、IdleChat integration
- エラー契約: Distributed transport factory error は既存通り fatal
- 変更禁止: v3/v4 分岐条件、channel handler 条件、event listener / report store / TTS / VTuber injection

### `buildChannelRuntimeHandlers`

- 入力: `*config.Config`, message processor
- 出力: line / telegram / discord / slack handlers
- 副作用: 各 channel adapter を組み立てる
- 永続化: なし
- ログ: 既存と同等
- エラー契約: token が空の adapter は登録しない
- 変更禁止: channel secret、public key、webhook secret、signing secret の扱い

### `buildHeartbeatRuntime`

- 入力: `*config.Config`, Mio agent、memory store、event listener
- 出力: HeartbeatService
- 副作用: HeartbeatService start
- 永続化: File memory store
- ログ: HeartbeatService enabled
- エラー契約: notification sender が nil でも service 側の既存挙動を維持する
- 変更禁止: heartbeat channel/chat_id の扱いと outbound channel registry 条件

## 実装手順

1. baseline を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw`
2. `buildRuntimeToolRegistry` と `buildCapabilityRuntime` を先に作る。
3. `buildLLMRuntimeProviders` を作り、Worker tool provider の fatal 条件を維持する。
4. `buildToolRuntime` を作り、Chat / Worker ToolRunner、Subagent、PolicyRunner、CompositeRunner の順序を維持する。
5. `buildConversationRuntime` を作り、ConversationEngine、L1SQLite、web search cache、source registry sweeper、parquet export の条件を維持する。
6. `buildGlossaryRuntime` を作り、Mio / IdleChat へ渡す recent provider を維持する。
7. `buildAgentRuntime` と `buildSessionRuntime` を作る。
8. `buildViewerRuntimeHandlers` を作り、`Dependencies` へ詰める viewer handler 群を維持する。
9. `buildViewerBridgeHandlers` を作り、viewer send / entry / chrome bridge closure を移す。
10. `buildIdleChatRuntime` を作り、IdleChat event emitter の Viewer/TTS 分離契約を維持する。
11. `buildOrchestratorRuntime` を作り、v3/v4 分岐と channel handler 条件を維持する。
12. `buildHeartbeatRuntime` を作る。
13. `buildDependencies()` は、上記 helper を順に呼ぶ薄い composition root にする。
14. `gofmt` / `goimports` を実行する。
15. after test を実行する。

実装中は、初期化順、nil check、handler 契約、store path、runtime config の意味、LLM Ops start gate の生成順を変えない。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./...
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
git diff --check
git diff --stat
```

live health が使える場合:

```bash
curl -fsS http://127.0.0.1:18790/health
```

live/browser E2E が使える場合:

```bash
PICOCLAW_LIVE_E2E=1 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25LiveRuntimeHealthAndViewerConfig -v
PICOCLAW_BROWSER_E2E=1 PICOCLAW_LIVE_BASE_URL=http://127.0.0.1:18790 GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e -run TestE2E_Phase25BrowserViewerSessionContract -v
```

Viewer / IdleChat / STT / TTS の handler 本体は Phase28 で変更しない。もし触る必要が出た場合は Phase28 を停止し、別 Phase として扱う。

## リスク

- 初期化順を変える。
- nil dependency の扱いを変える。
- Viewer handler を `Dependencies` に詰め忘れる。
- Evidence API 初期化失敗時の縮退挙動を変える。
- IdleChat 起動条件や event emitter の Viewer/TTS 分離を変える。
- LLM Ops start gate の生成順や注入先を変える。
- L1SQLite / Source Registry / Memory の保存先を変える。
- ToolRunner / PolicyEngine / CompositeRunner の共有境界を崩す。
- Chat / Worker / Coder の責務境界を崩す。
- fallback を正常系として扱う。
- repo example と live runtime config を混同する。

## 完了条件

- `buildDependencies()` の責務分類がこの文書に記録されている。
- 分割関数案と各関数の契約が記録されている。
- 実装手順と検証手順が記録されている。
- この文書作成時点ではコード変更を行っていない。
- 次にユーザーが Phase28 実装可否を判断できる。

## 実装時の停止条件

次の場合は実装を止め、状況と選択肢を報告する。

- 初期化順を維持したまま分割できない。
- handler 本体、DTO、SSE event、IdleChat 契約、Viewer 表示契約、STT/TTS provider 挙動、LLM provider 挙動の変更が必要になる。
- live runtime config の意味変更が必要になる。
- Phase28 の範囲を超えて Application / Domain / Infrastructure の設計変更が必要になる。
- テスト失敗の原因が Phase28 の構造整理内で安全に切り分けられない。
