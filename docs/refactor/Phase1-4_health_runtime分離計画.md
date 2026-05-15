# Phase1-4 health runtime 分離計画

## Phase 1-4 の目的

Phase 1-4 では、`cmd/picoclaw/main.go` に残っている health / status / doctor 系の CLI 処理と health service wiring を `cmd/picoclaw/health_runtime.go` へ分離する。

目的は次の通り。

- `cmd/picoclaw/main.go` を CLI dispatch と service 起動の入口として薄くする。
- `/health` / `/ready` の handler wiring に使う health service 構築を、独立した runtime 境界として読みやすくする。
- health / status / doctor の CLI 検証ロジックを、HTTP route registration や dependencies wiring から分ける。
- 挙動変更をせず、既存の health JSON、status JSON、doctor finding、LLM / TTS health check の意味を維持する。

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
- `docs/refactor/Phase1-2_route登録ファイル分離手順.md`
- `docs/refactor/Phase1-3_LLM_provider_factory分離計画.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `docs/codebase-map/modules/adapter.md`
- `docs/codebase-map/modules/infrastructure.md`
- `docs/codebase-map/modules/潜在バグ一覧.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/routes.go`
- `cmd/picoclaw/llm_runtime_factory.go`
- `cmd/picoclaw/main_status_health_test.go`

## docs/codebase-map/ との関係

`docs/codebase-map/modules/entrypoints_config_docs.md` は、`cmd/picoclaw/main.go` に CLI commands、health/status/doctor、runtime service が集中していると整理している。

`docs/codebase-map/結合ポイントマップ.md` は、`cmd/picoclaw/main.go` を runtime config、HTTP route、service restart、health に関わる結合点として示している。

Phase 1-4 ではこの解析を、health / status / doctor と health service wiring を `main.go` から分離する根拠として使う。ただし `docs/codebase-map/` は正本仕様ではないため、`/health` と `/ready` の意味や runtime config の扱いは現在コードと正本仕様を優先する。

現在コードとの差分リスク:

- Phase 1-2 により route registration は `routes.go` へ移動済みであり、`registerHealthRoutes` は `dependencies.buildHealthHandler(cfg)` を呼ぶだけになっている。
- `buildHealthService` は CLI の `cmdHealth` と HTTP handler の両方から使われるため、移動しても関数名と契約を変えない。
- codebase-map は live runtime config を読んでいないため、Phase 1-4 では `~/.picoclaw/config.yaml` の意味変更を行わない。

## 対象範囲

対象は `cmd/picoclaw/main.go` にある次の health / status / doctor 関連である。

- `cmdHealth`
- `cmdStatus`
- `type healthChecker`
- `runHealthCommand`
- `runStatusCommand`
- `cmdDoctor`
- `type doctorFinding`
- `runDoctorCommand`
- `loadExecutionStats`
- `loadEvidenceSummary`
- `buildHealthService`
- `buildLocalLLMHealthChecks`
- `collectOllamaHealthRequirements`
- `inferTTSDebugBaseURLFromConfig`
- `inferTTSDebugHealthPathFromConfig`

移動先:

- `cmd/picoclaw/health_runtime.go`

## 対象外

Phase 1-4 では次を変更しない。

- `/health` の JSON 契約。
- `/ready` の意味。
- health check の成功 / 失敗条件。
- status JSON の項目。
- doctor finding の severity、code、message。
- LLM endpoint、model、timeout の解決。
- TTS debug health path の推定。
- `registerHealthRoutes` の route path。
- `buildDependencies` の本体。
- LLM provider、STT provider、TTS provider の挙動。
- Viewer 表示契約、IdleChat 契約、SSE event、DTO。

## 現在の責務

現在 `main.go` は次を同時に持っている。

- CLI subcommand dispatch。
- `health` / `status` / `doctor` CLI の実装。
- runtime health service の構築。
- HTTP server 起動。
- dependencies wiring。
- route registration 呼び出し。

Phase 1-4 では health / status / doctor と health service wiring だけを `health_runtime.go` へ移す。

## 提案する分離単位

`cmd/picoclaw/health_runtime.go` は、`package main` のまま次の責務に限定する。

- health CLI の実行。
- status CLI の実行。
- doctor CLI の実行。
- health service の構築。
- local LLM health checks の構築。
- TTS debug health URL / path の推定。
- execution / evidence summary の読み取り。

置かないもの:

- HTTP route registration。
- Viewer handler。
- Dependencies 全体の組み立て。
- LLM provider factory 本体。
- STT / TTS provider factory 本体。
- Worker execution。
- 汎用 helper / util。

## 入力

- CLI args。
- `*config.Config`
- `healthChecker`
- output writer。
- 現在時刻を返す関数。
- execution report store。
- evidence report store。

## 出力

- CLI exit code。
- JSON 出力。
- doctor finding 一覧。
- health service。
- health check 一覧。
- execution status summary。
- evidence summary。

## 副作用

- `cmdHealth` / `cmdStatus` / `cmdDoctor` は config を読み込む。
- CLI commands は stdout / stderr へ出力する。
- `cmdHealth` は失敗時に process exit code を設定する。
- health check は設定された endpoint へ接続確認を行う可能性がある。
- execution / evidence summary は persistence を読み取る。

## 永続化

Phase 1-4 の対象は永続化へ書き込まない。

読み取り対象:

- execution report store。
- evidence report store。
- config path。

## ログ

Phase 1-4 では既存ログ文言を変更しない。

health / status / doctor CLI の主な観測は JSON / text 出力であり、ログ出力の追加は行わない。

## エラー契約

- config load 失敗は既存通り `log.Fatalf` または CLI error として扱う。
- health check 失敗は command exit code と JSON status に反映する。
- status summary の読み取り失敗は既存通り error として扱う。
- doctor finding は既存通り severity と code を含む。

Phase 1-4 ではこの契約を変えない。

## 変更してはいけない既存挙動

- `cmdHealth` が `buildHealthService(cfg)` を使うこと。
- `cmdStatus` が health、execution stats、evidence summary を出力すること。
- `cmdDoctor` の finding 判定。
- local LLM health checks の alias、base URL、model、timeout。
- TTS debug base URL / health path の推定。
- `/ready` を LLM server 一般仕様として扱わないこと。
- repo example config と live runtime config を混同しないこと。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmd/picoclaw/health_runtime.go` を作成する。
3. 対象の型と関数を `main.go` から `health_runtime.go` へ移す。
4. 関数名、型名、引数、戻り値を変更しない。
5. health/status/doctor の出力形式を変更しない。
6. `main.go` から不要 import を削除する。
7. `health_runtime.go` の import を最小化する。
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
git diff -- cmd/picoclaw/main.go cmd/picoclaw/health_runtime.go
git diff --check
git diff --stat
```

関数所在確認:

```bash
rg -n "func (cmdHealth|cmdStatus|runHealthCommand|runStatusCommand|cmdDoctor|runDoctorCommand|loadExecutionStats|loadEvidenceSummary|buildHealthService|buildLocalLLMHealthChecks|collectOllamaHealthRequirements|inferTTSDebugBaseURLFromConfig|inferTTSDebugHealthPathFromConfig)|type (healthChecker|doctorFinding)" cmd/picoclaw/main.go cmd/picoclaw/health_runtime.go
```

live health:

- Phase 1-4 は配置変更のみのため原則不要。
- health check の意味、HTTP route、server 起動に触れた場合のみ `http://127.0.0.1:18790/health` を確認する。

## リスク

- CLI 出力形式を変えてしまう。
- health check の alias / model / timeout 解決を崩す。
- `/ready` を LLM server の一般 health と誤解する。
- execution / evidence summary の error handling を変えてしまう。
- `buildHealthService` の移動により HTTP handler 側の参照を壊す。
- `health_runtime.go` に health 以外の責務を入れてしまう。

## 完了条件

- `docs/refactor/Phase1-4_health_runtime分離計画.md` が作成されている。
- health / status / doctor の移動対象が明記されている。
- `health_runtime.go` の責務が明記されている。
- 入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- 計画文書が Push されている。
- 実装後、`cmd/picoclaw/main.go` から対象関数が分離されている。
- 実装後、`GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- 実装後、`git diff --check` が成功している。
- 実装差分が Push されている。
- health/status/doctor の挙動を変更していない。
