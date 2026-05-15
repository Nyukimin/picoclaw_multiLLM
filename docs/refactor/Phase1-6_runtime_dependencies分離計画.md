# Phase1-6 runtime dependencies 分離計画

## Phase 1-6 の目的

Phase 1-6 では、`cmd/picoclaw/main.go` に残っている runtime dependency wiring を `cmd/picoclaw/runtime_dependencies.go` へ分離する。

目的は次の通り。

- `main.go` を `main()`、`cmdRun()`、config path、server startup の入口として薄くする。
- `Dependencies`、`buildDependencies`、shutdown、distributed/local agent、IdleChat HTTP handler、coder setup を runtime dependency wiring 境界としてまとめる。
- route registration、LLM provider factory、health runtime、CLI operations と分離された状態にする。
- 挙動変更をせず、既存の初期化順、handler 契約、event、log、fallback/error の扱いを維持する。

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
- `docs/refactor/Phase1-4_health_runtime分離計画.md`
- `docs/refactor/Phase1-5_CLI_operations分離計画.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/ユースケース逆引き.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `docs/codebase-map/modules/application.md`
- `docs/codebase-map/modules/adapter.md`
- `docs/codebase-map/modules/infrastructure.md`
- `docs/codebase-map/modules/潜在バグ一覧.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/routes.go`
- `cmd/picoclaw/llm_runtime_factory.go`
- `cmd/picoclaw/health_runtime.go`
- `cmd/picoclaw/cli_operations.go`
- `cmd/picoclaw/*_test.go`

## docs/codebase-map/ との関係

`docs/codebase-map/結合ポイントマップ.md` は、`cmd/picoclaw/main.go` を Config、LLM provider、stores、Viewer、channels、IdleChat、transports が集中する主要結合点として示している。

`docs/codebase-map/modules/entrypoints_config_docs.md` は、`main.go` に dependencies build、adapters、application、infrastructure、runtime service が集中していると整理している。

Phase 1-6 では、この集中点のうち runtime dependency wiring を `runtime_dependencies.go` へ分離する。ただし `docs/codebase-map/` は正本仕様ではないため、Chat / Worker / Coder の責務境界、IdleChat 契約、Viewer 表示契約、fallback/error の扱いは正本仕様と現在コードを優先する。

現在コードとの差分リスク:

- Phase 1-1 / 1-2 により route registration は `routes.go` へ分離済みである。
- Phase 1-3 により LLM provider factory は `llm_runtime_factory.go` へ分離済みである。
- Phase 1-4 により health runtime は `health_runtime.go` へ分離済みである。
- Phase 1-5 により CLI operations は `cli_operations.go` へ分離済みである。
- `Dependencies` はまだ大きい境界だが、Phase 1-6 では既存 wiring の配置変更に限定し、新しい manager / helper / util を作らない。

## 対象範囲

対象は `cmd/picoclaw/main.go` にある次の runtime dependency wiring 関連である。

- `type coderAdapter`
- `startSourceRegistrySweeper`
- `startParquetExportJob`
- `type Dependencies`
- `type idleChatStartGate`
- `type idleAwareEventListener`
- `shouldStopIdleChatByEvent`
- `Dependencies.Shutdown`
- `buildDependencies`
- `buildCoderCapabilities`
- `type channelNotificationSender`
- `buildHeartbeatNotificationSender`
- `channelNotificationSender.SendNotification`
- `Dependencies.buildDistributedMode`
- local / distributed agent helper 群
- IdleChat HTTP handler 群
- `writeJSON`
- `Dependencies.buildHealthHandler`
- `resolveSubagentProvider`
- `mustGetToolList`
- `setupCoders`
- `coderConfigWithRuntimePersonality`
- `resolveCoderPersonality`

移動先:

- `cmd/picoclaw/runtime_dependencies.go`

## 対象外

Phase 1-6 では次を変更しない。

- `cmdRun()` の初期化順。
- `buildDependencies(cfg)` の呼び出し位置。
- `Dependencies` の field。
- channel / Viewer / IdleChat / Source Registry / Memory / TTS / STT / LLM provider の挙動。
- Chat / Worker / Coder の責務境界。
- Worker execution の安全境界。
- local / SSH transport の実行方式。
- IdleChat raw / view / audio 契約。
- fallback を成功として扱わない方針。
- Viewer 表示、音声、口パク、ログを混同しない方針。
- runtime config の意味。
- route path。

## 現在の責務

現在 `main.go` は次を同時に持っている。

- `main()` の CLI dispatch。
- `cmdRun()` の service startup。
- runtime dependency wiring。
- distributed/local agent wiring。
- IdleChat HTTP handler。
- coder setup。

Phase 1-6 では `main.go` に dispatch と startup を残し、runtime dependency wiring を `runtime_dependencies.go` へ移す。

## 提案する分離単位

`cmd/picoclaw/runtime_dependencies.go` は、`package main` のまま次の責務に限定する。

- Application / Adapter / Infrastructure の runtime wiring。
- `Dependencies` lifecycle。
- distributed/local agent wiring。
- IdleChat route handler の接続先実装。
- coder setup。
- runtime background job の起動。

置かないもの:

- `main()`。
- `cmdRun()`。
- CLI operations。
- route registration。
- LLM provider factory。
- health CLI / health service factory。
- provider 固有 HTTP request 実装。
- Worker execution 本体。
- Viewer JS / CSS。

## 入力

- `*config.Config`
- runtime config から生成された provider / store / adapter。
- distributed agent 設定。
- HTTP request。
- transport message。

## 出力

- `*Dependencies`
- HTTP handler。
- coder adapter。
- distributed mode result。
- local agent response。
- JSON response。

## 副作用

- Adapter / Application / Infrastructure の初期化。
- goroutine 起動。
- Source Registry sweeper 起動。
- parquet export job 起動。
- local transport loop 起動。
- event relay / Viewer event 送信。
- HTTP response 書き込み。
- log 出力。

## 永続化

Phase 1-6 の対象は次の永続化に関わる。

- Source Registry / L1 SQLite store。
- memory persistence。
- session persistence。
- execution report store。
- parquet export。
- tool registry。

Phase 1-6 では永続化先、schema、state transition を変更しない。

## ログ

既存ログ文言と条件を維持する。

特に local worker / local coder / distributed transport / IdleChat prepare / setupCoders のログを変更しない。

## エラー契約

- fatal error の条件を変更しない。
- local agent error response の type / content を変更しない。
- IdleChat start gate の conflict / bad gateway の扱いを変更しない。
- provider / store / transport 初期化失敗時の扱いを変更しない。

## 変更してはいけない既存挙動

- `buildDependencies` の初期化順。
- `Dependencies.Shutdown` の停止対象。
- IdleChat route handler の HTTP method / status code / JSON。
- local worker / local coder の message delivery。
- Coder は proposal 生成、Worker は実行という責務境界。
- `resolveSubagentProvider` の provider 選択。
- `setupCoders` の persona / LightMemory 設定。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmd/picoclaw/runtime_dependencies.go` を作成する。
3. 対象の型と関数を `main.go` から `runtime_dependencies.go` へ移す。
4. 関数名、型名、field、引数、戻り値を変更しない。
5. `cmdRun()` の呼び出し順を変更しない。
6. IdleChat handler の中身を変更しない。
7. local / distributed agent の実行経路を変更しない。
8. import を整理する。
9. `gofmt` を実行する。
10. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
git diff --check
git diff --stat
```

関数所在確認:

```bash
rg -n "type (Dependencies|coderAdapter|idleChatStartGate|idleAwareEventListener|channelNotificationSender|sshTransportConnector)|func (buildDependencies|buildCoderCapabilities|buildHeartbeatNotificationSender|localAgentEnabled|registerSSHTransport|markAgentUnavailable|formatAgentUnavailableReason|distributedAgentAvailable|handleLocalWorkerMessage|newLocalAgentError|localCoderReplyTarget|writeJSON|resolveSubagentProvider|mustGetToolList|setupCoders|coderConfigWithRuntimePersonality|resolveCoderPersonality)" cmd/picoclaw/main.go cmd/picoclaw/runtime_dependencies.go
```

live health:

- Phase 1-6 は配置変更のみのため原則不要。
- server startup、route path、runtime config の意味に触れた場合のみ `http://127.0.0.1:18790/health` を確認する。

## リスク

- `buildDependencies` の初期化順を崩す。
- IdleChat handler の status code / JSON を変える。
- local worker / local coder の返信先を崩す。
- Coder / Worker の責務境界を曖昧にする。
- `runtime_dependencies.go` が大きくなりすぎる。
- import 整理時に必要な provider / adapter 参照を落とす。

`runtime_dependencies.go` が大きいことは既存集中点の移動結果であり、Phase 1-6 では新しい抽象化を作らない。Phase 2 以降で責務別分割が必要になった場合は、別 Phase として扱う。

## 完了条件

- `docs/refactor/Phase1-6_runtime_dependencies分離計画.md` が作成されている。
- runtime dependency wiring の移動対象が明記されている。
- `runtime_dependencies.go` の責務が明記されている。
- 入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 計画文書が Push されている。
- 実装後、`cmd/picoclaw/main.go` から対象関数が分離されている。
- `cmd/picoclaw/main.go` が composition root として説明できる。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `git diff --check` が成功している。
- 実装差分が Push されている。
