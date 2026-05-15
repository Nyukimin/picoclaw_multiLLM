# Phase1-1 route登録分割仕様

## Phase 1-1 の目的

Phase 1-1 では、`cmd/picoclaw/main.go` の `cmdRun()` 内にある HTTP route 登録だけを機能単位の registration 関数へ分割する。

目的は次の通り。

- `cmdRun()` の責務を薄くする。
- HTTP route 登録を機能単位に分け、composition root として読みやすくする。
- handler の中身や契約を変えず、route 登録の配置だけを整理する。
- モジュール化と疎結合を守り、将来 Adapter 側へ移しやすい境界を作る。

この Phase では route 登録以外の挙動変更をしない。

## 参照資料

- `AGENTS.md`
- `CLAUDE.md`
- `docs/01_正本仕様/実装仕様.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/段階移行計画.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase0_cmd構成整理計画.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/*_test.go`

## 対象範囲

対象は `cmd/picoclaw/main.go` の `cmdRun()` にある `mux.Handle` / `mux.HandleFunc` 登録部分である。

対象に含めるもの:

- `mux.Handle` / `mux.HandleFunc` の機能単位グループ化。
- route 登録に必要な `debugSystemOpts`、`sttRuntime`、`llmOpsOpts`、`dependencies` の受け渡し。
- LLM Ops route 登録時の `dependencies.idleChatStartGate` 設定。
- 既存の `registerSTTRuntimeRoutes(mux, sttRuntime)` 呼び出し位置の保持。
- `healthHandler := dependencies.buildHealthHandler(cfg)` と `/health` / `/ready` 登録の関数化。

## 対象外

Phase 1-1 では次を変更しない。

- handler 実装。
- DTO。
- SSE event。
- Viewer JS / CSS。
- IdleChat raw / view / audio 契約。
- `MessageOrchestrator`。
- `WorkerExecutionService`。
- LLM provider / STT provider / TTS provider。
- runtime config の意味。
- `/health` の意味。
- `/ready` を LLM server 一般仕様として扱うこと。
- live `~/.picoclaw/config.yaml`。

## 現在の route 登録の分類

### channel webhook routes

- `/webhook`
- `/webhook/telegram`
- `/webhook/discord`
- `/webhook/slack`

### viewer static / runtime routes

- `/viewer`
- `/viewer/assets/`
- `/viewer/runtime-config`
- `/viewer/logo.png`
- `/viewer/mio-lipsync-closed.svg`
- `/viewer/mio-lipsync-open.svg`
- `/viewer/shiro-lipsync-closed.svg`
- `/viewer/shiro-lipsync-open.svg`
- `/viewer/tts/audio`
- `/viewer/events`
- `/viewer/debug/system`
- `/viewer/assets-git/status`

### LLM Ops routes

- `/viewer/llm-ops/health`
- `/viewer/llm-ops/status`
- `/viewer/llm-ops/start`
- `/viewer/llm-ops/stop`
- `/viewer/llm-ops/restart`

### STT / audio routes

- `/viewer/stt/log`
- `/viewer/stt/wav`
- `/viewer/stt/autotest`
- `/stt/health`
- `/stt/file`
- `/stt/chat-input`
- `/stt`
- `/stt-ws`
- `/ws`
- `/audio-router/events`

`/stt/health`、`/stt/file`、`/stt/chat-input`、`/stt`、`/stt-ws`、`/ws` は既存の `registerSTTRuntimeRoutes(mux, sttRuntime)` と `registerSTTRoutes` の契約を維持する。

### viewer monitor / evidence / memory / source registry routes

- `/viewer/status`
- `/viewer/agents`
- `/viewer/agent/detail`
- `/viewer/jobs`
- `/viewer/logs`
- `/viewer/audit/summary`
- `/viewer/job/detail`
- `/viewer/send`
- `/viewer/evidence/recent`
- `/viewer/evidence/detail`
- `/viewer/evidence/summary`
- `/viewer/glossary/recent`
- `/viewer/memory/snapshot`
- `/viewer/memory/layers`
- `/viewer/memory/events`
- `/viewer/memory/state`
- `/viewer/memory/promote`
- `/viewer/recall/traces`
- `/viewer/source-registry`

### entry / chrome routes

