package conversation

import (
	"context"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// redisStoreIface はRedisStoreのインターフェース（テスト用モック差し替え可能）
type redisStoreIface interface {
	SaveSession(ctx context.Context, sess *conversation.SessionConversation) error
	GetSession(ctx context.Context, sessionID string) (*conversation.SessionConversation, error)
	DeleteSession(ctx context.Context, sessionID string) error
	ListActiveSessions(ctx context.Context) ([]string, error)
	SaveThread(ctx context.Context, thread *conversation.Thread) error
	GetThread(ctx context.Context, threadID int64) (*conversation.Thread, error)
	DeleteThread(ctx context.Context, threadID int64) error
	Close() error
}

// duckdbStoreIface はDuckDBStoreのインターフェース
type duckdbStoreIface interface {
	SaveThreadSummary(ctx context.Context, summary *conversation.ThreadSummary) error
	GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]*conversation.ThreadSummary, error)
	SearchByDomain(ctx context.Context, domain string, limit int) ([]*conversation.ThreadSummary, error)
	CleanupOldRecords(ctx context.Context) (int64, error)
	Close() error
}

// vectordbStoreIface はVectorDBStoreのインターフェース
type vectordbStoreIface interface {
	SaveThreadSummary(ctx context.Context, summary *conversation.ThreadSummary) error
	SearchSimilar(ctx context.Context, queryEmbedding []float32, topK int) ([]*conversation.ThreadSummary, error)
	SearchByDomain(ctx context.Context, domain string, limit int) ([]*conversation.ThreadSummary, error)
	IsNovelQuery(ctx context.Context, queryEmbedding []float32, threshold float32) (bool, float32, error)
	// KB (Knowledge Base) メソッド
	SaveKB(ctx context.Context, doc *conversation.Document) error
	SearchKB(ctx context.Context, domain string, queryEmbedding []float32, topK int) ([]*conversation.Document, error)
	// KB管理メソッド (kb-admin用)
	ListKBDocuments(ctx context.Context, domain string, limit int) ([]*conversation.Document, error)
	GetKBCollections(ctx context.Context) ([]string, error)
	GetKBStats(ctx context.Context, domain string) (*KBStats, error)
	DeleteOldKBDocuments(ctx context.Context, domain string, before time.Time) (int, error)
	Close() error
}

// noveltyThreshold は「新規情報」と判定する類似度の閾値
const noveltyThreshold = float32(0.85)

// _ はコンパイル時のインターフェース適合チェック
var _ redisStoreIface = (*RedisStore)(nil)
var _ duckdbStoreIface = (*DuckDBStore)(nil)
var _ vectordbStoreIface = (*VectorDBStore)(nil)
