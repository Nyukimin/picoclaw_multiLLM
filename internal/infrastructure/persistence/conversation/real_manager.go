package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/google/uuid"
)

// RealConversationManager は実ストアを統合した会話管理実装
type RealConversationManager struct {
	redisStore    redisStoreIface
	l1Store       l1StoreIface
	duckdbStore   duckdbStoreIface
	vectordbStore vectordbStoreIface
	embedder      domconv.EmbeddingProvider      // nilの場合はVectorDB機能無効
	summarizer    domconv.ConversationSummarizer // nilの場合は簡易実装
}

// NewRealConversationManager は新しいRealConversationManagerを生成
func NewRealConversationManager(redisURL, duckdbPath, vectordbURL string) (*RealConversationManager, error) {
	redisStore, err := NewRedisStore(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis store: %w", err)
	}

	duckdbStore, err := NewDuckDBStore(duckdbPath)
	if err != nil {
		redisStore.Close()
		return nil, fmt.Errorf("failed to create duckdb store: %w", err)
	}

	vectordbStore, err := NewVectorDBStore(vectordbURL, "picoclaw_memory")
	if err != nil {
		redisStore.Close()
		duckdbStore.Close()
		return nil, fmt.Errorf("failed to create vectordb store: %w", err)
	}

	return &RealConversationManager{
		redisStore:    redisStore,
		duckdbStore:   duckdbStore,
		vectordbStore: vectordbStore,
	}, nil
}

// WithEmbedder はEmbeddingProviderを注入する（チェーン可能）
func (r *RealConversationManager) WithEmbedder(e domconv.EmbeddingProvider) *RealConversationManager {
	r.embedder = e
	return r
}

// WithSummarizer はConversationSummarizerを注入する（チェーン可能）
func (r *RealConversationManager) WithSummarizer(s domconv.ConversationSummarizer) *RealConversationManager {
	r.summarizer = s
	return r
}

func (r *RealConversationManager) WithL1Store(store l1StoreIface) *RealConversationManager {
	if l1, ok := store.(*L1SQLiteStore); ok {
		if archiveStore, ok := r.duckdbStore.(L1ArchiveStore); ok {
			l1.WithArchiveStore(archiveStore)
		}
	}
	r.l1Store = store
	return r
}

