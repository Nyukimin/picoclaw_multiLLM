# Phase29 残存大型モジュール分割実装仕様

## 目的

Phase29 は、Phase28 後にも残っている大型ファイルを、仕様変更理由ごとに説明できる単位へ分割するための構造整理である。

目的は、単に行数を減らすことではない。仕様変更時に主に触るファイルを 1 対 1 に近づけ、モジュール化と疎結合を維持することである。

この Phase では、handler 本体、DTO、SSE event、IdleChat 契約、Viewer 表示契約、STT/TTS provider 挙動、LLM provider 挙動、persistence 契約は変更しない。既存の top-level declaration を責務別ファイルへ移すことを主な作業とする。

## 対象範囲

Phase29 の対象は、Phase28 後の実ファイル確認で、複数責務が 1 ファイルに残っている次の production file とする。

- `internal/application/idlechat/forecast_session.go`
- `internal/adapter/viewer/monitor.go`
- `internal/infrastructure/persistence/conversation/vectordb_store.go`
- `internal/infrastructure/persistence/conversation/real_manager.go`
- `internal/infrastructure/persistence/conversation/duckdb_store.go`

## 対象外

次は Phase29 の対象外とする。

- archive 配下の分割
- test file の大規模分割
- public interface の変更
- handler の request / response 契約変更
- persistence schema 変更
- DB path、collection 名、table 名の変更
- IdleChat raw / view / audio event 契約の変更
- Viewer 表示、音声、口パク、ログの意味変更
- fallback を正常系として扱う変更
- repo example と live runtime config の意味変更

## 分割判断

分割対象は、行数だけではなく、次の観点で選ぶ。

- 1 ファイルに複数の仕様変更理由がある。
- 仕様変更時に主担当ファイルを説明しにくい。
- store lifecycle、query、archive、KB、trend fetch、session runner などの責務が混在している。
- handler / monitor / persistence の契約が混ざると、検証対象を誤りやすい。

## 提案する分離単位

### IdleChat forecast session

対象: `internal/application/idlechat/forecast_session.go`

分割後の候補:

- `forecast_session.go`
  - forecast session の入口と最小限の共通定義を残す。
- `forecast_topic_stock.go`
  - `PreparedTopic`
  - `forecastTopicStock`
  - stock file load / save / pop / push / fill 状態管理
  - `InitForecastTopicStock`
  - `popForecastTopic`
  - `refillTopicStockAsync`
- `forecast_topic_generation.go`
  - inline topic generation
  - prompt construction
  - LLM topic normalization
  - keyword extraction
- `forecast_session_runner.go`
  - `RunForecastSession`
  - domain session loop
  - session domain selection
- `forecast_summary.go`
  - forecast summary 保存
  - summary LLM 呼び出し
  - covered theme extraction
  - session context update
- `forecast_trend_sources.go`
  - Google News / Google Trends / Reddit / Hatena source fetch
  - trend cache
  - seed ranking

変更してはいけないこと:

- forecast session の turn 数、domain 選択、topic stock path、event emit の意味を変えない。
- raw response、view data、audio trigger を混同しない。
- topic 生成失敗を正常系 fallback として扱わない。

### Viewer monitor store

対象: `internal/adapter/viewer/monitor.go`

分割後の候補:

- `monitor.go`
  - `MonitorStore` の中心構造と constructor を残す。
- `monitor_types.go`
  - snapshot / filter / detail / summary 型
- `monitor_events.go`
  - `OnEvent`
  - status / agent update の状態反映
- `monitor_queries.go`
  - `Status`
  - `Agents`
  - `AgentDetail`
  - `Jobs`
  - `Logs`
  - `ArchivedLogs`
  - `JobDetail`
  - `Summary`
- `monitor_reducers.go`
  - agent / job reducer
  - failure 判定
  - patch application
- `monitor_helpers.go`
  - phase classification
  - role helper
  - short text helper
  - `JobDetail.MarshalJSON`

変更してはいけないこと:

- Viewer monitor API の JSON 形状を変えない。
- event log、monitor snapshot、archive log の意味を混同しない。
- job failure 判定を成功系へ寄せない。

### VectorDB store

対象: `internal/infrastructure/persistence/conversation/vectordb_store.go`

分割後の候補:

- `vectordb_store.go`
  - `VectorDBStore`
  - constructor / close
  - base collection 初期化
- `vectordb_thread_memory.go`
  - thread summary 保存
  - similar search
  - domain search
  - novelty check
  - thread summary mapping
- `vectordb_kb.go`
  - KB collection 初期化
  - KB 保存 / 検索 / list / stats / delete
  - document mapping

変更してはいけないこと:

- Qdrant collection 名、vector dimension、score threshold を変えない。
- thread memory と KB document の mapping を混同しない。
- novelty 判定を fallback 成功扱いにしない。

### RealConversationManager

対象: `internal/infrastructure/persistence/conversation/real_manager.go`

分割後の候補:

