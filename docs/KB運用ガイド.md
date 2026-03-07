# Knowledge Base (KB) 運用ガイド

**バージョン:** 1.0
**更新日:** 2026-03-07
**対象:** PicoClaw v5.0 Conversation System with KB

---

## 目次

1. [概要](#概要)
2. [アーキテクチャ](#アーキテクチャ)
3. [ドメイン設計](#ドメイン設計)
4. [KB保存（Web検索結果）](#kb保存web検索結果)
5. [KB検索（RAG）](#kb検索rag)
6. [管理・運用](#管理運用)
7. [パフォーマンスチューニング](#パフォーマンスチューニング)
8. [トラブルシューティング](#トラブルシューティング)

---

## 概要

### Knowledge Base (KB) とは

PicoClaw の **Knowledge Base (KB)** は、Web検索結果やその他の外部情報を永続化し、
後続の会話で再利用するための仕組みです。

**主な用途:**
- Worker の RESEARCH タスク結果の保存
- Chat の RAG (Retrieval-Augmented Generation) による情報補完
- ドメイン特化知識の蓄積（映画、技術、歴史等）

**技術スタック:**
- **VectorDB:** Qdrant (ベクトル検索エンジン)
- **Embedding:** 768次元ベクトル（Cohere/OpenAI互換）
- **コレクション構造:** ドメイン別 (`kb_{domain}`)

---

## アーキテクチャ

### データフロー

```
1. Web検索実行
   ├─ Google Custom Search API
   └─ 検索結果 (Title, Link, Snippet)
          ↓
2. KB保存
   ├─ Title + Snippet → Embedding生成
   ├─ Document 作成（Content, Source, Meta）
   └─ Qdrant へ保存 (kb_{domain})
          ↓
3. KB検索（RAG）
   ├─ ユーザークエリ → Embedding生成
   ├─ Qdrant ベクトル検索 (Top-K)
   └─ RecallPack の LongFacts に追加
          ↓
4. Chat応答生成
   └─ KB検索結果を文脈として利用
```

### レイヤー構成

| レイヤー | 役割 | 実装 |
|---------|------|------|
| **Domain** | KB Document 定義 | `internal/domain/conversation/document.go` |
| **Application** | - | - |
| **Infrastructure** | VectorDB 操作 | `internal/infrastructure/persistence/conversation/vectordb_store.go` |
| **Adapter** | ConversationManager | `internal/infrastructure/persistence/conversation/real_manager.go` |
| **Engine** | RAG統合 | `internal/infrastructure/persistence/conversation/engine_impl.go` |

---

## ドメイン設計

### ドメインとは

**ドメイン** は KB の論理的な分類単位です。Qdrant では `kb_{domain}` という名前のコレクションとして管理されます。

### 標準ドメイン

| ドメイン | 用途 | 例 |
|---------|------|-----|
| `general` | 汎用・一般知識 | ニュース、日常会話 |
| `programming` | プログラミング技術 | Go言語、アルゴリズム |
| `movie` | 映画情報 | おすすめ映画、レビュー |
| `anime` | アニメ情報 | 作品紹介、キャラクター |
| `tech` | 技術トレンド | AI動向、新技術 |
| `history` | 歴史・文化 | 歴史的事実、文化背景 |

### ドメイン選定ガイドライン

**原則:**
1. **粒度:** 広すぎず狭すぎず（検索精度とメンテナンス性のバランス）
2. **安定性:** 頻繁に変更しない（データ移行コストが高い）
3. **明確性:** 境界が曖昧なドメインは避ける

**良い例:**
- `programming` - 明確な技術分野
- `movie` - 明確なエンタメ分野

**悪い例:**
- `entertainment` - 広すぎる（movie/anime/game を分離すべき）
- `go_concurrency` - 狭すぎる（programming に統合すべき）

### カスタムドメイン追加

新しいドメインを追加する場合：

1. **ドメイン名を決定**（小文字英数字、アンダースコア可）
2. **既存ドメインと重複しないか確認**
3. **初回保存時に自動作成される**（手動作成不要）

```go
// 例: 料理レシピドメインを追加
domain := "recipe"
mgr.SaveWebSearchToKB(ctx, domain, "パスタ レシピ", results)
```

---

## KB保存（Web検索結果）

### API: SaveWebSearchToKB

**シグネチャ:**
```go
func (m *RealConversationManager) SaveWebSearchToKB(
    ctx context.Context,
    domain string,
    query string,
    results []WebSearchResult,
) error
```

**パラメータ:**
- `domain` - 保存先ドメイン（例: "programming"）
- `query` - 検索クエリ（メタ情報として保存）
- `results` - Web検索結果の配列

**WebSearchResult 構造:**
```go
type WebSearchResult struct {
    Title   string `json:"title"`   // ページタイトル
    Link    string `json:"link"`    // URL
    Snippet string `json:"snippet"` // 要約文
}
```

### 使用例

```go
// 1. Web検索を実行（Google Custom Search API等）
results := []WebSearchResult{
    {
        Title:   "Go言語の並行処理入門",
        Link:    "https://example.com/go-concurrency",
        Snippet: "ゴルーチンとチャネルの基本的な使い方を解説します。",
    },
    {
        Title:   "Go Concurrency Patterns",
        Link:    "https://example.com/go-patterns",
        Snippet: "実践的な並行処理パターンを紹介します。",
    },
}

// 2. KB に保存
err := mgr.SaveWebSearchToKB(ctx, "programming", "Go言語 並行処理", results)
if err != nil {
    log.Printf("Failed to save to KB: %v", err)
}
```

### 自動保存（Phase 4.2 実装予定）

**Worker RESEARCH ルート**で自動的にKB保存する仕組み：

```go
// Orchestrator レベルでフック
if route == routing.RouteRESEARCH {
    response, err := mio.Chat(ctx, task)
    if err == nil {
        // Web検索結果を抽出してKB保存
        if webResults := extractWebSearchResults(response); len(webResults) > 0 {
            mgr.SaveWebSearchToKB(ctx, task.Domain, task.Message, webResults)
        }
    }
}
```

**注意:** 現状は手動API呼び出しが必要です。

---

## KB検索（RAG）

### 自動統合

ConversationEngine の `BeginTurn()` で **自動的にKB検索が実行されます**。

**動作フロー:**
1. ユーザーメッセージを受信
2. 現在のドメインを取得（Thread.Domain）
3. ユーザーメッセージの Embedding を生成
4. Qdrant で類似度検索（Top-3）
5. 検索結果を `[KB]` プレフィックス付きで RecallPack.LongFacts に追加

**コード例（engine_impl.go）:**
```go
// Knowledge Base (KB) 検索（RAG統合）
if realMgr, ok := e.manager.(*RealConversationManager); ok {
    domain := "general"
    if thread, err := e.manager.GetActiveThread(ctx, sessionID); err == nil && thread != nil {
        domain = thread.Domain
    }

    kbDocs, err := realMgr.SearchKB(ctx, domain, userMessage, 3)
    if err != nil {
        log.Printf("[ConversationEngine] WARN: SearchKB failed: %v", err)
    } else if len(kbDocs) > 0 {
        for _, doc := range kbDocs {
            fact := "[KB] " + doc.Content
            pack.LongFacts = append(pack.LongFacts, fact)
        }
    }
}
```

### 手動検索（テスト・デバッグ用）

```go
// SearchKB を直接呼び出し
docs, err := mgr.SearchKB(ctx, "programming", "Go言語のエラーハンドリング", 5)
if err != nil {
    log.Fatalf("SearchKB failed: %v", err)
}

for i, doc := range docs {
    fmt.Printf("%d. [Score: %.4f] %s\n", i+1, doc.Score, doc.Source)
    fmt.Printf("   %s\n\n", doc.Content[:200])
}
```

### RecallPack での表示

KB検索結果は `RecallPack.LongFacts` に含まれます：

```
RecallPack {
    ShortContext: [...],  // 短期記憶
    MidSummaries: [...],  // 中期記憶（DuckDB）
    LongFacts: [
        "[KB] # Go言語の並行処理入門\n\nゴルーチンとチャネルの基本...\n\nSource: https://...",
        "[KB] # Go Concurrency Patterns\n\n実践的な並行処理パターン...\n\nSource: https://...",
    ],
}
```

---

## 管理・運用

### kb-admin CLI

KB の管理には `kb-admin` コマンドを使用します。

#### 1. KB検索テスト

```bash
kb-admin search programming "Go言語 並行処理"
```

**出力例:**
```
🔍 Searching KB in domain 'programming' for: Go言語 並行処理

Found 2 documents:

--- Document 1 ---
ID:     550e8400-e29b-41d4-a716-446655440000
Source: https://example.com/go-concurrency
Score:  0.8752
Created: 2026-03-07T10:30:00Z
Content:
# Go言語の並行処理入門

ゴルーチンとチャネルの基本的な使い方を解説します。
goroutine は軽量スレッドで、channel で通信します...
```

#### 2. 統計情報確認

```bash
kb-admin stats
```

**出力例:**
```
📊 Knowledge Base Statistics

Checking known domains:
  ✓ programming - has documents
  ✓ movie - has documents
  ✓ general - empty
  ✓ anime - empty
  ✓ tech - empty
  ✓ history - empty
```

#### 3. ドキュメント一覧（Phase 4.2）

```bash
kb-admin list programming
```

#### 4. 古いドキュメント削除（Phase 4.2）

```bash
kb-admin cleanup programming 30  # 30日より古いドキュメントを削除
```

### Qdrant Web UI

Qdrant の Web UI でも直接管理できます：

```bash
# Qdrant起動
docker run -p 6333:6333 -p 6334:6334 qdrant/qdrant

# Web UI アクセス
open http://localhost:6333/dashboard
```

**操作:**
- コレクション一覧表示: Collections → `kb_programming` 等
- ドキュメント検索: Search → Query入力
- 削除: Points → Filter → Delete

---

## パフォーマンスチューニング

### 1. Embedding キャッシュ

**問題:** 同一クエリで毎回 Embedding を生成するのは非効率

**対策:**
```go
type EmbeddingCache struct {
    cache map[string][]float32
    mu    sync.RWMutex
}

func (c *EmbeddingCache) GetOrGenerate(ctx context.Context, text string, embedder EmbeddingProvider) ([]float32, error) {
    c.mu.RLock()
    if cached, ok := c.cache[text]; ok {
        c.mu.RUnlock()
        return cached, nil
    }
    c.mu.RUnlock()

    embedding, err := embedder.Embed(ctx, text)
    if err != nil {
        return nil, err
    }

    c.mu.Lock()
    c.cache[text] = embedding
    c.mu.Unlock()

    return embedding, nil
}
```

### 2. バッチ保存

**問題:** Web検索結果を1件ずつ保存すると遅い

**対策:**
```go
// SaveWebSearchToKB 内部でバッチ Upsert
points := make([]*qdrant.PointStruct, 0, len(results))
for _, result := range results {
    // Point 作成
    points = append(points, point)
}

// 一括 Upsert
v.client.Upsert(ctx, &qdrant.UpsertPoints{
    CollectionName: collectionName,
    Points:         points,
    Wait:           &waitTrue,
})
```

### 3. Top-K 調整

**問題:** Top-K が大きすぎると遅い、小さすぎると情報不足

**推奨値:**
- **Chat RAG:** Top-3 〜 Top-5（文脈補完）
- **テスト検索:** Top-10（精度確認）
- **一覧表示:** Top-100（管理用）

### 4. Qdrant インデックスチューニング

**HNSW パラメータ:**
```yaml
# Qdrant 設定ファイル
collections:
  kb_programming:
    hnsw_config:
      m: 16               # グラフの接続数（デフォルト: 16）
      ef_construct: 100   # 構築時の探索幅（デフォルト: 100）
```

**推奨値:**
- 高速検索優先: `m: 8, ef_construct: 64`
- 精度優先: `m: 32, ef_construct: 200`
- バランス: `m: 16, ef_construct: 100`（デフォルト）

---

## トラブルシューティング

### Q1. KB検索結果が空になる

**原因:**
1. Embedder が未設定
2. ドメインが存在しない
3. 検索クエリの Embedding が失敗

**確認:**
```bash
# 1. Embedder 設定確認
kb-admin search programming "test"
# → "embedder not configured" エラーが出る場合は Embedder 未設定

# 2. ドメイン存在確認
kb-admin stats
# → 対象ドメインが "empty" の場合はドキュメント未保存

# 3. ログ確認
tail -f ~/.picoclaw/logs/picoclaw.log | grep "SearchKB"
```

### Q2. KB保存が失敗する

**原因:**
1. Qdrant が起動していない
2. Embedding 生成に失敗
3. コレクション作成エラー

**確認:**
```bash
# 1. Qdrant 起動確認
curl http://localhost:6333/
# → {"title":"qdrant - vector search engine",...} が返る

# 2. Embedding 確認
# ログで "failed to generate embedding" を検索

# 3. コレクション確認
curl http://localhost:6333/collections/kb_programming
```

### Q3. 検索精度が低い

**原因:**
1. Embedding モデルの品質
2. ドキュメント内容が不適切（Snippet が短すぎる等）
3. Top-K が小さすぎる

**対策:**
```go
// 1. Embedding モデルを変更
embedder := cohere.NewCohereProvider(apiKey, "embed-multilingual-v3.0")

// 2. Content を充実させる
doc.Content = fmt.Sprintf("# %s\n\n%s\n\n詳細: %s\n\nSource: %s",
    result.Title, result.Snippet, result.FullText, result.Link)

// 3. Top-K を増やす
docs, _ := mgr.SearchKB(ctx, domain, query, 10)  // 3 → 10
```

### Q4. メモリ使用量が多い

**原因:**
1. Embedding キャッシュが肥大化
2. VectorDB のメモリマップ

**対策:**
```go
// 1. キャッシュサイズ制限
type LRUEmbeddingCache struct {
    maxSize int
    // LRU実装
}

// 2. Qdrant のメモリ設定
# docker-compose.yml
services:
  qdrant:
    environment:
      - QDRANT__STORAGE__ON_DISK_PAYLOAD=true  # Payload をディスクに保存
```

---

## まとめ

### チェックリスト

**KB保存:**
- [ ] Web検索結果を `WebSearchResult` に変換
- [ ] 適切なドメインを選定
- [ ] `SaveWebSearchToKB()` を呼び出し
- [ ] エラーハンドリング（ログ記録）

**KB検索（RAG）:**
- [ ] ConversationEngine が Embedder を持っている
- [ ] Thread.Domain が適切に設定されている
- [ ] RecallPack.LongFacts に `[KB]` プレフィックスが含まれる

**運用:**
- [ ] `kb-admin stats` で定期的に確認
- [ ] Qdrant バックアップ（定期スナップショット）
- [ ] 古いドキュメント削除（Phase 4.2 後）

---

**次のステップ:** [実装仕様_会話LLM_v5.md](./実装仕様_会話LLM_v5.md) を参照
