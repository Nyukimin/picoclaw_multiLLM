# Phase30 config / ToolRunner 分割実装仕様

## 目的

Phase30 は、Phase29 後に残っている大型 production file のうち、設定読み込みと tool 実行の責務集中を分割するための構造整理である。

対象は次の 2 ファイルとする。

- `internal/adapter/config/config.go`
- `internal/infrastructure/tools/runner.go`

目的は、設定仕様変更時と tool 実行仕様変更時に触る主担当ファイルを説明できる状態へ近づけることである。挙動変更は行わず、top-level declaration の移動を主作業とする。

## 対象範囲

### config

対象:

- `internal/adapter/config/config.go`

分離単位:

- `config.go`
  - `LoadConfig`
  - package の入口としての最小責務
- `config_types.go`
  - `Config`
  - `ServerConfig`
  - `TLSConfig`
  - LLM / channel / runtime / audio / security / memory の設定型
- `config_defaults.go`
  - `setDefaults`
- `config_validation.go`
  - `Validate`
  - `validateCoderConfig`
- `config_local_llm.go`
  - `LocalLLMWarmupEnabled`
  - `shouldEnableLocalTLSSkipVerify`

### ToolRunner

対象:

- `internal/infrastructure/tools/runner.go`

分離単位:

- `runner.go`
  - `ToolRunner`
  - constructor
  - public execution entrypoint
- `runner_registration.go`
  - core / optional tool registration
  - metadata registration
  - subagent / registry tool registration
- `runner_wrappers.go`
  - v1 / v2 wrapper
  - v1 error classification
  - tool definition generation
- `runner_shell.go`
  - shell execution
  - shell command allow 判定
- `runner_file.go`
  - file read / write / list
  - dry-run
  - path allow 判定
  - integer argument helper
- `runner_web_search.go`
  - web search v1 / v2
  - Google search response type
  - search result formatting

## 対象外

次は Phase30 の対象外とする。

- config の YAML key 変更
- default 値変更
- validation 条件変更
- ToolRunner の public API 変更
- tool 名、metadata、引数契約の変更
- shell/file/web search の挙動変更
- security policy、audit、protected file contract の変更
- Chat / Worker / Coder の責務境界変更
- fallback を正常系として扱う変更

## 契約

### config

- 入力: config file path、environment variables
- 出力: `*Config`
- 副作用: config file read、environment override
- 永続化: なし
- ログ: 既存通り
- エラー契約: load / parse / validation error は既存通り返す
- 変更禁止: default 値、YAML tag、env override、coder validation

### ToolRunner

- 入力: `ToolRunnerConfig`、tool name、arguments
- 出力: tool 実行結果、`tool.ExecutionResult`
- 副作用: shell command、file read/write/list、web search、subagent/tool registry call
- 永続化: file write、web search cache、audit repository
- ログ: 既存通り
- エラー契約: unknown tool、validation error、security rejection、tool execution error を既存通り維持する
- 変更禁止: Chat / Worker の tool 境界、shell allow 判定、file write dry-run、Google search cache、tool metadata

## 実装手順

1. baseline test を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/config ./internal/infrastructure/tools`
2. `config.go` の型定義、defaults、validation、local LLM helper を責務別ファイルへ移す。
3. `runner.go` の registration、wrapper、shell、file、web search を責務別ファイルへ移す。
4. function signature、type name、YAML tag、tool name、metadata、error text は変更しない。
5. `gofmt` / `goimports` を実行する。
6. after test を実行する。
7. full test / E2E を実行する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/config ./internal/infrastructure/tools
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/config ./internal/infrastructure/tools
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./...
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
git diff --check
```

live health が使える場合:

```bash
curl -fsS http://127.0.0.1:18790/health
```

## TDD 方針

Phase30 は挙動変更を伴わない declaration move を主作業とするため、新しい仕様テストは追加しない。

代替 TDD として、baseline / after で既存の config test と ToolRunner test を実行し、default、validation、tool registration、shell/file/web search contract が維持されることを確認する。

もし移動だけではテストが維持できず、contract 変更が必要になった場合は Phase30 を停止し、別 Phase として仕様化する。

## リスク

- YAML tag を移動時に壊す。
- default 値や validation 条件を変える。
- ToolRunner の registration 順や metadata を変える。
- shell/file/web search の security contract を崩す。
- Chat / Worker / Coder の tool 実行境界を崩す。
- WebSearchCache と live web search の責務を混同する。

## 完了条件

- この文書が `docs/refactor/` に作成されている。
- config と ToolRunner の分離単位が明記されている。
- `internal/adapter/config/config.go` が入口と最小責務へ薄くなっている。
- `internal/infrastructure/tools/runner.go` が constructor / public entrypoint 中心へ薄くなっている。
- config の public type / YAML tag / default / validation が維持されている。
- ToolRunner の public API / tool name / metadata / security contract が維持されている。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/adapter/config ./internal/infrastructure/tools` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` が成功している。
- `git diff --check` が成功している。

## 停止条件

次の場合は作業を止め、状況と選択肢を報告する。

- config の default / validation / YAML contract 変更が必要になる。
- ToolRunner の tool contract や security contract 変更が必要になる。
- テスト失敗の原因が Phase30 内で安全に切り分けられない。