// Close はすべてのストアを閉じる
func (r *RealConversationManager) Close() error {
	var errs []error
	if err := r.redisStore.Close(); err != nil {
		errs = append(errs, fmt.Errorf("redis close: %w", err))
	}
	if err := r.duckdbStore.Close(); err != nil {
		errs = append(errs, fmt.Errorf("duckdb close: %w", err))
	}
	if err := r.vectordbStore.Close(); err != nil {
		errs = append(errs, fmt.Errorf("vectordb close: %w", err))
	}
	if r.l1Store != nil {
		if err := r.l1Store.Close(); err != nil {
			errs = append(errs, fmt.Errorf("l1 sqlite close: %w", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing stores: %v", errs)
	}
	return nil
}

// Recall は会話記憶を3層から取得（短期→中期→長期）
func (r *RealConversationManager) Recall(ctx context.Context, sessionID string, query string, topK int) ([]domconv.Message, error) {
	// 1. 短期記憶（Redis: ActiveThread）
	thread, err := r.GetActiveThread(ctx, sessionID)
	if err == nil && thread != nil && len(thread.Turns) > 0 {
		return thread.Turns, nil
	}

	// 1.5. L1 hot store（SQLite: 再起動後の当日会話）
	if r.l1Store != nil {
		events, err := r.l1Store.RecentBySession(ctx, sessionID, topK*4)
		if err != nil {
			log.Printf("Recall: L1 SQLite search failed for session %q: %v", sessionID, err)
		} else if len(events) > 0 {
			messages := l1EventsToMessages(events)
			if len(messages) > 0 {
				return messages, nil
			}
		}
	}

	// 2. 中期記憶（DuckDB: Session履歴）
	summaries, err := r.duckdbStore.GetSessionHistory(ctx, sessionID, topK)
	if err == nil && len(summaries) > 0 {
		messages := make([]domconv.Message, 0, len(summaries))
		for _, s := range summaries {
			msg := domconv.NewMessage(
				domconv.SpeakerSystem,
				fmt.Sprintf("[Summary] %s (domain: %s)", s.Summary, s.Domain),
				map[string]interface{}{"thread_id": s.ThreadID, "keywords": s.Keywords},
			)
			messages = append(messages, msg)
		}
		return messages, nil
	}

	// 3. 長期記憶（VectorDB: 類似度検索）
	if r.embedder == nil {
		log.Printf("Recall: Embedder not configured, skipping long-term memory search")
		return []domconv.Message{}, nil
	}
	embedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		log.Printf("Recall: Failed to embed query %q: %v", query, err)
		return []domconv.Message{}, nil
	}

	// VectorDB検索をリトライ付きで実行
	var vdbResults []*domconv.ThreadSummary
	err = withRetry(ctx, DefaultRetryConfig, func() error {
		var searchErr error
		vdbResults, searchErr = r.vectordbStore.SearchSimilar(ctx, embedding, topK)
		return searchErr
	})
	if err != nil {
		log.Printf("Recall: VectorDB search failed after retries for query %q: %v", query, err)
		return []domconv.Message{}, nil
	}
	if len(vdbResults) == 0 {
		return []domconv.Message{}, nil
	}
	messages := make([]domconv.Message, 0, len(vdbResults))
	for _, s := range vdbResults {
		msg := domconv.NewMessage(
			domconv.SpeakerSystem,
			fmt.Sprintf("[LongTermMemory] %s (score: %.2f)", s.Summary, s.Score),
			map[string]interface{}{"thread_id": s.ThreadID, "score": s.Score},
		)
		messages = append(messages, msg)
	}
	return messages, nil
}

func l1EventsToMessages(events []L1MemoryEvent) []domconv.Message {
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
	})
	messages := make([]domconv.Message, 0, len(events))
	for _, ev := range events {
		if ev.Message == "" {
			continue
		}
		meta := map[string]interface{}{
			"namespace":    ev.Namespace,
			"thread_id":    ev.ThreadID,
			"memory_state": ev.MemoryState,
			"layer":        ev.Layer,
			"source":       ev.Source,
		}
		for k, v := range ev.Meta {
			if _, exists := meta[k]; !exists {
				meta[k] = v
			}
		}
		messages = append(messages, domconv.Message{
			Speaker:   ev.Speaker,
			Msg:       ev.Message,
			Timestamp: ev.CreatedAt,
			Meta:      meta,
		})
	}
	return messages
}

