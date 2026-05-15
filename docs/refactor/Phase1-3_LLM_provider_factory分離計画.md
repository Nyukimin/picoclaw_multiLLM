# Phase1-3 LLM provider factory 分離計画

## Phase 1-3 の目的

Phase 1-3 では、`cmd/picoclaw/main.go` に残っている LLM provider factory 関連の関数を `cmd/picoclaw/llm_runtime_factory.go` へ分離する。

目的は次の通り。

- `cmd/picoclaw/main.go` を composition root としてさらに薄くする。
- Chat / Worker / Heavy / Wild の provider wiring を、HTTP server 起動や CLI entrypoint から分ける。
- conversation summary provider と embedding provider の runtime factory を同じ LLM runtime 境界に置く。
- 挙動変更をせず、既存の provider interface、raw log middleware、timeout、warmup、model alias 解決を維持する。
- 将来 LLM provider factory を `internal/infrastructure/llm` または Adapter/Application 境界へ移す前の中間段階として、`cmd/` 配下で責務を明確にする。

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
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `docs/codebase-map/modules/infrastructure.md`
- `docs/codebase-map/modules/adapter.md`
- `docs/codebase-map/modules/潜在バグ一覧.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/routes.go`
- `cmd/picoclaw/main_local_llm_test.go`
- `cmd/picoclaw/main_conversation_provider_test.go`

## docs/codebase-map/ との関係

`docs/codebase-map/modules/entrypoints_config_docs.md` は、`cmd/picoclaw/main.go` が provider build、conversation embedder / text provider、dependencies build、HTTP routes をまとめていると整理している。

`docs/codebase-map/結合ポイントマップ.md` は、LLM provider factory を runtime config、factory、middleware rawlog、health、Viewer LLM ops に関わる結合点として示している。

Phase 1-3 ではこの解析を、LLM provider factory を `main.go` から外す根拠として使う。ただし `docs/codebase-map/` は正本仕様ではないため、実装判断は `docs/01_正本仕様/実装仕様.md` と現在コードを優先する。

現在コードとの差分リスク:

- codebase-map は `main.go` に route 登録が集中している前提を含むが、Phase 1-1 / 1-2 により route registration は `routes.go` へ分離済みである。
- codebase-map は live runtime config を読んでいないため、Phase 1-3 では runtime config の意味変更を行わない。
- `main.go` の LLM provider factory は `buildDependencies`、health check、Viewer debug options から参照されるため、移動時は関数名と契約を変えない。

## 対象範囲

対象は `cmd/picoclaw/main.go` にある次の LLM provider factory 関連である。

- `type primaryLLMProviders`
- `const localLLMDefaultTimeout`
- `const localLLMChatTimeout`
- `const localLLMWildTimeout`
- `const localLLMHeavyTimeout`
- `buildPrimaryLLMProviders`
- `buildConversationTextProvider`
- `buildConversationEmbedder`
- `buildLocalAliasProvider`
- `localLLMTimeoutForAlias`
- `localLLMBaseURLForAlias`
- `localLLMModelForAlias`
- `firstNonEmpty`
- `maxDuration`
- `warmPrimaryLLMProviders`

移動先:

- `cmd/picoclaw/llm_runtime_factory.go`

## 対象外

Phase 1-3 では次を変更しない。

- LLM provider の選択ロジック。
- Chat / Worker / Heavy / Wild の alias 解決。
- timeout 値。
- model fallback。
- raw log middleware の有無。
- date/time middleware の有無。
- warmup の実行条件。
- conversation summary provider の選択。
- embedding provider の選択。
- `buildDependencies` の処理内容。
- health check の処理内容。
- Viewer LLM Ops route。
- `~/.picoclaw/config.yaml` の意味。
- repo example config。
- provider 固有実装。
- DTO、SSE event、Viewer 表示契約、IdleChat 契約、STT/TTS provider 挙動。

## 現在の責務

現在 `main.go` は次の責務を同時に持っている。

- CLI entrypoint。
- config load。
- signal handling。
- HTTP server 起動。
- route registration 呼び出し。
- LLM provider factory。
- conversation summary / embedder factory。
- dependencies wiring。
- health check wiring。
- command handlers。

このうち Phase 1-3 は、LLM provider factory のみを `llm_runtime_factory.go` へ移す。

## 提案する分離単位

`cmd/picoclaw/llm_runtime_factory.go` は、`package main` のまま次の責務に限定する。

- primary LLM provider の生成。
- local LLM alias の base URL / model / timeout 解決。
- conversation summary provider の生成。
- conversation embedder の生成。
- local LLM warmup の実行。
- provider factory に必要な小さな文字列・duration helper。

置かないもの:

- `cmdRun()`。
- HTTP route registration。
- Viewer handler。
- LLM Ops HTTP proxy route。
- health endpoint handler。
- `Dependencies` の組み立て本体。
- provider 固有 HTTP request の実装。
- 汎用 helper / util。