- `/entry`
- `/chrome/bridge`
- `/chrome/bridge/status`
- `/chrome/bridge/events`

### IdleChat routes

- `/viewer/idlechat/start`
- `/viewer/idlechat/stop`
- `/viewer/idlechat/status`
- `/viewer/idlechat/logs`
- `/viewer/idlechat/forecast`
- `/viewer/idlechat/story`
- `/viewer/idlechat/story-simple`

### health routes

- `/health`
- `/ready`

`/ready` は現在 `cmdRun()` で登録されている route として維持する。ただし LLM server 側の一般仕様として前提にしない。運用確認の主対象は `/health` とする。

## 提案する関数分割

Phase 1-1 では、まず `cmd/picoclaw/main.go` 内に private 関数として追加する。別ファイル化は次の段階で判断する。

```go
func registerChannelRoutes(mux *http.ServeMux, dependencies *Dependencies)
func registerViewerBaseRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies, debugSystemOpts viewer.DebugSystemOptions)
func registerLLMOpsRoutes(mux *http.ServeMux, cfg *config.Config, dependencies *Dependencies, debugSystemOpts *viewer.DebugSystemOptions)
func registerSTTAndAudioRoutes(mux *http.ServeMux, sttRuntime sttRuntime, dependencies *Dependencies)
func registerViewerDynamicRoutes(mux *http.ServeMux, dependencies *Dependencies)
func registerEntryAndChromeRoutes(mux *http.ServeMux, dependencies *Dependencies)
func registerIdleChatRoutes(mux *http.ServeMux, dependencies *Dependencies)
func registerHealthRoutes(mux *http.ServeMux, dependencies *Dependencies, cfg *config.Config)
```

`cmdRun()` 側は、config 読み込み、dependency build、signal handling、server 作成、listen を残し、route 登録は上記関数呼び出しへ置き換える。

## 各関数の契約

### `registerChannelRoutes`

入力:

- `*http.ServeMux`
- `*Dependencies`

出力:

- なし。

副作用:

- `/webhook` を常に登録する。
- telegram / discord / slack handler が nil でない場合だけ対応 route を登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。
- nil handler の条件付き登録を既存通り維持する。

変更してはいけない既存挙動:

- `/webhook` の登録。
- telegram / discord / slack の nil check。

### `registerViewerBaseRoutes`

入力:

- `*http.ServeMux`
- `*config.Config`
- `*Dependencies`
- `viewer.DebugSystemOptions`

出力:

- なし。

副作用:

- Viewer page、assets、runtime config、logo、lip sync SVG、TTS audio、SSE、debug system、assets git status を登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- `viewer.HandleRuntimeConfig(debugSystemOpts)` は同じ値を受け取る。
- `/viewer/events` は `dependencies.eventHub.HandleSSE` のままにする。
- `/viewer/tts/audio` は `handleLocalTTSAudio(cfg.TTS.OutputDir)` のままにする。

### `registerLLMOpsRoutes`

入力:

- `*http.ServeMux`
- `*config.Config`
- `*Dependencies`
- `*viewer.DebugSystemOptions`

出力:

- なし。

副作用:

- `debugSystemOpts.LLMOpsEnabled` が true の場合だけ `/viewer/llm-ops/*` を登録する。
- `dependencies.idleChatStartGate` に `viewer.NewLLMOpsIdleChatGate(llmOpsOpts)` を設定する。
- 既存の LLM Ops proxy log を出す。

ログ:

- 既存の `Viewer: MLX llm-ops proxy -> ...` を維持する。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- `LLM_OPS_TOKEN` が空なら route を登録しない。
- `cfg.LLMOps.Enabled` と `cfg.LLMOps.BaseURL` の判定条件を変えない。
- IdleChat start gate の設定順を変えない。

注意:

- `debugSystemOpts` は `registerViewerBaseRoutes` に渡す前に LLM Ops 設定が反映されている必要がある。実装では `debugSystemOpts` を作成し、LLM Ops の enabled / base URL / LocalLLM を埋めてから Viewer runtime config route を登録する。

### `registerSTTAndAudioRoutes`

入力:

- `*http.ServeMux`
- `sttRuntime`
- `*Dependencies`

出力:

- なし。

副作用:

