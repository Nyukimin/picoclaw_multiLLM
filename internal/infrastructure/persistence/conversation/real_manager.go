package conversation

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// RealConversationManager は実ストアを統合した会話管理実装
type RealConversationManager struct {
	redisStore    *RedisStore
	duckdbStore   *DuckDBStore
	vectordbStore *VectorDBStore
}

// NewRealConversationManager は新しいRealConversationManagerを生成
func NewRealConversationManager(redisURL, duckdbPath, vectordbURL string) (*RealConversationManager, error) {
	// RedisStore初期化
	redisStore, err := NewRedisStore(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis store: %w", err)
	}

	// DuckDBStore初期化
	duckdbStore, err := NewDuckDBStore(duckdbPath)
	if err != nil {
		redisStore.Close()
		return nil, fmt.Errorf("failed to create duckdb store: %w", err)
	}

	// VectorDBStore初期化
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
func (r *RealConversationManager) Recall(ctx context.Context, sessionID string, query string, topK int) ([]conversation.Message, error) {
	// 1. 短期記憶（Redis: ActiveThread）
	thread, err := r.GetActiveThread(ctx, sessionID)
	if err == nil && thread != nil {
		// ActiveThreadのターンをそのまま返す（最新の会話）
		if len(thread.Turns) > 0 {
			return thread.Turns, nil
		}
	}

	// 2. 中期記憶（DuckDB: Session履歴）
	summaries, err := r.duckdbStore.GetSessionHistory(ctx, sessionID, topK)
	if err == nil && len(summaries) > 0 {
		// ThreadSummaryからメッセージ風に復元
		messages := make([]conversation.Message, 0, len(summaries))
		for _, s := range summaries {
			msg := conversation.NewMessage(
				conversation.SpeakerSystem,
				fmt.Sprintf("[Summary] %s (domain: %s)", s.Summary, s.Domain),
				map[string]interface{}{
					"thread_id": s.ThreadID,
					"keywords":  s.Keywords,
				},
			)
			messages = append(messages, msg)
		}
		return messages, nil
	}

	// 3. 長期記憶（VectorDB: 類似度検索）
	// TODO: queryをembeddingに変換する必要がある（Phase 3で実装）
	// 現状は空の結果を返す
	log.Printf("VectorDB recall not implemented yet (query: %s)", query)

	return []conversation.Message{}, nil
}

// Store はメッセージをActiveThreadに追加
func (r *RealConversationManager) Store(ctx context.Context, sessionID string, msg conversation.Message) error {
	// ActiveThread取得または作成
	thread, err := r.GetActiveThread(ctx, sessionID)
	if err == conversation.ErrThreadNotFound {
		// 新規Thread作成（domainは初期値"general"）
		thread, err = r.CreateThread(ctx, sessionID, "general")
		if err != nil {
			return fmt.Errorf("failed to create thread: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get active thread: %w", err)
	}

	// メッセージ追加
	thread.AddMessage(msg)

	// Threadが満杯（12ターン）かチェック
	if len(thread.Turns) >= 12 {
		// Threadが満杯 → Flush
		summary, err := r.FlushThread(ctx, thread.ID)
		if err != nil {
			log.Printf("FlushThread failed: %v", err)
		} else {
			log.Printf("Thread #%d flushed: %s", thread.ID, summary.Summary)
		}

		// 新規Thread作成
		newThread, err := r.CreateThread(ctx, sessionID, thread.Domain)
		if err != nil {
			return fmt.Errorf("failed to create new thread after flush: %w", err)
		}
		// 新Threadに現在のメッセージを追加
		newThread.AddMessage(msg)
		thread = newThread
	}

	// Redis保存
	if err := r.redisStore.SaveThread(ctx, thread); err != nil {
		return fmt.Errorf("failed to save thread to redis: %w", err)
	}

	return nil
}

// FlushThread はThreadを要約してDuckDB/VectorDBに保存
func (r *RealConversationManager) FlushThread(ctx context.Context, threadID int64) (*conversation.ThreadSummary, error) {
	// Redis から Thread 取得
	thread, err := r.redisStore.GetThread(ctx, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get thread from redis: %w", err)
	}

	// Thread を要約（Phase 3 で LLM 統合、現状は簡易実装）
	summary := &conversation.ThreadSummary{
		ThreadID:  thread.ID,
		SessionID: thread.SessionID,
		Domain:    thread.Domain,
		Summary:   generateSimpleSummary(thread),
		Keywords:  extractSimpleKeywords(thread),
		Embedding: []float32{}, // Phase 3 で LLM 生成
		StartTime: thread.StartTime,
		EndTime:   time.Now(),
		IsNovel:   false, // Phase 3 で判定
	}

	// DuckDB 保存
	if err := r.duckdbStore.SaveThreadSummary(ctx, summary); err != nil {
		return nil, fmt.Errorf("failed to save summary to duckdb: %w", err)
	}

	// VectorDB 保存（embedding がある場合のみ）
	if len(summary.Embedding) > 0 {
		if err := r.vectordbStore.SaveThreadSummary(ctx, summary); err != nil {
			log.Printf("Failed to save summary to vectordb: %v", err)
		}
	}

	// Redis から Thread 削除
	if err := r.redisStore.DeleteThread(ctx, threadID); err != nil {
		log.Printf("Failed to delete thread from redis: %v", err)
	}

	return summary, nil
}

// IsNovelInformation は情報が新規かを判定
func (r *RealConversationManager) IsNovelInformation(ctx context.Context, msg conversation.Message) (bool, float32, error) {
	// TODO: Phase 3 で LLM による embedding 生成を実装
	// 現状は常に false を返す
	return false, 0.0, nil
}

// GetActiveThread は SessionID に紐づく ActiveThread を取得
func (r *RealConversationManager) GetActiveThread(ctx context.Context, sessionID string) (*conversation.Thread, error) {
	// Session 取得
	sess, err := r.redisStore.GetSession(ctx, sessionID)
	if err == conversation.ErrSessionNotFound {
		// Session なし → Thread なし
		return nil, conversation.ErrThreadNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// ActiveThread 取得（LastThreadIDを使用）
	if sess.LastThreadID == 0 {
		return nil, conversation.ErrThreadNotFound
	}

	thread, err := r.redisStore.GetThread(ctx, sess.LastThreadID)
	if err != nil {
		return nil, err
	}

	return thread, nil
}

// CreateThread は新規 Thread を作成
func (r *RealConversationManager) CreateThread(ctx context.Context, sessionID string, domain string) (*conversation.Thread, error) {
	// Session 取得または作成
	sess, err := r.redisStore.GetSession(ctx, sessionID)
	if err == conversation.ErrSessionNotFound {
		// 新規 Session 作成
		sess = conversation.NewSessionConversation(sessionID, "")
	} else if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// 新規 Thread 作成
	thread := conversation.NewThread(sessionID, domain)

	// Session 更新
	sess.LastThreadID = thread.ID
	sess.UpdatedAt = time.Now()

	// Redis 保存
	if err := r.redisStore.SaveThread(ctx, thread); err != nil {
		return nil, fmt.Errorf("failed to save thread to redis: %w", err)
	}
	if err := r.redisStore.SaveSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("failed to save session to redis: %w", err)
	}

	return thread, nil
}

// GetAgentStatus は Agent の状態を取得
func (r *RealConversationManager) GetAgentStatus(ctx context.Context, agentName string) (*conversation.AgentStatus, error) {
	// TODO: Phase 3 で Redis 実装（現状は空）
	return conversation.NewAgentStatus(agentName), nil
}

// UpdateAgentStatus は Agent の状態を更新
func (r *RealConversationManager) UpdateAgentStatus(ctx context.Context, status *conversation.AgentStatus) error {
	// TODO: Phase 3 で Redis 実装（現状は no-op）
	return nil
}

// --- ヘルパー関数 ---

// generateSimpleSummary は Thread の簡易要約を生成（Phase 3 で LLM 統合）
func generateSimpleSummary(thread *conversation.Thread) string {
	if len(thread.Turns) == 0 {
		return "Empty thread"
	}

	// 最初と最後のメッセージを結合
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

// extractSimpleKeywords は Thread の簡易キーワード抽出（Phase 3 で LLM 統合）
func extractSimpleKeywords(thread *conversation.Thread) []string {
	// 簡易実装: domain をキーワードとして返す
	return []string{thread.Domain}
}
