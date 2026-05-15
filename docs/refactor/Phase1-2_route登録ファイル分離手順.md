# Phase1-2 route登録ファイル分離手順

## Phase 1-2 の目的

Phase 1-2 では、Phase 1-1 で `cmd/picoclaw/main.go` 内に追加した route registration 関数群を `cmd/picoclaw/routes.go` へ移す。

目的は次の通り。

- `cmd/picoclaw/main.go` をさらに薄くする。
- route registration 関数群を `routes.go` へ移す。
- 挙動変更なしで、ファイル責務を分ける。
- 将来 Adapter 側へ移すための中間段階として、`cmd/` 配下で route registration 境界を明確にする。
- モジュール化と疎結合を守り、単なる巨大ファイル移動にしない。

## 参照資料

- `AGENTS.md`
- `CLAUDE.md`
- `docs/01_正本仕様/実装仕様.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/段階移行計画.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase0_cmd構成整理計画.md`
- `docs/refactor/Phase1-1_route登録分割仕様.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/*_test.go`

## 対象範囲

対象は `cmd/picoclaw/main.go` にある次の route registration 関数群である。

- `registerChannelRoutes`
- `registerViewerBaseRoutes`
- `registerLLMOpsRoutes`
- `registerSTTAndAudioRoutes`
- `registerViewerDynamicRoutes`
- `registerEntryAndChromeRoutes`
- `registerIdleChatRoutes`
- `registerHealthRoutes`

移動先:

- `cmd/picoclaw/routes.go`

## 対象外

Phase 1-2 では次を変更しない。

- `cmdRun()` の呼び出し順。
- route path。
- nil handler 条件。
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

## 移動する関数一覧

### `registerChannelRoutes`

現在の責務:

- `/webhook` を登録する。
- telegram / discord / slack handler が nil でない場合だけ route を登録する。

移動後の責務:

- `routes.go` で channel webhook route registration のみを担当する。
- handler 本体や channel adapter 初期化は持たない。

### `registerViewerBaseRoutes`

現在の責務:

- Viewer page、assets、runtime config、logo、lip sync SVG、TTS audio、SSE、debug system、assets git status を登録する。

移動後の責務:

- `routes.go` で Viewer の静的 / runtime route 接続だけを担当する。
- Viewer handler の中身や runtime config の意味解決は持たない。

### `registerLLMOpsRoutes`

現在の責務:

- `debugSystemOpts.LLMOpsEnabled` が true の場合だけ `/viewer/llm-ops/*` を登録する。
- `dependencies.idleChatStartGate` を設定する。
- 既存の LLM Ops proxy log を出す。

移動後の責務:

- `routes.go` で LLM Ops route と IdleChat start gate の既存接続だけを担当する。
- LLM provider の起動、停止、状態判定の意味変更は持たない。

### `registerSTTAndAudioRoutes`

現在の責務:

- Viewer STT log / wav / autotest route を登録する。
- `registerSTTRuntimeRoutes(mux, sttRuntime)` を呼ぶ。
- `/audio-router/events` を登録する。

移動後の責務:

- `routes.go` で STT / audio route 接続だけを担当する。
- STT provider、WebSocket proxy、TTS provider の挙動変更は持たない。

### `registerViewerDynamicRoutes`

現在の責務:

- Viewer monitor、evidence、glossary、memory、source registry、viewer send route を nil check 付きで登録する。

移動後の責務:

- `routes.go` で Viewer 動的 API route 接続だけを担当する。
- memory / source registry / evidence の状態遷移や表示契約は持たない。

### `registerEntryAndChromeRoutes`

現在の責務:

- `/entry` と `/chrome/bridge*` route を nil check 付きで登録する。

移動後の責務:

- `routes.go` で entry / chrome bridge route 接続だけを担当する。
- entry stage handling や chrome bridge event 実装は持たない。

### `registerIdleChatRoutes`

現在の責務:

