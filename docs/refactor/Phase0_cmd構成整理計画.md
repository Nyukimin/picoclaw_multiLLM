# Phase0 cmd構成整理計画

## Phase 0 の目的

この文書は、`cmd/picoclaw/main.go` の composition root 整理に入る前に、対象範囲、移動候補、触らない対象、検証方法を固定するための計画である。

Phase 0 ではコード変更を行わない。目的は、最初のリファクタリング単位を小さく決め、ユーザーが実装開始可否を判断できる状態にすることである。

## 参照した資料

- `AGENTS.md`
- `CLAUDE.md`
- `docs/01_正本仕様/実装仕様.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/段階移行計画.md`
- `docs/refactor/検証方針.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/stt_runtime_factory.go`
- `cmd/picoclaw/tts_runtime_factory.go`
- `cmd/picoclaw/tts_client_bridge.go`
- `cmd/picoclaw/vtuber_bridge.go`

## 現在の `cmd/picoclaw/main.go` の役割整理

`cmd/picoclaw/main.go` は約 3,900 行あり、起動 entrypoint、CLI、HTTP route 登録、依存構築、各 runtime の wiring が集中している。

### 起動処理

- `main()` が `run`、`health`、`status`、`doctor`、`channels`、`gateway`、`ollama`、`logs`、`evidence`、`source-registry`、`knowledge` などのサブコマンドへ分岐する。
- `cmdRun()` が config 読み込み、依存構築、signal handling、HTTP server 起動を担当する。
- `cmdRun()` は server 起動だけでなく、Viewer route、STT route、LLM Ops route、IdleChat route、health route の登録も直接持っている。

### CLI 引数処理

次の CLI 系処理が `main.go` にまとまっている。

- health / status / doctor
- channels
- gateway / ollama
- logs
- evidence
- source-registry
- knowledge

CLI 本体と補助関数は `cmd/` に置いてよいが、Source Registry や evidence の状態処理本体は Application / Infrastructure へ寄せる候補になる。

### HTTP server 起動

`cmdRun()` は `http.NewServeMux()` を作り、次を直接登録している。

- `/webhook` と channel webhook
- `/viewer`、Viewer assets、Viewer runtime config
- `/viewer/llm-ops/*`
- `/viewer/stt/*`
- `/audio-router/events`
- Viewer monitor / evidence / memory / source registry / glossary
- `/entry`
- `/chrome/bridge*`
- `/viewer/idlechat/*`
- `/health`
- `/ready`

composition root として route 登録は残してよい。ただし、route 群ごとの登録補助関数や handler factory は切り出し候補になる。

### runtime config 読み込み

- `getConfigPath()` と `config.LoadConfig()` は `cmdRun()` と CLI 各コマンドから使われる。
- `~/.picoclaw/config.yaml` が live runtime config であり、repo example だけで判断してはいけない。
- Viewer runtime config は `viewer.HandleRuntimeConfig(debugSystemOpts)` に渡されるため、表示値と live config の混同に注意する。

### runtime factory

すでに一部は `main.go` から別ファイルに分離済みである。

- STT: `cmd/picoclaw/stt_runtime_factory.go`
- TTS provider: `cmd/picoclaw/tts_runtime_factory.go`
- TTS bridge: `cmd/picoclaw/tts_client_bridge.go`
- VTuber bridge: `cmd/picoclaw/vtuber_bridge.go`
- autonomous entry: `cmd/picoclaw/autonomous_entry.go`

一方、次はまだ `main.go` 側にある。

- primary LLM provider 構築
- conversation text provider / embedder 構築
- local LLM alias 解決
- health service 構築
- channel registry 構築
- coder setup
- subagent provider 解決

### handler 登録

Viewer、IdleChat、Chrome bridge、Entry、Evidence、Memory、Source Registry、Glossary の handler 登録が `cmdRun()` と `buildDependencies()` にまたがっている。

handler の生成と route 登録は分ける余地がある。ただし Viewer 表示契約、IdleChat raw/view/audio 契約、SSE event 契約は変更しない。

