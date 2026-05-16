# Phase9 route dispatcher 境界整理実装仕様

## 1. Phase9 の目的

Phase9 は、Phase8 で method のまま残した `MessageOrchestrator` の route-specific execution を、`messageRouteDispatcher` という private collaborator 境界へ整理する段階である。

目的は次の通り。

- `MessageOrchestrator.ProcessMessage` の top-level orchestration を維持する。
- route dispatch の入力、出力、副作用、ログ、エラー契約を明確にする。
- Phase6 で固定した Chat / Worker / Coder route chain 契約を変更しない。
- Phase8 で collaborator 化した `autonomousExecutionCoordinator` から route direct execution を呼べる境界を維持する。
- TTS lifecycle、event emitter、task context builder は Phase9 では collaborator 化しない。

Phase9 は挙動変更ではなく構造整理である。

## 2. 対象範囲

対象範囲は次の通り。

- `internal/application/orchestrator/message_orchestrator_routes.go`
  - `executeTask`
  - `executeChatRoute`
  - `executeRouteDirect`
  - `executeOPSRoute`
  - `executeCodeRoute`
  - `executeWildRoute`
  - `executePlanRoute`
  - `executeAnalyzeRoute`
  - `executeResearchRoute`
  - `executeCodeViaShiro`
- `internal/application/orchestrator/message_orchestrator.go`
  - `MessageOrchestrator` field
  - constructor
  - `ProcessMessage` の route execution 呼び出し
- `internal/application/orchestrator/message_orchestrator_autonomous.go`
  - autonomous coordinator に渡す route direct executor
- route chain / TTS / Phase8 collaborator tests

## 3. 対象外

Phase9 では次を対象外にする。

- TTS lifecycle collaborator 化。
- event emitter collaborator 化。
- task context builder collaborator 化。
- CodeExecutor 再分割。
- WorkerExecutionService 内部。
- ToolRunner / PolicyEngine。
- handler / DTO / SSE event。
- Viewer JS / CSS。
- IdleChat。
- STT / TTS provider。
- LLM provider。
- runtime config。
- distributed orchestrator。
- 未追跡の `tests/`。

fallback、degraded、error、handled success を混同しない。Viewer 表示、音声、口パク、ログも混同しない。

## 4. 現在の route dispatcher 構造

### `executeTask`

`executeTask` は route が `CHAT` 以外の場合に `executeAutonomousTask` へ委譲し、`CHAT` の場合だけ `executeChatRoute` を直接呼ぶ。

契約:

- `CHAT` は autonomous executor を通さない。
- `CHAT` 以外は autonomous coordinator を通す。
- autonomous coordinator は必要に応じて route direct executor を呼ぶ。

### `executeChatRoute`

`executeChatRoute` は Mio chat route を実行する。

責務:

- `agent.start` event を user 向けに emit する。
- stream hook を接続する。
- `MioAgent.Chat` を呼ぶ。
- 成功時だけ `agent.response` event を emit する。
- 成功時だけ TTS / VTuber stream を finalize する。

### `executeRouteDirect`

`executeRouteDirect` は autonomous coordinator から呼ばれる direct dispatcher である。

責務:

- OPS / CODE / WILD / PLAN / ANALYZE / RESEARCH を route 別 method へ分岐する。
- unsupported route は `unsupported autonomous route` error を返す。

### route-specific execution

- OPS: `ShiroAgent.Execute` を呼び、成功時に `shiro -> mio` response event と TTS push を行う。
- CODE: `executeCodeViaShiro` から `CodeExecutor.ExecuteCode` へ委譲し、成功時に TTS push を行う。
- WILD: `WildAgent.Generate` を呼ぶ。nil wild は error にする。
- PLAN: `MioAgent.Chat` を PLAN route として呼ぶ。
- ANALYZE: heavy があれば `HeavyAgent.Generate`、なければ Mio chat を使う。
- RESEARCH: `MioAgent.Chat` を RESEARCH route として呼ぶ。

### `executeCodeViaShiro`

`executeCodeViaShiro` は CODE 系 route の `CodeExecutionRequest` を作り、`CodeExecutor.ExecuteCode` へ委譲する。

Phase6 で固定した Shiro relay / CodeExecutor / WorkerExecutionService 契約に直結するため、Phase9 では挙動を変えない。

### TTS lifecycle と event emitter への依存

route dispatcher は次に依存している。

- `emit`
- `withStreamHooks`
- `pushTTS`
- `MioAgent`
- `ShiroAgent`
- `WildAgent`
- `HeavyAgent`
- `CodeExecutor`

Phase9 では、これらを `messageRouteDispatcher` の dependency として渡す。TTS lifecycle と event emitter 自体は method のまま残す。

## 5. 提案する collaborator

### `messageRouteDispatcher`

`messageRouteDispatcher` は private struct とする。初期段階では interface 化しない。

配置:

- `message_orchestrator_routes.go` に定義する。
- `MessageOrchestrator` に field として持たせる。
- `NewMessageOrchestrator` で組み立てる。

