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

## KB / Source

Web search result や外部ソースは、Source Registry / KB として保存される。

保存、stage、validate、promote は別フェーズである。正式な memory / knowledge として扱うには検証済み状態が必要である。

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
