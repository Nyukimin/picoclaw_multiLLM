# Phase32 MioAgent / picoclaw-agent 分割実装仕様

## 目的

Phase32 は、Phase31 後に残っている domain agent と standalone worker process entrypoint の責務集中を分割するための構造整理である。

対象は次の 2 ファイルとする。

- `internal/domain/agent/mio.go`
- `cmd/picoclaw-agent/main.go`

目的は、Mio の会話判断、web search、persona edit、attribution guard と、`picoclaw-agent` の handler / provider / config / message loop を分け、仕様変更時に主担当ファイルを説明しやすくすることである。挙動変更は行わず、top-level declaration の移動を主作業とする。

## 対象範囲

### MioAgent

対象:

- `internal/domain/agent/mio.go`

分離単位:

- `mio.go`
  - Mio の主要 type
  - constructor
  - `DecideAction`
  - `Chat`
- `mio_options.go`
  - `WithKBManager`
  - `WithSearchCacheManager`
  - `WithPersonaEditor`
  - `WithRecentContextProvider`
  - `WithSystemPrompt`
- `mio_web_search.go`
  - web search execution
  - metadata conversion
  - result formatting
  - search query clean
  - web search need detection
- `mio_persona.go`
  - explicit command parse
  - persona edit intent detection
  - persona edit execution
- `mio_attribution.go`
  - attribution context
  - latest other message extraction
  - attribution violation guard
  - attribution text normalization
- `mio_helpers.go`
  - log truncation
  - domain inference
  - field helper

### picoclaw-agent

対象:

- `cmd/picoclaw-agent/main.go`

分離単位:

- `main.go`
  - `main`
  - package entrypoint
- `handler_worker.go`
  - worker handler
  - proposal / task execution
- `handler_coder.go`
  - coder handler
  - coder config extraction
- `runtime_provider.go`
  - provider creation
  - dotenv load
  - stdout protection
- `runtime_init.go`
  - handler initialization
  - worker / coder initialization
  - coder personality resolution
- `message_loop.go`
  - message loop

## 対象外

次は Phase32 の対象外とする。

- Mio の prompt / response 契約変更
- web search query 判定変更
- persona edit 仕様変更
- attribution guard の判定変更
- `picoclaw-agent` の message protocol 変更
- worker / coder の実行契約変更
- provider selection 変更
- environment variable の意味変更
- Chat / Worker / Coder の責務境界変更

## 契約

### MioAgent

- 入力: user message、conversation context、KB/search/persona dependencies
- 出力: routing decision、chat response
- 副作用: web search、search cache、persona edit
- 永続化: persona file、search cache は既存通り
- ログ: 既存通り
- エラー契約: web search / persona edit の error handling を既存通り維持する
- 変更禁止: Chat が破壊的操作を抱え込まない境界、attribution guard、web search cache 契約

### picoclaw-agent

- 入力: stdin message、environment variables、config
- 出力: stdout response
- 副作用: worker execution、coder generation、provider call
- 永続化: worker の既存処理に従う
- ログ: stdout protocol を壊さない
- エラー契約: handler init / provider creation / message loop error を既存通り維持する
- 変更禁止: message protocol、shutdown timeout、stdout protection、worker/coder handler 境界

## 実装手順

1. baseline test を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/agent ./cmd/picoclaw-agent`
2. `mio.go` の options、web search、persona、attribution、helper を責務別ファイルへ移す。
3. `cmd/picoclaw-agent/main.go` の worker handler、coder handler、provider/runtime init、message loop を責務別ファイルへ移す。
4. function signature、protocol、env var、prompt、判定条件は変更しない。
5. `gofmt` / `goimports` を実行する。
6. after test を実行する。
7. full test / E2E を実行する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/agent ./cmd/picoclaw-agent
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/agent ./cmd/picoclaw-agent
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

Phase32 は挙動変更を伴わない declaration move を主作業とするため、新しい仕様テストは追加しない。

代替 TDD として、baseline / after で MioAgent test と `picoclaw-agent` package compile を確認し、full test と E2E で protocol 回帰がないことを確認する。

もし移動だけではテストが維持できず、prompt、protocol、handler 契約変更が必要になった場合は Phase32 を停止し、別 Phase として仕様化する。

## リスク

- Chat / Worker / Coder の責務境界を崩す。
- Mio に worker/coder 的な破壊的操作を混ぜる。
- web search cache と live search の責務を混同する。
- persona edit intent と通常 chat を混同する。
- stdout protocol をログ出力で壊す。
- coder provider config の意味を変える。

## 完了条件

- この文書が `docs/refactor/` に作成されている。
- MioAgent と `picoclaw-agent` の分離単位が明記されている。
- `mio.go` が Mio の中心 type / constructor / public behavior 中心へ薄くなっている。
- `cmd/picoclaw-agent/main.go` が entrypoint 中心へ薄くなっている。
- prompt、protocol、env var、判定条件が維持されている。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/agent ./cmd/picoclaw-agent` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` が成功している。
- `git diff --check` が成功している。

## 停止条件

次の場合は作業を止め、状況と選択肢を報告する。

- Mio prompt / response / attribution 契約変更が必要になる。
- `picoclaw-agent` の stdin/stdout protocol 変更が必要になる。
- テスト失敗の原因が Phase32 内で安全に切り分けられない。