dependency:

- `MioAgent`
- `ShiroAgent`
- `WildAgent`
- `HeavyAgent`
- `CodeExecutor`
- event emit function
- stream hook function
- TTS push function

setter 反映:

- `SetWildAgent` 後は dispatcher が最新 wild を使う。
- `SetHeavyAgent` 後は dispatcher が最新 heavy を使う。
- `SetEventListener` は event emit function が `MessageOrchestrator.emit` を経由するため、最新 listener を使う。
- `SetTTSBridge` / `SetVTuberBridge` は `withStreamHooks` / `pushTTS` が `MessageOrchestrator` method を経由するため、最新 bridge を使う。

autonomous coordinator との接続:

- `autonomousExecutionCoordinator` へ渡す route direct executor は `messageRouteDispatcher.ExecuteDirect` にする。
- これにより autonomous coordinator は route-specific execution の実装詳細を知らない。

## 6. `messageRouteDispatcher` の契約

入力:

- `context.Context`
- `task.Task`
- `routing.Route`
- session ID
- channel
- chat ID
- TTS session ID

出力:

- response text
- error

副作用:

- agent call。
- route event emission。
- stream hook 接続。
- TTS / VTuber push または finalize。
- CodeExecutor 呼び出し。

永続化:

- dispatcher 自体は直接永続化しない。
- CODE route では CodeExecutor / WorkerExecutionService 側で execution record や patch execution report が発生し得る。

ログ:

- dispatcher 自体は新しいログを増やさない。
- event emission、TTS degraded、CodeExecutor / WorkerExecutionService の既存ログを維持する。

エラー契約:

- unsupported autonomous route は error。
- nil wild は `no wild agent available` error。
- CodeExecutor / WorkerExecutionService error は success response にしない。
- Generate / Execute error は fallback success にしない。
- TTS degraded は route execution success と混同しない。

差し替え可能性:

- route-specific execution を `MessageOrchestrator` から分離することで、将来 adapter / application 境界に移しやすくする。
- Phase9 では private struct に留め、外部公開 interface は作らない。

変更してはいけない既存挙動:

- `CHAT` は autonomous executor を通さない。
- CODE 系 route は `CodeExecutor.ExecuteCode` を通る。
- Phase6 の Shiro relay event order を維持する。
- TTS push / stream finalize のタイミングを変えない。
- ANALYZE の heavy nil fallback を変えない。
- WILD の nil error を変えない。
- route-specific event type / from / to / route / jobID を変えない。

## 7. 実装手順

1. baseline test を実行する。
2. `messageRouteDispatcher` を `message_orchestrator_routes.go` に追加する。
3. dispatcher dependency として agents、CodeExecutor、emit、stream hook、TTS push を持たせる。
4. `MessageOrchestrator` に `routeDispatcher *messageRouteDispatcher` field を追加する。
5. `NewMessageOrchestrator` で dispatcher を組み立てる。
6. `SetWildAgent` / `SetHeavyAgent` で dispatcher の dependency も更新する。
7. `ProcessMessage` の route execution 呼び出しを dispatcher 経由にする。
8. `autonomousExecutionCoordinator` に渡す route direct executor を dispatcher 経由にする。
9. route-specific method の処理を dispatcher method へ移す。
10. route path、event order、TTS push/finalize、CodeExecutor handoff を変更しない。
11. gofmt を実行する。
12. focused test と全体 test を実行する。
13. `docs/refactor/Phase9_完了判定.md` を作成する。

## 8. テスト方針

baseline / after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator ./internal/application/service ./internal/domain/routing ./internal/domain/proposal ./internal/domain/patch ./cmd/picoclaw
```

route chain:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_CodeRoute_|TestMessageOrchestrator_RouteChainContract_|TestCodeExecutor_'
```

TTS:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestMessageOrchestrator_TTSBridge_|TestTTSStreamForwarder|TestMessageOrchestrator_ProcessMessage_OPSRoute_StartsMaleTTSVoice'
```

Phase8:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/orchestrator -run 'TestPhase8'
```

差分確認:

```bash
git diff --check
git diff --stat
```

## 9. リスク

- route chain event order を壊す。
- CODE route が CodeExecutor を経由しなくなる。
- Worker error / Generate error を success に変換する。
- TTS push / stream finalize のタイミングを変える。
- WILD / HEAVY nil fallback の挙動を変える。
- autonomous coordinator の route direct executor と dispatcher が循環する。
- route dispatcher が巨大 manager になる。
- event emitter / TTS lifecycle まで同時に切り出して Phase9 の範囲を超える。

## 10. 完了条件

Phase9 の完了条件は次の通り。

- `docs/refactor/Phase9_route_dispatcher境界整理実装仕様.md` が作成されている。
- route dispatcher の現在構造が棚卸しされている。
- `messageRouteDispatcher` の入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- Phase6 / Phase8 契約を維持する方針が明記されている。
- コード変更は行っていない。
- ユーザーが次に Phase9 を実装してよいか判断できる。