### IdleChat / STT / TTS / VTuber / Viewer / LLM Ops wiring

- IdleChat は `buildDependencies()` 内で provider、topic store、event emitter、TTS bridge、Viewer event を結線する。
- STT は `cmdRun()` で `buildSTTRuntime()` し、debug system options と route 登録を行う。
- TTS は `buildDependencies()` で `buildTTSEntryRuntime()`、`buildTTSClientBridge()`、VTuber lip sync と接続する。
- Viewer LLM Ops は `cmdRun()` で token と config を確認し、IdleChat start gate と `/viewer/llm-ops/*` route を登録する。

### 実処理が混ざっている可能性のある箇所

次は composition root というより、Application / Infrastructure / Adapter 側へ寄せる候補である。

- Source Registry CLI の parse / save / sweep 表示変換。
- Evidence CLI の filter / summarize。
- IdleChat HTTP handler の request validation と response DTO 組み立て。
- Viewer send / entry / chrome bridge の closure 内にある event emission と error handling。
- primary LLM provider と local alias 解決。
- coder setup と persona / light memory 初期化。
- channel registry 構築。

## composition root に残すもの

`cmd/picoclaw` に残す責務は次に限定する。

- process 起動。
- CLI entrypoint。
- config 読み込みの入口。
- dependency wiring。
- HTTP server 起動。
- route 登録の最終接続。
- adapter / application / infrastructure の接続。
- signal handling と shutdown 呼び出し。

残す場合でも、巨大な `main.go` にすべてを直書きせず、機能単位の小さな registration / factory 関数へ分ける。

## 移動候補

### Application に寄せる候補

- IdleChat start / stop / forecast / story の usecase coordination。
- Entry request の orchestration と event stage emission。
- Viewer send の ProcessMessage 呼び出しと attachment handling の組み立て。
- Source Registry sweep の実行調整。
- Evidence summary / filter の usecase 化。

### Infrastructure に寄せる候補

- primary LLM provider 構築。
- local LLM alias、model、timeout、base URL 解決。
- conversation text provider / embedder 構築。
- coder provider setup。
- channel outbound registry の技術実装寄り構築。
- health check provider 構築の技術詳細。

### Adapter に寄せる候補

- Viewer route group 登録。
- Viewer runtime config handler factory。
- IdleChat HTTP handler 群。
- Chrome bridge handler group。
- Entry handler adapter。
- Source Registry / Memory / Evidence Viewer handler group 登録。

### まだ動かさない候補

次は最初の移行単位では動かさない。

- `MessageOrchestrator` の route dispatch 本体。
- `WorkerExecutionService` の実行安全境界。
- `ToolRunner` / `PolicyEngine` の policy 判定。
- `L1SQLiteStore` の状態遷移。
- IdleChat の生成、sanitize、fallback / invalid response 判定。
- Viewer JS / CSS。
- STT / TTS provider の挙動。
- LLM provider の request / response 挙動。

## 触らない対象

Phase 0 直後の最初の実装では、以下を変更しない。

- `WorkerExecutionService` の安全境界。
- `MessageOrchestrator` の route dispatch 本体。
- Viewer 表示契約。
- IdleChat の raw / view / audio 契約。
- runtime config の意味変更。
- LLM provider の挙動変更。
- STT / TTS provider の挙動変更。
- fallback の扱い。
- `/health` の契約。
- live `~/.picoclaw/config.yaml`。

## モジュール化・疎結合の観点

単にファイルを分けるだけではモジュール化ではない。切り出す単位ごとに、次を明記する。

- 入力。
- 出力。
- 副作用。
- 永続化の有無。
- ログ。
- エラー契約。
- 差し替え時に守る interface / contract / event / DTO / adapter。

共通化は、意味のある責務境界がある場合だけ行う。「便利だから共有する」「似ているからまとめる」だけの共通化は禁止する。

`service` / `manager` / `helper` / `util` という巨大な依存集約先は作らない。composition root を薄くするために別の巨大 object を作ることも禁止する。

