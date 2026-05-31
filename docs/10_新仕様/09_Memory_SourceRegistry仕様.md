# Memory / Source Registry 仕様

## 目的

Memory / Source Registry は、会話記憶、外部ソース、知識、検証済み情報を扱う永続化境界である。

記憶は無審査で正式化しない。observed、candidate、validated、promoted の状態を分ける。

## 構成

| 領域 | 役割 | 主担当 |
| --- | --- | --- |
| conversation memory | 会話履歴、summary、RecallPack | `internal/domain/conversation`, `internal/infrastructure/persistence/conversation` |
| L1SQLite | event、staging、source registry、news、knowledge、search cache | `internal/infrastructure/persistence/conversation/l1_sqlite_*.go` |
| VectorDB | thread memory / KB vector search | `internal/infrastructure/persistence/conversation/vectordb_*.go` |
| DuckDB | archive、thread summary、parquet export | `internal/infrastructure/persistence/conversation/duckdb_*.go` |
| RealConversationManager | recall、thread、archive、KB の統合 facade | `internal/infrastructure/persistence/conversation/real_manager_*.go` |
| Source Registry | 外部ソース登録、sweep、stage、validate、promote | `internal/application/sourcefetcher`, `internal/adapter/viewer/source_registry_handler.go` |
| Web Gather | 外部 API key に依存しない Web 検索候補取得、fetch、本文抽出、staging | `46_Web情報収集ツール仕様.md`, `modules/webgather`, `internal/application/webgather` |
| OperationMemory | runtime-readable な運用記憶、日次ノート | `operation_memory_dir`（既定: `~/.picoclaw/rencrow/memory`）, `internal/infrastructure/persistence/memory` |
| session repository | session state と distributed session の永続化 | `internal/domain/session`, `internal/infrastructure/persistence/session`, `cmd/picoclaw/runtime_sessions.go` |
| Glossary / RSS | RSS/Atom 由来の topic / glossary 文脈 | `internal/glossary`, `cmd/glossary` |
| Knowledge CLI / core importer | KB 初期投入、語彙更新、運用 CLI | `cmd/kb-admin`, `cmd/vocabulary`, `cmd/picoclaw/cli_knowledge.go`, `internal/application/knowledge` |

## 状態遷移

```text
observed
  -> candidate / staging
  -> validated or rejected
  -> promoted to memory / news / knowledge
```

禁止:

- Source Registry を無審査で正式 memory へ昇格する。
- observed / candidate / validated / promoted を同じ状態として扱う。
- Viewer 表示 state と永続化 state を混同する。

## RecallPack

RecallPack はプロンプト注入用の文脈である。

- role に応じて KB / search cache / thread summary を選別する。
- token budget を守る。
- rejected trace を残せるようにする。
- prompt text だけを真実の保存先にしない。
- L0/L1/L2/L3 の layer、score、採用理由、prompt 位置、採用/不採用 decision を trace できるようにする。

Recall budget は context の一部に収める。現行実装は `ApplyRecallBudget()` と `RecallBudgetRatio` を持ち、精密 tokenizer に差し替えられる `TokenEstimator` 入口を持つ。

role-filtered retrieval は Chat / Worker / Wild で retrieval 候補を変える。Chat は会話記憶中心、Worker は KB/search 込み、Wild は記憶と KB を中心に扱う。

Agent KPI / Level は AgentStatus として runtime state 側に保持する。現行実装は `internal/domain/conversation/agent_status.go` と `internal/infrastructure/persistence/conversation/real_manager_agent_status.go` で KPI 加算、Level 更新、RealConversationManager での保持を扱う。Viewer 表示や運用 UI は未接続のため、実装済み core と未接続 UI を分けて追跡する。

## OperationMemory

OperationMemory は repo の `workspace/` ではなく、DB や runtime state と同じ永続領域に置く。
既定の保存先は `~/.picoclaw/rencrow/memory/` で、設定 `operation_memory_dir` で上書きできる。

- 長期記憶は `MEMORY.md` に保存する。
- 日次ノートは `YYYYMM/YYYYMMDD.md` に保存する。
- キャラクター設定やスキル定義を置く `workspace/` と混同しない。

## session repository

session repository は session state を保存する境界であり、RecallPack や OperationMemory と同じ memory 周辺にあるが責務は異なる。

- `internal/domain/session` は session / distributed session の domain contract を持つ。
- `internal/infrastructure/persistence/session` は JSON repository などの永続化を担当する。
- `cmd/picoclaw/runtime_sessions.go` は session repository と OperationMemory の runtime wiring を担当する。
- session_id は発話、応答、chunk、job_id と混同しない。

## KB / Source

Web search result や外部ソースは、Source Registry / KB として保存される。

保存、stage、validate、promote は別フェーズである。正式な memory / knowledge として扱うには検証済み状態が必要である。

RenCrow の常用 Web 情報収集は `46_Web情報収集ツール仕様.md` を正とする。検索候補取得、URL fetch、本文抽出、staging を分離し、外部 API key を前提にしない。取得結果は pending staging として扱い、validate / promote を通さず正式 memory / knowledge にしてはいけない。

