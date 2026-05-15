# Phase1-5 CLI operations 分離計画

## Phase 1-5 の目的

Phase 1-5 では、`cmd/picoclaw/main.go` に残っている CLI subcommand 本体を `cmd/picoclaw/cli_operations.go` へ分離する。

目的は次の通り。

- `main.go` を CLI dispatch、`cmdRun()`、server startup の入口として薄くする。
- channels / gateway / ollama / logs / evidence / source-registry / knowledge / help などの CLI operations を runtime service wiring から分ける。
- CLI の出力形式、引数解釈、store 読み取り、error handling を変更しない。
- Phase 1-4 で分離した health runtime と同じく、CLI 実装を `main.go` に残さない。

## 参照資料

- `AGENTS.md`
- `CLAUDE.md`
- `docs/01_正本仕様/実装仕様.md`
- `docs/refactor/リファクタリング指針.md`
- `docs/refactor/フォルダ構成方針.md`
- `docs/refactor/段階移行計画.md`
- `docs/refactor/検証方針.md`
- `docs/refactor/Phase0_cmd構成整理計画.md`
- `docs/refactor/Phase1-3_LLM_provider_factory分離計画.md`
- `docs/refactor/Phase1-4_health_runtime分離計画.md`
- `docs/codebase-map/アーキテクチャ総合.md`
- `docs/codebase-map/結合ポイントマップ.md`
- `docs/codebase-map/modules/entrypoints_config_docs.md`
- `docs/codebase-map/modules/潜在バグ一覧.md`
- `cmd/picoclaw/main.go`
- `cmd/picoclaw/main_channels_test.go`
- `cmd/picoclaw/main_ollama_cli_test.go`
- `cmd/picoclaw/main_ops_cli_test.go`
- `cmd/picoclaw/main_logs_test.go`
- `cmd/picoclaw/main_source_registry_cli_test.go`

## docs/codebase-map/ との関係

`docs/codebase-map/modules/entrypoints_config_docs.md` は、`cmd/picoclaw/main.go` が CLI commands と runtime service の両方を持つと整理している。

Phase 1-5 では、CLI commands を `cli_operations.go` へ分離し、`main.go` を dispatch と service 起動に寄せる。`docs/codebase-map/` は補助解析資料であり、CLI 出力や store 契約の正本ではない。

現在コードとの差分リスク:

- Phase 1-4 により health / status / doctor は `health_runtime.go` へ分離済みである。
- `hasFlag` と `writeJSONCLI` は health runtime からも使われるため、Phase 1-5 で移動しても関数名を変えない。
- CLI は live runtime service を起動しないものが多いため、Phase 1-5 では live health 確認を原則不要とする。

## 対象範囲

対象は `cmd/picoclaw/main.go` にある次の CLI operations 関連である。

- `cmdVersion`
- `lineWebhookConfigured`
- `cmdChannels`
- `buildChannelRegistry`
- `buildOutboundChannelRegistry`
- `type channelRegistry`
- `runChannelsCommand`
- `cmdGateway`
- `cmdOllama`
- `runGatewayCommand`
- `runOllamaCommand`
- `gatewayHealthURL`
- `buildOllamaRestartAction`
- `isLocalOllamaHost`
- `cmdLogs`
- `runLogsCommand`
- `cmdEvidence`
- `type evidenceStore`
- `cmdSourceRegistry`
- `cmdKnowledge`
- `type sourceRegistryCLIStore`
- `type knowledgeCLIStore`
- `runKnowledgeCommand`
- `runSourceRegistryCommand`
- `runEvidenceCommand`
- `parseEvidenceListArgs`
- `parseSourceRegistrySaveArgs`
- `parseSourceRegistryDisableArgs`
- `parseSourceRegistrySweepArgs`
- `sourceRegistrySweepResultCLI`
- `sourceRegistryCLIEntries`
- `sourceRegistryCLIEntry`
- `filterEvidence`
- `summarizeEvidence`
- `hasFlag`
- `writeJSONCLI`
- `loadEvidenceStore`
- `loadSourceRegistryStore`
- `printLastLines`
- `printLastLinesTo`
- `followFile`
- `followFileTo`
- `cmdHelp`

移動先:

- `cmd/picoclaw/cli_operations.go`

## 対象外

Phase 1-5 では次を変更しない。

