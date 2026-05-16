# Phase31 IdleChat 補助モジュール分割実装仕様

## 目的

Phase31 は、Phase30 後に残っている IdleChat の大型補助モジュールを、仕様変更理由ごとに分割するための構造整理である。

対象は次の 4 ファイルとする。

- `internal/application/idlechat/orchestrator_sanitize.go`
- `internal/application/idlechat/topic_generator.go`
- `internal/application/idlechat/orchestrator_response_generation.go`
- `internal/application/idlechat/orchestrator_loop_detection.go`

目的は、IdleChat の raw response、view data、topic generation、loop detection、style retry の責務を混同しない構造へ近づけることである。挙動変更は行わず、top-level declaration の移動を主作業とする。

## 対象範囲

### response sanitize

対象:

- `internal/application/idlechat/orchestrator_sanitize.go`

分離単位:

- `orchestrator_sanitize.go`
  - public に近い sanitize entrypoint
- `orchestrator_sanitize_extract.go`
  - visible answer 抽出
  - final answer block 抽出
  - dialogue candidate 抽出
- `orchestrator_sanitize_leak.go`
  - prompt leak / reasoning leak / internal script 判定
- `orchestrator_sanitize_style.go`
  - awkward style 判定
  - style retry 判定
  - sentence end 判定
- `orchestrator_sanitize_similarity.go`
  - latest other / self との重複判定
  - longest common substring
  - phrase repetition
  - token split

### topic generator

対象:

- `internal/application/idlechat/topic_generator.go`

分離単位:

- `topic_generator.go`
  - strategy type と strategy selection
- `topic_generator_sources.go`
  - daily seed cache
  - Wikipedia / news headline fetch
- `topic_generator_prompts.go`
  - prompt footer
  - anchor selection
  - single / double / external prompt generation

### response generation

対象:

- `internal/application/idlechat/orchestrator_response_generation.go`

分離単位:

- `orchestrator_response_generation.go`
  - response generation entrypoint
  - raw response 付き generation
- `orchestrator_response_retry.go`
  - compact retry messages
  - unusable response 判定
  - truncation 判定
- `orchestrator_response_helpers.go`
  - first turn label
  - max tokens
  - fun score
  - topic text extraction
  - topic resolution
  - speaker temperature
- `orchestrator_response_llm.go`
  - IdleChat LLM 呼び出し
  - fallback response function

### loop detection

対象:

- `internal/application/idlechat/orchestrator_loop_detection.go`

分離単位:

- `orchestrator_loop_detection.go`
  - loop detection entrypoint 群
- `orchestrator_loop_patterns.go`
  - transcript speaker split
  - lead pattern
  - repeated lead pattern
  - speaker context split
- `orchestrator_loop_similarity.go`
  - topic similarity
  - response similarity
  - normalized loop text
  - n-gram similarity
- `orchestrator_loop_attribution.go`
  - latest other / self utterance
  - attribution violation 判定

## 対象外

次は Phase31 の対象外とする。

- IdleChat の raw / view / audio event 契約変更
- fallback を正常系として扱う変更
- topic generation prompt の意味変更
- sanitize 判定条件の変更
- loop detection threshold の変更
- LLM provider 呼び出し契約変更
- Viewer 表示、音声、口パク、ログの意味変更
- STT input を IdleChat に流す変更

## 契約

- 入力: IdleChat transcript、speaker、topic、LLM raw response、topic seed source
- 出力: sanitized response、retry 判定、topic prompt、loop 判定
- 副作用: topic source fetch、LLM generation、IdleChat event emission
- 永続化: topic generator cache、IdleChat history は既存通り
- ログ: 既存通り
- エラー契約: unusable response、generation error、fallback 関数の扱いを既存通り維持する
- 変更禁止: raw response と view data の境界、audio trigger の境界、fallback を成功扱いしない方針

## 実装手順

1. baseline test を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat`
2. 対象 4 ファイルの top-level declaration を責務別ファイルへ移す。
3. function signature、判定条件、prompt text、threshold、fallback 関数の戻り値は変更しない。
4. `gofmt` / `goimports` を実行する。
5. after test を実行する。
6. full test / E2E を実行する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat
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

Phase31 は挙動変更を伴わない declaration move を主作業とするため、新しい仕様テストは追加しない。

代替 TDD として、baseline / after で IdleChat 既存テストを実行し、sanitize、topic generation、response generation、loop detection の契約が維持されることを確認する。

もし移動だけではテストが維持できず、判定条件や契約変更が必要になった場合は Phase31 を停止し、別 Phase として仕様化する。

## リスク

- raw response と view data の境界を崩す。
- fallback 関数を正常系として扱ってしまう。
- topic prompt や banned keyword 条件を変える。
- loop detection の threshold や similarity 判定を変える。
- LLM generation error と sanitize rejection を混同する。

## 完了条件

- この文書が `docs/refactor/` に作成されている。
- 対象 4 ファイルの分離単位が明記されている。
- IdleChat 補助モジュールが責務別ファイルへ分割されている。
- raw response / view data / audio trigger の境界が維持されている。
- sanitize / topic / response / loop detection の判定条件が維持されている。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` が成功している。
- `git diff --check` が成功している。

## 停止条件

次の場合は作業を止め、状況と選択肢を報告する。

- IdleChat の raw / view / audio event 契約変更が必要になる。
- fallback、sanitize、loop detection の判定変更が必要になる。
- テスト失敗の原因が Phase31 内で安全に切り分けられない。