- `real_manager.go`
  - `RealConversationManager`
  - constructor / options / close
- `real_manager_recall.go`
  - recall flow
  - L1 event から message への変換
- `real_manager_threads.go`
  - store
  - flush thread
  - active thread
  - summary / keyword generation
- `real_manager_agent_status.go`
  - agent status get / update
  - KPI copy
- `real_manager_archive.go`
  - session history
  - domain search
  - knowledge archive FTS
  - parquet export
- `real_manager_kb.go`
  - web search result 保存
  - L1 knowledge item 保存
  - web search cache
  - KB search / list / stats / delete

変更してはいけないこと:

- recall、store、archive、KB の保存先とエラー契約を変えない。
- observed / candidate / validated / promoted の意味を混同しない。
- Source Registry を無審査で正式 memory へ昇格しない。

### DuckDB store

対象: `internal/infrastructure/persistence/conversation/duckdb_store.go`

分割後の候補:

- `duckdb_store.go`
  - `DuckDBStore`
  - schema constant
  - constructor / close / table init
- `duckdb_l1_archive.go`
  - L1 memory / news / knowledge / staging archive
  - archive JSON marshal
  - knowledge archive FTS
- `duckdb_threads.go`
  - thread summary 保存
  - session history
  - domain search
- `duckdb_export.go`
  - parquet export
  - cleanup

変更してはいけないこと:

- DuckDB schema、table 名、column 名を変えない。
- archive、thread summary、parquet export の責務を混同しない。
- cleanup 条件を変えない。

## 実装手順

1. baseline test を実行する。
   - `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw`
   - `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat ./internal/adapter/viewer ./internal/infrastructure/persistence/conversation`
2. top-level declaration を責務別ファイルへ移す。
3. public interface、function signature、route、schema、DB path、collection 名、table 名は変更しない。
4. 移動先ファイルの import は `goimports` で最小化する。
5. `gofmt` / `goimports` を実行する。
6. after test を実行する。
7. 残存大型 production file を再確認し、次 Phase の対象が必要か判断する。

## 検証手順

baseline:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat ./internal/adapter/viewer ./internal/infrastructure/persistence/conversation
```

after:

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw
GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat ./internal/adapter/viewer ./internal/infrastructure/persistence/conversation
GOCACHE=/tmp/picoclaw-gocache go test ./...
GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e
git diff --check
git diff --stat
```

live health が使える場合:

```bash
curl -fsS http://127.0.0.1:18790/health
```

Phase29 は handler 本体や live runtime config を変更しないため、live/browser E2E は原則として回帰確認の補助扱いにする。ただし Viewer monitor API の JSON 契約に触れた場合は、Viewer session または同等の E2E で確認する。

## TDD 方針

Phase29 は挙動変更を伴わない top-level declaration 移動を主作業とするため、新しい仕様テストは追加しない。

代替 TDD として、分割前後で既存テストを baseline / after の両方で実行し、次を検証する。

- IdleChat forecast session の compile / unit test が維持される。
- Viewer monitor の API contract test が維持される。
- conversation persistence の unit test が維持される。
- full `go test ./...` と Phase25 E2E が通る。

もし移動だけでは compile が維持できず、signature や contract の変更が必要になった場合は Phase29 を停止し、別 Phase として仕様化する。

## リスク

- top-level declaration の移動漏れで compile が壊れる。
- 同名 helper を別ファイルへ移した結果、import や build tag の前提が崩れる。
- Viewer monitor の JSON contract を誤って変える。
- VectorDB の thread memory と KB mapping を混同する。
- RealConversationManager の recall / archive / KB の責務境界を崩す。
- DuckDB schema と archive query を誤って変更する。
- 大型ファイルを単に別の大型ファイルへ移すだけになる。

## 完了条件

- この文書が `docs/refactor/` に作成されている。
- 対象 5 ファイルの分割単位が明記されている。
- 実装前 baseline と実装後 after の検証手順が明記されている。
- 対象 production file が責務別ファイルへ分割されている。
- public interface、handler contract、DB schema、runtime config の意味が変わっていない。
- `GOCACHE=/tmp/picoclaw-gocache go test ./cmd/picoclaw` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./internal/application/idlechat ./internal/adapter/viewer ./internal/infrastructure/persistence/conversation` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test ./...` が成功している。
- `GOCACHE=/tmp/picoclaw-gocache go test -count=1 -tags=e2e ./test/e2e` が成功している。
- `git diff --check` が成功している。
- 残存大型 production file を再確認し、次 Phase の対象が必要か判断されている。

## 停止条件

次の場合は作業を止め、状況と選択肢を報告する。

- top-level declaration の移動だけでは compile を維持できない。
- public interface、handler contract、DB schema、runtime config の意味変更が必要になる。
- Viewer / IdleChat / STT / TTS / LLM provider の挙動変更が必要になる。
- テスト失敗の原因が Phase29 内で安全に切り分けられない。