// Store はメッセージをActiveThreadに追加
func (r *RealConversationManager) Store(ctx context.Context, sessionID string, msg domconv.Message) error {
	thread, err := r.GetActiveThread(ctx, sessionID)
	if err == domconv.ErrThreadNotFound {
		thread, err = r.CreateThread(ctx, sessionID, "general")
		if err != nil {
			return fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get active thread: %w", err)
	}

	thread.AddMessage(msg)
	if r.l1Store != nil {
		namespace := fmt.Sprintf("conv:%d", thread.ID)
		if err := r.l1Store.SaveMessage(ctx, sessionID, thread.ID, namespace, msg, MemoryStateObserved); err != nil {
			log.Printf("Failed to save message to L1 SQLite: %v", err)
		}
	}

	if len(thread.Turns) >= 12 {
		summary, err := r.FlushThread(ctx, thread.ID)
		if err != nil {
			log.Printf("FlushThread failed: %v", err)
		} else {
			log.Printf("Thread #%d flushed: %s", thread.ID, summary.Summary)
		}
		newThread, err := r.CreateThread(ctx, sessionID, thread.Domain)
		if err != nil {
			return fmt.Errorf("failed to create new thread after flush: %w", err)
		}
		newThread.AddMessage(msg)
		thread = newThread
	}

	if err := r.redisStore.SaveThread(ctx, thread); err != nil {
		return fmt.Errorf("failed to save thread to redis: %w", err)
	}
	return nil
}

// FlushThread はThreadを要約してDuckDB/VectorDBに保存
func (r *RealConversationManager) FlushThread(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
	thread, err := r.redisStore.GetThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread from redis: %w", err)
	}

	summaryText, keywords := r.generateSummaryAndKeywords(ctx, thread)

	var embedding []float32
	if r.embedder != nil {
		emb, err := r.embedder.Embed(ctx, summaryText)
		if err != nil {
			log.Printf("Failed to generate embedding (skipping VectorDB): %v", err)
		} else {
			embedding = emb
		}
	}

	summary := &domconv.ThreadSummary{
		ThreadID:  thread.ID,
		SessionID: thread.SessionID,
		Domain:    thread.Domain,
		Summary:   summaryText,
		Keywords:  keywords,
		Embedding: embedding,
		StartTime: thread.StartTime,
		EndTime:   time.Now(),
		IsNovel:   false,
	}

	if err := r.duckdbStore.SaveThreadSummary(ctx, summary); err != nil {
		return nil, fmt.Errorf("failed to save summary to duckdb: %w", err)
	}

	if len(summary.Embedding) > 0 {
		if err := r.vectordbStore.SaveThreadSummary(ctx, summary); err != nil {
			log.Printf("Failed to save summary to vectordb: %v", err)
		}
	}

	if err := r.redisStore.DeleteThread(ctx, threadID); err != nil {
		log.Printf("Failed to delete thread from redis: %v", err)
	}
	return summary, nil
}

// IsNovelInformation は情報が新規かを判定
func (r *RealConversationManager) IsNovelInformation(ctx context.Context, msg domconv.Message) (bool, float32, error) {
	if r.embedder == nil {
		return false, 0.0, nil
	}
	embedding, err := r.embedder.Embed(ctx, msg.Msg)
	if err != nil {
		return false, 0.0, fmt.Errorf("failed to embed message: %w", err)
	}
	isNovel, score, err := r.vectordbStore.IsNovelQuery(ctx, embedding, noveltyThreshold)
	if err != nil {
		return false, 0.0, fmt.Errorf("failed to query vectordb: %w", err)
	}
	return isNovel, score, nil
}

// GetActiveThread は SessionID に紐づく ActiveThread を取得
func (r *RealConversationManager) GetActiveThread(ctx context.Context, sessionID string) (*domconv.Thread, error) {
	sess, err := r.redisStore.GetSession(ctx, sessionID)
	if err == domconv.ErrSessionNotFound {
		return nil, domconv.ErrThreadNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if sess.LastThreadID == 0 {
		return nil, domconv.ErrThreadNotFound
	}
	return r.redisStore.GetThread(ctx, sess.LastThreadID)
}

// CreateThread は新規 Thread を作成
func (r *RealConversationManager) CreateThread(ctx context.Context, sessionID string, domain string) (*domconv.Thread, error) {
	sess, err := r.redisStore.GetSession(ctx, sessionID)
	if err == domconv.ErrSessionNotFound {
		sess = domconv.NewSessionConversation(sessionID, "")
	} else if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	thread := domconv.NewThread(sessionID, domain)
	sess.LastThreadID = thread.ID
	sess.UpdatedAt = time.Now()

	if err := r.redisStore.SaveThread(ctx, thread); err != nil {
		return nil, fmt.Errorf("failed to save thread to redis: %w", err)
	}
	if err := r.redisStore.SaveSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to save session to redis: %w", err)
	}
	return thread, nil
}