- Viewer STT log / wav / autotest route を登録する。
- 既存の `registerSTTRuntimeRoutes(mux, sttRuntime)` を呼び出す。
- `/audio-router/events` を登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- `/stt`、`/stt-ws`、`/ws` の互換 endpoint を落とさない。
- `registerSTTRuntimeRoutes` の内部契約を変えない。
- `/audio-router/events` は `viewer.HandleAudioRouterSSE(dependencies.eventHub)` のままにする。

### `registerViewerDynamicRoutes`

入力:

- `*http.ServeMux`
- `*Dependencies`

出力:

- なし。

副作用:

- Viewer monitor、evidence、glossary、memory、source registry、viewer send route を nil check 付きで登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- 各 handler の nil check 条件を変えない。
- route path を変えない。
- Viewer 表示契約、SSE event、memory/source registry の handler 契約を変えない。

### `registerEntryAndChromeRoutes`

入力:

- `*http.ServeMux`
- `*Dependencies`

出力:

- なし。

副作用:

- `/entry` と `/chrome/bridge*` route を nil check 付きで登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- entry / chrome bridge handler の nil check を維持する。
- handler 内の event emission や stage handling は変更しない。

### `registerIdleChatRoutes`

入力:

- `*http.ServeMux`
- `*Dependencies`

出力:

- なし。

副作用:

- `dependencies.idleChatOrch` が nil でない場合だけ `/viewer/idlechat/*` route を登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- IdleChat route の登録条件を変えない。
- `handleIdleChatStart`、`handleIdleChatStop`、`handleIdleChatStatus`、`handleIdleChatLogs`、`handleIdleChatForecast`、`handleIdleChatStory`、`handleIdleChatStorySimple` の中身を変えない。
- raw / view / audio 契約を変えない。

### `registerHealthRoutes`

入力:

- `*http.ServeMux`
- `*Dependencies`
- `*config.Config`

出力:

- なし。

副作用:

- `dependencies.buildHealthHandler(cfg)` を呼び、`/health` と `/ready` を登録する。

ログ:

- 追加しない。

エラー契約:

- error は返さない。

変更してはいけない既存挙動:

- `/health` の契約を変えない。
- `/ready` の登録は維持する。
- `/ready` を外部 LLM server の一般契約として扱わない。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmdRun()` の route 登録を、既存順序と条件を保ったまま関数へ移す。
3. `buildSTTRuntime`、`debugSystemOpts`、LLM Ops start gate の生成順を変えない。
4. `debugSystemOpts` の LocalLLM 設定、LLM Ops configured / enabled / base URL 設定を既存通り行う。
5. `registerSTTRuntimeRoutes` は既存関数をそのまま使う。
6. handler の中身、DTO、SSE event、IdleChat 契約は変更しない。
7. `gofmt` を実行する。
8. テストを再実行する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

route 条件に触った場合のみ live health:

```bash
curl -fsS http://127.0.0.1:18790/health
```

Viewer / IdleChat / STT / TTS の handler 中身を変えた場合は追加で実ブラウザ確認が必要。ただし Phase 1-1 では handler 中身を変えない。

## リスク

- LLM Ops start gate の設定順を変える。
- `debugSystemOpts` の値渡し / pointer 渡しを間違える。
- nil handler の条件付き route 登録を崩す。
- STT route の互換 endpoint `/stt-ws` / `/ws` を落とす。
- `/ready` を LLM server 一般仕様と誤解する。
- Viewer route を登録漏れする。
- `/viewer/runtime-config` 登録時点の `debugSystemOpts` が既存とずれる。
- route 登録順の変更で、同じ path / prefix の解決順に影響する。

## 完了条件

- `docs/refactor/Phase1-1_route登録分割仕様.md` が作成されている。
- route 分類が網羅されている。
- 関数分割案がある。
- 各関数の契約が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- ユーザーが次に「実装してよいか」を判断できる。

## 次に確認すること

実装に進む場合は、次の範囲でよいか確認する。

1. `cmdRun()` の route 登録だけを private registration 関数へ分ける。
2. handler の中身、DTO、SSE event、IdleChat 契約は変えない。
3. baseline / after で `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` を実行する。
4. route 条件や runtime config の意味には触れない。