- CLI subcommand 名。
- CLI 引数。
- CLI 出力 JSON / text の形式。
- exit code。
- gateway / ollama command の実行内容。
- channels registry の組み立て。
- evidence / source registry / knowledge store の読み取り・保存契約。
- health / status / doctor の実装。
- HTTP server 起動。
- route registration。
- `buildDependencies`。
- Viewer、IdleChat、STT/TTS、LLM provider の挙動。

## 現在の責務

現在 `main.go` は CLI dispatch と CLI operation 本体を同時に持っている。Phase 1-5 では CLI dispatch は `main.go` に残し、operation 本体を `cli_operations.go` へ移す。

## 提案する分離単位

`cmd/picoclaw/cli_operations.go` は、`package main` のまま次の責務に限定する。

- CLI subcommand の実行本体。
- CLI 用 registry / store loader。
- CLI 用 args parser。
- CLI 用 JSON / text output helper。
- CLI 用 logs tail / follow helper。

置かないもの:

- `main()` の dispatch。
- `cmdRun()`。
- HTTP route registration。
- `Dependencies` の組み立て。
- provider factory 本体。
- Worker execution 本体。

## 入力

- CLI args。
- `*config.Config`
- registry / store interface。
- stdout / stderr writer。
- current time provider。
- file path。

## 出力

- CLI exit code。
- JSON / text output。
- parsed args。
- registry / store。
- evidence / source registry summary。

## 副作用

- CLI command によって filesystem read / write、process execution、HTTP request、store read / write が発生する。
- logs command は log file を tail / follow する。
- source registry command は save / disable / sweep により store を更新する。
- ollama command は restart action を実行する場合がある。

## 永続化

対象 CLI は次の永続化に関わる。

- execution evidence JSONL store。
- Source Registry SQLite store。
- Knowledge CLI store。
- log file read。

Phase 1-5 では永続化先や schema を変更しない。

## ログ

CLI 出力を主たる観測結果とし、Phase 1-5 ではログ文言を変更しない。

## エラー契約

- 引数不正は既存の error message / exit code を維持する。
- store load 失敗は既存通り CLI error として返す。
- gateway / ollama command の HTTP / process error handling を変更しない。
- logs file read / follow の error handling を変更しない。

## 変更してはいけない既存挙動

- CLI 出力形式。
- `--json`、`--limit`、`--status`、`--error-kind`、`--since-hours` などの意味。
- Source Registry の save / disable / sweep の意味。
- Evidence summary の分類。
- Ollama restart の local host 判定。
- `hasFlag` と `writeJSONCLI` の挙動。

## 実装手順

1. baseline test を実行する。

   ```bash
   GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
   ```

2. `cmd/picoclaw/cli_operations.go` を作成する。
3. 対象の型と関数を `main.go` から `cli_operations.go` へ移す。
4. 関数名、型名、引数、戻り値を変更しない。
5. CLI 出力と exit code を変更しない。
6. `main.go` と `cli_operations.go` の import を整理する。
7. `gofmt` を実行する。
8. after test を実行する。

## 検証手順

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
git diff --check
git diff --stat
```

関数所在確認:

```bash
rg -n "func (cmdVersion|cmdChannels|runChannelsCommand|cmdGateway|cmdOllama|runGatewayCommand|runOllamaCommand|cmdLogs|runLogsCommand|cmdEvidence|cmdSourceRegistry|cmdKnowledge|runKnowledgeCommand|runSourceRegistryCommand|runEvidenceCommand|cmdHelp)|type (channelRegistry|evidenceStore|sourceRegistryCLIStore|knowledgeCLIStore)" cmd/picoclaw/main.go cmd/picoclaw/cli_operations.go
```

## リスク

- CLI 出力形式を変えてしまう。
- `hasFlag` / `writeJSONCLI` を health runtime から参照できなくする。
- source registry CLI の永続化契約を変えてしまう。
- ollama restart の安全判定を崩す。
- `main.go` の dispatch から呼ぶ関数名を変えてしまう。

## 完了条件

- `docs/refactor/Phase1-5_CLI_operations分離計画.md` が作成されている。
- CLI operations の移動対象が明記されている。
- `cli_operations.go` の責務が明記されている。
- 入力、出力、副作用、永続化、ログ、エラー契約が書かれている。
- 計画文書が Push されている。
- 実装後、`cmd/picoclaw/main.go` から対象関数が分離されている。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `git diff --check` が成功している。
- 実装差分が Push されている。