// GetAgentStatus は Agent の状態を取得
func (r *RealConversationManager) GetAgentStatus(ctx context.Context, agentName string) (*domconv.AgentStatus, error) {
	return domconv.NewAgentStatus(agentName), nil
}

// UpdateAgentStatus は Agent の状態を更新
func (r *RealConversationManager) UpdateAgentStatus(_ context.Context, _ *domconv.AgentStatus) error {
	return nil
}

func (r *RealConversationManager) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]*domconv.ThreadSummary, error) {
	if r == nil || r.duckdbStore == nil {
		return []*domconv.ThreadSummary{}, nil
	}
	return r.duckdbStore.GetSessionHistory(ctx, sessionID, limit)
}

func (r *RealConversationManager) SearchByDomain(ctx context.Context, domain string, limit int) ([]*domconv.ThreadSummary, error) {
	if r == nil || r.duckdbStore == nil {
		return []*domconv.ThreadSummary{}, nil
	}
	return r.duckdbStore.SearchByDomain(ctx, domain, limit)
}

// --- 内部ヘルパー ---

func (r *RealConversationManager) generateSummaryAndKeywords(ctx context.Context, thread *domconv.Thread) (string, []string) {
	if r.summarizer != nil {
		summary, err := r.summarizer.Summarize(ctx, thread)
		if err != nil {
			log.Printf("Summarizer failed, falling back to simple: %v", err)
		} else {
			keywords, err := r.summarizer.ExtractKeywords(ctx, thread)
			if err != nil {
				log.Printf("ExtractKeywords failed, using domain: %v", err)
				keywords = []string{thread.Domain}
			}
			return summary, keywords
		}
	}
	return generateSimpleSummary(thread), []string{thread.Domain}
}

func generateSimpleSummary(thread *domconv.Thread) string {
	if len(thread.Turns) == 0 {
		return "Empty thread"
	}
	first := thread.Turns[0].Msg
	last := thread.Turns[len(thread.Turns)-1].Msg
	if len(first) > 50 {
		first = first[:50] + "..."
	}
	if len(last) > 50 {
		last = last[:50] + "..."
	}
	return fmt.Sprintf("Start: %s ... End: %s (%d turns)", first, last, len(thread.Turns))
}

// SaveWebSearchToKB はWeb検索結果をKnowledge Baseに保存
func (m *RealConversationManager) SaveWebSearchToKB(ctx context.Context, domain string, query string, results []WebSearchResult) error {
	if len(results) == 0 {
		return nil
	}

	if m.embedder == nil {
		log.Printf("SaveWebSearchToKB: Embedder not configured, skipping save (domain=%s, query=%q, %d results)", domain, query, len(results))
		return nil
	}

	successCount := 0
	var lastErr error

	// 各検索結果を Document に変換して保存
	for i, result := range results {
		// Content の Embedding 生成
		contentEmbedding, err := m.embedder.Embed(ctx, result.Title+" "+result.Snippet)
		if err != nil {
			log.Printf("SaveWebSearchToKB: Failed to embed result %d/%d (title=%q): %v", i+1, len(results), result.Title, err)
			lastErr = err
			continue
		}

		doc := &domconv.Document{
			ID:        uuid.New().String(),
			Domain:    domain,
			Content:   fmt.Sprintf("# %s\n\n%s\n\nSource: %s", result.Title, result.Snippet, result.Link),
			Source:    result.Link,
			Embedding: contentEmbedding,
			Meta: map[string]interface{}{
				"title":        result.Title,
				"snippet":      result.Snippet,
				"query":        query,
				"search_index": i,
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// VectorDB保存をリトライ付きで実行
		err = withRetry(ctx, DefaultRetryConfig, func() error {
			return m.vectordbStore.SaveKB(ctx, doc)
		})
		if err != nil {
			log.Printf("SaveWebSearchToKB: Failed to save result %d/%d to VectorDB after retries (title=%q): %v", i+1, len(results), result.Title, err)
			lastErr = err
			continue
		}

		successCount++
	}

	// 一部でも成功していれば成功とみなす
	if successCount > 0 {
		log.Printf("SaveWebSearchToKB: Saved %d/%d results (domain=%s, query=%q)", successCount, len(results), domain, query)
		return nil
	}

	// 全て失敗した場合はエラーを返す
	if lastErr != nil {
		return fmt.Errorf("failed to save all %d web search results to KB (domain=%s, query=%q): %w", len(results), domain, query, lastErr)
	}

	return nil
}

func (m *RealConversationManager) GetFreshWebSearchCache(ctx context.Context, query string) ([]WebSearchResult, bool, error) {
	if m.l1Store == nil {
		return nil, false, nil
	}
	entry, err := m.l1Store.GetFreshSearchCache(ctx, "web", query, time.Now().UTC())
	if err != nil {
		return nil, false, err
	}
	if entry == nil {
		return nil, false, nil
	}
	var results []WebSearchResult
	if err := json.Unmarshal([]byte(entry.ResultsJSON), &results); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal cached web search results: %w", err)
	}
	return results, true, nil
}

func (m *RealConversationManager) SaveWebSearchCache(ctx context.Context, query string, results []WebSearchResult, ttl time.Duration) error {
	if m.l1Store == nil || len(results) == 0 {
		return nil
	}
	resultsJSON, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to marshal web search cache results: %w", err)
	}
	sourceURLs := make([]string, 0, len(results))
	for _, result := range results {
		if result.Link != "" {
			sourceURLs = append(sourceURLs, result.Link)
		}
	}
	_, err = m.l1Store.SaveSearchCache(ctx, "web", query, string(resultsJSON), sourceURLs, ttl)
	return err
}