- `dependencies.idleChatOrch` が nil でない場合だけ `/viewer/idlechat/*` route を登録する。

移動後の責務:

- `routes.go` で IdleChat HTTP route 接続だけを担当する。
- IdleChat raw / view / audio 契約、生成、sanitize、fallback / invalid response 判定は持たない。

### `registerHealthRoutes`

現在の責務:

- `dependencies.buildHealthHandler(cfg)` を呼び、`/health` と `/ready` を登録する。

移動後の責務:

- `routes.go` で health route 接続だけを担当する。
- `/health` の意味や `/ready` の扱いを変更しない。

## `routes.go` のファイル責務

`cmd/picoclaw/routes.go` は次の責務に限定する。

- `package main` のままにする。
- HTTP route registration 専用ファイルとする。
- route path と handler 接続の対応だけを置く。
- nil handler 条件を既存通り保持する。
- `cmdRun()` から呼ばれる private registration 関数を置く。

置かないもの:

- handler の中身。
- provider setup。
- runtime config の意味解決。
- route dispatch。
- Worker execution。
- IdleChat 生成・表示契約。
- STT / TTS provider 挙動。
- LLM provider 挙動。
- 汎用 helper / util。

## 必要な import 方針

`routes.go` に必要な import:

- `log`
- `net/http`
- `os`
- `strings`
- `github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/config`
- `github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer`

`main.go` から不要になる可能性がある import:

- route registration 関数だけで使っていた import があれば削除する。
- ただし `main.go` の他の関数で使っている import は削除しない。

方針:

- `gofmt` を実行する。
- 必要に応じて `go test` で import 過不足を検出する。
- import のためだけに `routes.go` の責務を広げない。
- `routes.go` が新しい巨大ファイルになり始めたら、次の Phase で route group ごとの分割を検討する。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmd/picoclaw/routes.go` を作成する。
3. route registration 関数群を `main.go` から `routes.go` へ移す。
4. `cmdRun()` の呼び出し順は変更しない。
5. route path と nil check 条件を変更しない。
6. `main.go` の不要 import を削除する。
7. `routes.go` の import を最小化する。
8. `gofmt` を実行する。
9. after test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
```

差分確認:

```bash
git diff -- cmd/picoclaw/main.go cmd/picoclaw/routes.go
```

route path が落ちていないことを確認する。

```bash
rg -n '"/(webhook|viewer|entry|chrome|health|ready|stt|stt-ws|ws|audio-router)' cmd/picoclaw/main.go cmd/picoclaw/routes.go
```

live health は原則不要である。ただし route 条件や server 起動に触った場合のみ、再起動手順に従って確認する。

```bash
curl -fsS http://127.0.0.1:18790/health
```

## リスク

- `main.go` から必要 import を消しすぎる。
- `routes.go` に不要 import を増やす。
- route path の登録漏れ。
- nil handler 条件の崩れ。
- LLM Ops start gate の設定順を崩す。
- `debugSystemOpts` の受け渡しを変える。
- STT 互換 route `/stt-ws` / `/ws` を落とす。
- `routes.go` が新しい巨大ファイルになる。
- `routes.go` に provider setup や handler 本体が混ざり始める。

## 完了条件

- `docs/refactor/Phase1-2_route登録ファイル分離手順.md` が作成されている。
- 移動対象関数が明記されている。
- `routes.go` の責務が明記されている。
- 実装手順が書かれている。
- 検証手順が書かれている。
- コード変更は行っていない。
- ユーザーが次に「実装してよいか」を判断できる。

## 次に確認すること

実装に進む場合は、次の範囲でよいか確認する。

1. route registration 関数群だけを `cmd/picoclaw/routes.go` へ移す。
2. `cmdRun()` の呼び出し順、route path、nil check 条件は変更しない。
3. handler の中身、DTO、SSE event、IdleChat 契約は変えない。
4. baseline / after で `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` を実行する。
