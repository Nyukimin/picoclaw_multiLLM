# Phase33 TTS / LLM provider 分割実装仕様

## 目的

Phase33 は、Phase32 後に残っている TTS / LLM provider 周辺の大型 production file を、provider 本体、request/response、stream、helper、retry の責務へ分割するための構造整理である。

対象は次の 4 ファイルとする。

- `internal/infrastructure/tts/irodori_provider.go`
- `internal/infrastructure/tts/rencrow_tts_bridge.go`
- `internal/infrastructure/llm/providers/openai/provider.go`
- `internal/infrastructure/llm/providers/ollama/provider.go`

目的は、音声合成、provider API、LLM response parsing、streaming、retry/backoff の変更点を分け、Viewer 表示、音声、口パク、ログを混同しない構造へ近づけることである。挙動変更は行わず、top-level declaration の移動を主作業とする。

## 対象範囲

### Irodori provider

対象:

- `internal/infrastructure/tts/irodori_provider.go`

分離単位:

- `irodori_provider.go`
  - provider type
  - constructor
  - `Name`
  - `Synthesize`
- `irodori_urls.go`
  - synthesis / generation URL
  - loopback rewrite
  - simple audio URL parse
- `irodori_reference_audio.go`
  - reference audio file / URL
  - upload
  - uploaded audio metadata
- `irodori_audio_download.go`
  - generated audio download
  - audio URL parse
- `irodori_defaults.go`
  - defaults
  - voice / style resolution

### RenCrow TTS bridge

対象:

- `internal/infrastructure/tts/rencrow_tts_bridge.go`

分離単位:

- `rencrow_tts_bridge.go`
  - bridge type
  - constructor
  - session public API
- `rencrow_tts_session.go`
  - session get / create
  - media base URL
  - synthesis URL normalize
- `rencrow_tts_params.go`
  - provider params filter
  - speech speed / pitch
  - language / bool / numeric helpers
- `rencrow_tts_errors.go`
  - synthesis error parse
  - invalid request error
  - error code normalize
- `rencrow_tts_retry.go`
  - retry 判定
  - backoff
  - retry post
  - transport error 判定

### OpenAI provider

対象:

- `internal/infrastructure/llm/providers/openai/provider.go`

分離単位:

- `provider.go`
  - provider type
  - constructor
  - `Name`
  - `Generate`
  - `Chat`
- `stream.go`
  - chat completions stream read
- `messages.go`
  - message conversion
  - tool message conversion
- `thinking_bridge.go`
  - thinking bridge fields
  - reasoning sanitize / final answer extraction
- `response_parse.go`
  - response parsing

### Ollama provider

対象:

- `internal/infrastructure/llm/providers/ollama/provider.go`

分離単位:

- `provider.go`
  - provider type
  - constructor
  - `Generate`
  - `Name`
  - `Chat`
- `stream.go`
  - stream read
- `chat_types.go`
  - Ollama chat request / response / tool types
- `prompt.go`
  - prompt build
- `model_ready.go`
  - model ready preflight
  - ps response
  - warmup

## 対象外

次は Phase33 の対象外とする。

- TTS provider API contract 変更
- TTS request parameter 変更
- audio chunk / lipsync / Viewer 表示契約変更
- LLM provider request / response contract 変更
- OpenAI / Ollama model selection 変更
- retry 条件や timeout 変更
- fallback を正常系として扱う変更
- repo example と live runtime config の意味変更

## 契約

### TTS provider / bridge

- 入力: text、voice/style/provider params、session ID
- 出力: audio bytes、media URL、TTS event
- 副作用: external provider HTTP call、audio download、session queue
- 永続化: 既存通り
- ログ: 既存通り
- エラー契約: synthesis error、invalid request、retry exhausted を既存通り維持する
- 変更禁止: audio chunk を Viewer 表示本文の根拠にしないこと、TTS と lipsync の境界

### LLM provider

- 入力: prompt/messages/model/options
- 出力: generated text、tool call、stream response
- 副作用: provider HTTP call、model warmup/preflight
- 永続化: なし
- ログ: 既存通り
- エラー契約: stream error、parse error、model not ready、provider error を既存通り維持する
- 変更禁止: fallback を成功扱いしないこと、thinking bridge の sanitize 契約

## 実装手順

1. baseline test を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tts ./internal/infrastructure/llm/providers/openai ./internal/infrastructure/llm/providers/ollama`
2. 対象 4 ファイルの top-level declaration を責務別ファイルへ移す。
3. function signature、provider request/response、retry 条件、timeout、error text は変更しない。
4. `gofmt` / `goimports` を実行する。
5. after test を実行する。
6. full test / E2E を実行する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tts ./internal/infrastructure/llm/providers/openai ./internal/infrastructure/llm/providers/ollama
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tts ./internal/infrastructure/llm/providers/openai ./internal/infrastructure/llm/providers/ollama
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

Phase33 は挙動変更を伴わない declaration move を主作業とするため、新しい仕様テストは追加しない。

代替 TDD として、baseline / after で TTS provider / OpenAI provider / Ollama provider の既存テストを実行し、full test と E2E で runtime wiring 回帰がないことを確認する。

もし移動だけではテストが維持できず、provider contract、retry、timeout、response parsing の変更が必要になった場合は Phase33 を停止し、別 Phase として仕様化する。

## リスク

- TTS と Viewer 表示本文を混同する。
- 音声、口パク、ログの境界を崩す。
- provider params filtering を変える。
- OpenAI thinking bridge sanitize を変える。
- Ollama model preflight / warmup の条件を変える。
- fallback を正常系として扱う。

## 完了条件

- この文書が `docs/refactor/` に作成されている。
- 対象 4 ファイルの分離単位が明記されている。
- TTS / LLM provider が責務別ファイルへ分割されている。
- provider request/response、retry、timeout、error contract が維持されている。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/infrastructure/tts ./internal/infrastructure/llm/providers/openai ./internal/infrastructure/llm/providers/ollama` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` が成功している。
- `git diff --check` が成功している。

## 停止条件

次の場合は作業を止め、状況と選択肢を報告する。

- TTS / LLM provider contract 変更が必要になる。
- Viewer 表示、音声、口パク、ログの意味変更が必要になる。
- テスト失敗の原因が Phase33 内で安全に切り分けられない。