// WebSearchResult は agent.WebSearchResult のエイリアス（Phase 4.2）
type WebSearchResult = agent.WebSearchResult

// SearchKB はKnowledge Baseから関連ドキュメントを検索
func (m *RealConversationManager) SearchKB(ctx context.Context, domain string, query string, topK int) ([]*domconv.Document, error) {
	if m.embedder == nil {
		log.Printf("SearchKB: Embedder not configured, returning empty results (domain=%s, query=%q)", domain, query)
		return []*domconv.Document{}, nil
	}

	// Query の Embedding 生成
	queryEmbedding, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding for domain=%s, query=%q: %w", domain, query, err)
	}

	// VectorDB 検索をリトライ付きで実行
	var docs []*domconv.Document
	err = withRetry(ctx, DefaultRetryConfig, func() error {
		var searchErr error
		docs, searchErr = m.vectordbStore.SearchKB(ctx, domain, queryEmbedding, topK)
		return searchErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to search kb after retries (domain=%s, query=%q, topK=%d): %w", domain, query, topK, err)
	}

	return docs, nil
}

// --- KB管理API (kb-admin用) ---

// ListKBDocuments はKBコレクション内の全ドキュメントを取得
func (m *RealConversationManager) ListKBDocuments(ctx context.Context, domain string, limit int) ([]*domconv.Document, error) {
	return m.vectordbStore.ListKBDocuments(ctx, domain, limit)
}

// GetKBCollections は存在するKBコレクション一覧を取得
func (m *RealConversationManager) GetKBCollections(ctx context.Context) ([]string, error) {
	return m.vectordbStore.GetKBCollections(ctx)
}

// GetKBStats はKBコレクションの統計情報を取得
func (m *RealConversationManager) GetKBStats(ctx context.Context, domain string) (*KBStats, error) {
	return m.vectordbStore.GetKBStats(ctx, domain)
}

// DeleteOldKBDocuments は指定日時より古いKBドキュメントを削除
func (m *RealConversationManager) DeleteOldKBDocuments(ctx context.Context, domain string, before time.Time) (int, error) {
	return m.vectordbStore.DeleteOldKBDocuments(ctx, domain, before)
}