## 入力

- `*config.Config`
- `primaryLLMProviders`
- alias 名。
- model 名。
- timeout。
- global concurrency semaphore。
- warmup 用 `context.Context`
- warmup 対象 provider map。

## 出力

- `primaryLLMProviders`
- `llm.LLMProvider`
- `conversation.EmbeddingProvider`
- provider 表示 label。
- alias ごとの base URL。
- alias ごとの model。
- alias ごとの timeout。
- 最大 duration。

## 副作用

- `buildPrimaryLLMProviders` は local LLM warmup が有効な場合に goroutine を起動する。
- `warmPrimaryLLMProviders` は provider に最小 token の generate request を送る。
- `warmPrimaryLLMProviders` は warmup 成否を log に出す。
- provider 生成時に raw log middleware / date time middleware / limited provider を組み合わせる。

## 永続化

Phase 1-3 の対象関数は永続化を直接行わない。

ただし raw log middleware は provider 呼び出し時のログ出力や診断に関係するため、middleware の有無や label を変更しない。

## ログ

既存のログ契約を維持する。

- `WARN: local LLM warmup failed alias=%s provider=%s err=%v`
- `Local LLM warmup ok alias=%s provider=%s`

Phase 1-3 ではログ文言、ログ条件、provider label を変更しない。

## エラー契約

- provider factory は、現在と同じく多くの場合に error を返さず provider または nil を返す。
- `buildConversationTextProvider` は summary model がない場合に `nil, ""` を返す。
- `buildConversationEmbedder` は embed model がない場合に `nil, ""` を返す。
- warmup の error は log へ記録し、起動処理を止めない。

Phase 1-3 ではこの契約を変えない。

## 変更してはいけない既存挙動

- `cfg.LocalLLM.Enabled` の場合に Chat / Worker / Heavy / Wild provider を local LLM 設定から作ること。
- local LLM disabled の場合に Ollama provider を使うこと。
- Worker model 空欄時に chat model を使う fallback。
- Heavy base URL 空欄かつ Worker base URL がある場合に Heavy model として Worker model を使うこと。
- Chat / Wild / Heavy の timeout 固定値。
- Worker timeout が `cfg.LocalLLM.TimeoutSec` または default timeout に従うこと。
- raw log provider の role label。
- date/time provider の付与。
- local LLM warmup が有効な場合だけ warmup goroutine を起動すること。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmd/picoclaw/llm_runtime_factory.go` を作成する。
3. 対象関数と定数を `main.go` から `llm_runtime_factory.go` へ移す。
4. 関数名、型名、定数名、引数、戻り値を変更しない。
5. provider 選択、timeout、model fallback、middleware、warmup 条件を変更しない。
6. `main.go` から不要 import を削除する。
7. `llm_runtime_factory.go` の import を最小化する。
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
git diff -- cmd/picoclaw/main.go cmd/picoclaw/llm_runtime_factory.go
git diff --check
git diff --stat
```

関数所在確認:

```bash
rg -n "func (buildPrimaryLLMProviders|buildConversationTextProvider|buildConversationEmbedder|buildLocalAliasProvider|localLLMTimeoutForAlias|localLLMBaseURLForAlias|localLLMModelForAlias|firstNonEmpty|maxDuration|warmPrimaryLLMProviders)|type primaryLLMProviders|localLLMDefaultTimeout" cmd/picoclaw/main.go cmd/picoclaw/llm_runtime_factory.go
```

live health:

- Phase 1-3 は provider factory の配置だけを変えるため原則不要。
- runtime config の意味、server 起動、health wiring に触れた場合のみ `http://127.0.0.1:18790/health` を確認する。

## リスク

- `main.go` から import を消しすぎる。
- `llm_runtime_factory.go` に provider 以外の責務を入れる。
- raw log middleware の label を変える。
- date/time middleware を落とす。
- Heavy alias の base URL / model fallback を崩す。
- Worker timeout の default を崩す。
- warmup goroutine の条件や timeout を変える。
- conversation summary / embedder の provider 選択を変える。
- repo example config と live runtime config の区別を曖昧にする。

## 完了条件

- `docs/refactor/Phase1-3_LLM_provider_factory分離計画.md` が作成されている。
- LLM provider factory の移動対象が明記されている。
- `llm_runtime_factory.go` の責務が明記されている。
- 入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 実装手順と検証手順が書かれている。
- 計画文書が Push されている。
- 実装後、`cmd/picoclaw/main.go` から対象関数が分離されている。
- 実装後、`GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- 実装後、`git diff --check` が成功している。
- 実装差分が Push されている。
- handler、DTO、SSE event、Viewer 表示契約、IdleChat 契約、STT/TTS provider、LLM provider 挙動を変更していない。