## 最初の小さい移行単位案

### 第一候補: HTTP route 登録のグループ化

最初の実装候補は、`cmdRun()` 内の route 登録を機能単位の小さな関数へ分けることである。

候補:

- `registerChannelRoutes(mux, dependencies)`
- `registerViewerStaticRoutes(mux, cfg, dependencies, debugSystemOpts)`
- `registerViewerMonitorRoutes(mux, dependencies)`
- `registerEntryAndChromeRoutes(mux, dependencies)`
- `registerIdleChatRoutes(mux, dependencies)`

この段階では、handler の中身、DTO、SSE event、IdleChat 契約は変えない。`mux.HandleFunc` の配置だけを整理する。

入力:

- `*http.ServeMux`
- `*config.Config`
- `*Dependencies`
- `viewer.DebugSystemOptions`
- 必要な runtime options

出力:

- 戻り値なし。`mux` へ route を登録する。

副作用:

- HTTP route 登録。
- LLM Ops が有効な場合のみ IdleChat start gate を設定する可能性がある。

ログ:

- 既存の Viewer LLM Ops proxy log を維持する。

エラー契約:

- 既存の挙動通り、登録時に error を返さない。

動かす前に確認するテスト:

- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw`

移動後に確認するコマンド:

- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw`
- route 登録に触った場合、必要なら live 起動後に `http://127.0.0.1:18790/health` を確認する。

live runtime 確認:

- 単純な関数分割だけなら必須ではない。
- `/viewer/*`、`/viewer/idlechat/*`、`/stt/*`、LLM Ops route の登録条件に触る場合は live 確認が必要。

### 第二候補: health / channel / local alias factory のファイル分離

route グループ化後の候補である。

- `buildHealthService` と health check 補助関数を `cmd/picoclaw/health_runtime.go` へ移す。
- channel registry 構築を `cmd/picoclaw/channel_runtime.go` へ移す。
- local LLM alias 解決を `cmd/picoclaw/local_llm_runtime.go` へ移す。

この候補はファイル移動であり、挙動変更を含めない。

## 検証方法

### Go test

最初の構成整理では、まず `cmd/picoclaw` の既存テストを確認する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

LLM provider、Application、Adapter へ影響が広がる場合は次を追加する。

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/...
GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/...
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/...
```

### health 確認

HTTP server 起動または route 登録条件に触った場合は、再起動手順に従ったうえで次を確認する。

```bash
curl -fsS http://127.0.0.1:18790/health
```

`/ready` は `cmdRun()` で登録されているが、LLM server 側の一般契約として前提にしない。運用確認の主対象は `/health` とする。

### Viewer / IdleChat / STT / TTS 追加確認

次に触る場合は実表示または実ブラウザ相当の確認を追加する。

- Viewer route group。
- IdleChat route group。
- STT route registration。
- TTS / VTuber / lipsync event。
- Viewer runtime config / LLM Ops 表示。

Viewer 関連では DOM 要素の存在だけで完了にしない。最低 1 セッションの表示本文、event log、終了状態を確認する。

### runtime config 確認

runtime config を伴う場合は、repo example ではなく live config を確認する。

- `~/.picoclaw/config.yaml`
- `http://127.0.0.1:18790/health`
- Viewer runtime config / LLM Ops 表示

## Phase 0 の完了条件

- 移動対象が明確である。
- 触らない対象が明確である。
- 最初の移行単位が小さい。
- 検証方法が明記されている。
- モジュール境界の入力、出力、副作用、ログ、エラー契約が書かれている。
- コード変更前にユーザーが判断できる状態である。

## 次にユーザーへ確認すること

最初の実装として、`cmdRun()` 内の HTTP route 登録を機能単位の registration 関数へ分ける方針で進めてよいか確認する。

確認したい実装単位:

1. route 登録だけを分ける。
2. handler の中身、契約、DTO、SSE event は変えない。
3. `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` で確認する。
4. live runtime は route 条件を変えた場合だけ確認する。
