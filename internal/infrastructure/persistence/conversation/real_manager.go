package conversation

import (
	"fmt"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// RealConversationManager は実ストアを統合した会話管理実装
type RealConversationManager struct {
	redisStore    redisStoreIface
	l1Store       l1StoreIface
	duckdbStore   duckdbStoreIface
	vectordbStore vectordbStoreIface
	embedder      domconv.EmbeddingProvider      // nilの場合はVectorDB機能無効
	summarizer    domconv.ConversationSummarizer // nilの場合は簡易実装
	agentStatuses map[string]*domconv.AgentStatus
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
		agentStatuses: map[string]*domconv.AgentStatus{},
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
		l1.WithKnowledgeVectorSink(r)
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

// --- 内部ヘルパー ---

// --- KB管理API (kb-admin用) ---