`cmd/kb-admin`、`cmd/vocabulary`、`cmd/picoclaw/cli_knowledge.go` は Knowledge DB の初期投入・確認・運用補助である。CLI から投入する場合も、未検証外部データを直接 confirmed memory として扱わない。

## 外部入力 risk metadata

外部ソース、添付、channel message は、本文や memory と混ぜずに risk metadata を持つ。

現行実装では `internal/domain/security.DetectPromptInjectionWarnings` が代表的な prompt injection pattern を検出し、attachment 抽出文の `SecurityWarnings` に保存する。Source Registry fetch 由来テキストにも同じ検出器を適用し、`L1SourceFetchPayload.Meta` / `L1StagingItem.Meta` の `security_warnings` と `security_warning_source: source_registry` に保存する。これは拒否判定そのものではなく、外部入力を扱う downstream が警告として参照するための metadata である。Viewer は Source Registry run 結果と staging review table で warning 件数を表示し、本文 / memory / prompt と混同しない。

検出対象の例:

- previous instruction の無視要求。
- system prompt の開示要求。
- tool / shell / command 実行の誘導。

warning 付き外部入力を、検証済み memory や prompt 方針として昇格してはいけない。

Viewer / API は Source Registry staging を次の順に扱う。

1. `GET /viewer/source-registry?action=staging&status=pending` で candidate を確認する。
2. `POST /viewer/source-registry?action=validate` で `ValidateStagingItem` を実行する。
3. `POST /viewer/source-registry?action=promote` で validated staging のみを news / knowledge / memory へ昇格する。

pending のまま promote した場合は失敗であり、fallback 成功扱いにしない。

## L1 SQLite

L1SQLite は hot store として次を扱う。

- memory event: `observed` / `candidate` / `confirmed` の状態を持つ会話・記憶 event。
- event log: message saved、search cache、state update、promotion、recall trace などの追跡 event。
- search cache: query 正規化、hash、TTL、source URL、fresh hit、類似 query hit、manual invalidate。
- staging: external fetch、memory candidate、search result を raw_text / summary_draft / raw_hash / validation_status つきで保持する。
- source registry: source_id、URL、kind、trust score、fetch interval、license note、enabled を持つ。
- news: validated staging 由来の news item、category 別 recent、source metadata。
- daily digest: day / morning / noon / evening slot の digest。
- knowledge: `kb:<domain>` の汎用 knowledge item、lexical 検索入口、Qdrant KB 同期。

## Archive

DuckDB は thread summary と L1 memory / news / knowledge / staging archive を扱う。
Parquet export は cold archive として使い、保存時または promotion 時に archive table へ同期する。

## Viewer との境界

Viewer memory / source registry panel は永続化状態の投影である。

Viewer で見えることは重要な観測だが、表示状態を直接正式 memory state と混同しない。

## 実装箇所

| 仕様 | 主担当 |
| --- | --- |
| memory domain | `internal/domain/memory`, `internal/domain/conversation` |
| L1SQLite schema / state | `internal/infrastructure/persistence/conversation/l1_sqlite_*.go` |
| staging validation | `internal/infrastructure/persistence/conversation/l1_sqlite_staging_validation.go` |
| promotion | `internal/infrastructure/persistence/conversation/l1_sqlite_promotions.go` |
| VectorDB thread / KB | `internal/infrastructure/persistence/conversation/vectordb_thread_memory.go`, `vectordb_kb.go` |
| DuckDB archive / export | `internal/infrastructure/persistence/conversation/duckdb_*.go` |
| source sweep | `internal/application/sourcefetcher/registry_sweeper.go` |
| search cache | `internal/infrastructure/persistence/conversation/l1_sqlite_search_cache.go` |
| event log | `internal/infrastructure/persistence/conversation/l1_sqlite_events.go` |
| news / digest | `internal/infrastructure/persistence/conversation/l1_sqlite_news_digest.go` |
| knowledge DB | `internal/infrastructure/persistence/conversation/l1_sqlite_knowledge.go` |
| knowledge CLI / importer | `cmd/kb-admin`, `cmd/vocabulary`, `cmd/picoclaw/cli_knowledge.go`, `internal/application/knowledge` |
| session repository | `internal/domain/session`, `internal/infrastructure/persistence/session`, `cmd/picoclaw/runtime_sessions.go` |
| archive job | `internal/application/archive`, `internal/infrastructure/persistence/conversation/duckdb_export.go` |
| Viewer source API | `internal/adapter/viewer/source_registry_handler.go` |
| Viewer memory API | `internal/adapter/viewer/memory_*_handler.go` |

## 検証

```bash
GOCACHE=/tmp/picoclaw-gocache go test ./internal/domain/conversation ./internal/domain/memory ./internal/infrastructure/persistence/conversation ./internal/application/sourcefetcher ./internal/adapter/viewer
```

確認対象:

- state transition が飛ばされない。
- rejected / validated / promoted が区別される。
- duplicate raw hash が扱える。
- Viewer 表示と永続化 state が整合する。
- Source Registry の無審査 promote が起きない。
