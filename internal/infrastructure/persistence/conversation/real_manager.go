package conversation

import (
	"context"
	"fmt"
	"log"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// RealConversationManager は実ストアを統合した会話管理実装
type RealConversationManager struct {
	redisStore    redisStoreIface
	duckdbStore   duckdbStoreIface
	vectordbStore vectordbStoreIface
	embedder      domconv.EmbeddingProvider     // nilの場合はVectorDB機能無効
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
		return []domconv.Message{}, nil
	}
	embedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		log.Printf("Failed to embed query for recall: %v", err)
		return []domconv.Message{}, nil
	}
	vdbResults, err := r.vectordbStore.SearchSimilar(ctx, embedding, topK)
	if err != nil || len(vdbResults) == 0 {
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
