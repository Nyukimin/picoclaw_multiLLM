package conversation

import (
	"context"
	"fmt"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// --- モック実装 ---

type mockRedisStore struct {
	sessions map[string]*domconv.SessionConversation
	threads  map[int64]*domconv.Thread
}

func newMockRedisStore() *mockRedisStore {
	return &mockRedisStore{
		sessions: make(map[string]*domconv.SessionConversation),
		threads:  make(map[int64]*domconv.Thread),
	}
}

func (m *mockRedisStore) SaveSession(_ context.Context, sess *domconv.SessionConversation) error {
	m.sessions[sess.ID] = sess
	return nil
}
func (m *mockRedisStore) GetSession(_ context.Context, sessionID string) (*domconv.SessionConversation, error) {
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, domconv.ErrSessionNotFound
	}
	return s, nil
}
func (m *mockRedisStore) DeleteSession(_ context.Context, sessionID string) error {
	delete(m.sessions, sessionID)
	return nil
}
func (m *mockRedisStore) ListActiveSessions(_ context.Context) ([]string, error) {
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids, nil
}
func (m *mockRedisStore) SaveThread(_ context.Context, thread *domconv.Thread) error {
	m.threads[thread.ID] = thread
	return nil
}
func (m *mockRedisStore) GetThread(_ context.Context, threadID int64) (*domconv.Thread, error) {
	t, ok := m.threads[threadID]
	if !ok {
		return nil, domconv.ErrThreadNotFound
	}
	return t, nil
}
func (m *mockRedisStore) DeleteThread(_ context.Context, threadID int64) error {
	delete(m.threads, threadID)
	return nil
}
func (m *mockRedisStore) Close() error { return nil }

type mockDuckDBStore struct {
	saved []*domconv.ThreadSummary
}

func (m *mockDuckDBStore) SaveThreadSummary(_ context.Context, s *domconv.ThreadSummary) error {
	m.saved = append(m.saved, s)
	return nil
}
func (m *mockDuckDBStore) GetSessionHistory(_ context.Context, _ string, _ int) ([]*domconv.ThreadSummary, error) {
	return m.saved, nil
}
func (m *mockDuckDBStore) SearchByDomain(_ context.Context, _ string, _ int) ([]*domconv.ThreadSummary, error) {
	return nil, nil
}
func (m *mockDuckDBStore) CleanupOldRecords(_ context.Context) (int64, error) { return 0, nil }
func (m *mockDuckDBStore) Close() error                                        { return nil }

type mockVectorDBStore struct {
	saved     []*domconv.ThreadSummary
	mockScore float32
}

func (m *mockVectorDBStore) SaveThreadSummary(_ context.Context, s *domconv.ThreadSummary) error {
	m.saved = append(m.saved, s)
	return nil
}
func (m *mockVectorDBStore) SearchSimilar(_ context.Context, _ []float32, _ int) ([]*domconv.ThreadSummary, error) {
	if len(m.saved) == 0 {
		return nil, nil
	}
	result := make([]*domconv.ThreadSummary, 0, len(m.saved))
	for _, s := range m.saved {
		cp := *s
		cp.Score = m.mockScore
		result = append(result, &cp)
	}
	return result, nil
}
func (m *mockVectorDBStore) SearchByDomain(_ context.Context, _ string, _ int) ([]*domconv.ThreadSummary, error) {
	return nil, nil
}
func (m *mockVectorDBStore) IsNovelQuery(_ context.Context, _ []float32, threshold float32) (bool, float32, error) {
	return m.mockScore < threshold, m.mockScore, nil
}
func (m *mockVectorDBStore) SaveKB(_ context.Context, _ *domconv.Document) error {
	return nil
}
func (m *mockVectorDBStore) SearchKB(_ context.Context, _ string, _ []float32, _ int) ([]*domconv.Document, error) {
	return []*domconv.Document{}, nil
}
func (m *mockVectorDBStore) ListKBDocuments(_ context.Context, _ string, _ int) ([]*domconv.Document, error) {
	return []*domconv.Document{}, nil
}
func (m *mockVectorDBStore) GetKBCollections(_ context.Context) ([]string, error) {
	return []string{}, nil
}
func (m *mockVectorDBStore) GetKBStats(_ context.Context, _ string) (*KBStats, error) {
	return &KBStats{Domain: "test", DocumentCount: 0, VectorSize: 768}, nil
}
func (m *mockVectorDBStore) DeleteOldKBDocuments(_ context.Context, _ string, _ time.Time) (int, error) {
	return 0, nil
}
func (m *mockVectorDBStore) Close() error { return nil }

type mockEmbeddingProvider struct {
	vec []float32
	err error
}

func (m *mockEmbeddingProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	return m.vec, m.err
}

type mockSummarizer struct {
	summary  string
	keywords []string
	err      error
}

func (m *mockSummarizer) Summarize(_ context.Context, _ *domconv.Thread) (string, error) {
	return m.summary, m.err
}
func (m *mockSummarizer) ExtractKeywords(_ context.Context, _ *domconv.Thread) ([]string, error) {
	return m.keywords, m.err
}

// dummy for time import
var _ = time.Duration(0)

// --- ヘルパー ---

func newTestManager(embedder domconv.EmbeddingProvider, summarizer domconv.ConversationSummarizer) *RealConversationManager {
	return &RealConversationManager{
		redisStore:    newMockRedisStore(),
		duckdbStore:   &mockDuckDBStore{},
		vectordbStore: &mockVectorDBStore{mockScore: 0.5},
		embedder:      embedder,
		summarizer:    summarizer,
	}
}

// --- テスト ---

func TestFlushThread_WithLLMSummary(t *testing.T) {
	embedder := &mockEmbeddingProvider{vec: []float32{0.1, 0.2, 0.3}}
	summarizer := &mockSummarizer{
		summary:  "Go言語の基本について話し合った",
		keywords: []string{"Go", "プログラミング", "言語"},
	}
	mgr := newTestManager(embedder, summarizer)
	ctx := context.Background()

	thread, err := mgr.CreateThread(ctx, "sess-1", "programming")
	if err != nil {
		t.Fatalf("CreateThread failed: %v", err)
	}
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerUser, "Go言語について教えて", nil))
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerMio, "Go言語はGoogleが開発したシステム言語です", nil))
	mgr.redisStore.(*mockRedisStore).threads[thread.ID] = thread

	summary, err := mgr.FlushThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("FlushThread failed: %v", err)
	}

	if summary.Summary != "Go言語の基本について話し合った" {
		t.Errorf("Expected LLM summary, got: %s", summary.Summary)
	}
	if len(summary.Keywords) != 3 {
		t.Errorf("Expected 3 keywords, got %d", len(summary.Keywords))
	}
	if len(summary.Embedding) == 0 {
		t.Error("Expected embedding to be generated")
	}
}

func TestFlushThread_EmbedderError_FallsBackToSimple(t *testing.T) {
	embedder := &mockEmbeddingProvider{err: fmt.Errorf("API error")}
	summarizer := &mockSummarizer{summary: "summary", keywords: []string{"kw"}}
	mgr := newTestManager(embedder, summarizer)
	ctx := context.Background()

	thread, _ := mgr.CreateThread(ctx, "sess-2", "general")
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerUser, "hello", nil))
	mgr.redisStore.(*mockRedisStore).threads[thread.ID] = thread

	// Embedderエラーでも FlushThread は成功する（embeddingなしで保存）
	summary, err := mgr.FlushThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("FlushThread should not fail on embedder error: %v", err)
	}
	if len(summary.Embedding) != 0 {
		t.Error("Embedding should be empty on error")
	}
}

func TestFlushThread_NoSummarizer_FallsBackToSimple(t *testing.T) {
	mgr := newTestManager(nil, nil)
	ctx := context.Background()

	thread, _ := mgr.CreateThread(ctx, "sess-3", "general")
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerUser, "こんにちは", nil))
	mgr.redisStore.(*mockRedisStore).threads[thread.ID] = thread

	summary, err := mgr.FlushThread(ctx, thread.ID)
	if err != nil {
		t.Fatalf("FlushThread failed: %v", err)
	}
	if summary.Summary == "" {
		t.Error("Summary should not be empty")
	}
	// 簡易実装のフォールバック確認
	if len(summary.Keywords) == 0 {
		t.Error("Keywords should not be empty (domain fallback)")
	}
}

func TestIsNovelInformation_EmptyVectorDB_IsNovel(t *testing.T) {
	embedder := &mockEmbeddingProvider{vec: []float32{0.1, 0.2, 0.3}}
	mgr := newTestManager(embedder, &mockSummarizer{})
	ctx := context.Background()

	msg := domconv.NewMessage(domconv.SpeakerUser, "新しい情報です", nil)

	isNovel, _, err := mgr.IsNovelInformation(ctx, msg)
	if err != nil {
		t.Fatalf("IsNovelInformation failed: %v", err)
	}
	// vectordbが空（score=0.5 < threshold=0.85） → 新規情報
	if !isNovel {
		t.Error("Should be novel when similarity is below threshold")
	}
}

func TestIsNovelInformation_HighSimilarity_NotNovel(t *testing.T) {
	embedding := []float32{0.1, 0.2, 0.3}
	embedder := &mockEmbeddingProvider{vec: embedding}
	// 類似度スコア0.95（閾値0.85を超える → 新規でない）
	vdb := &mockVectorDBStore{mockScore: 0.95}
	vdb.saved = []*domconv.ThreadSummary{{Summary: "既存の記憶"}}
	mgr := &RealConversationManager{
		redisStore:    newMockRedisStore(),
		duckdbStore:   &mockDuckDBStore{},
		vectordbStore: vdb,
		embedder:      embedder,
		summarizer:    &mockSummarizer{},
	}
	ctx := context.Background()

	msg := domconv.NewMessage(domconv.SpeakerUser, "似たような情報", nil)
	isNovel, score, err := mgr.IsNovelInformation(ctx, msg)
	if err != nil {
		t.Fatalf("IsNovelInformation failed: %v", err)
	}
	if isNovel {
		t.Errorf("Should not be novel when similarity=%.2f >= threshold", score)
	}
}

func TestIsNovelInformation_NoEmbedder_ReturnsFalse(t *testing.T) {
	mgr := newTestManager(nil, &mockSummarizer{})
	ctx := context.Background()
	msg := domconv.NewMessage(domconv.SpeakerUser, "何か", nil)

	isNovel, _, err := mgr.IsNovelInformation(ctx, msg)
	if err != nil {
		t.Fatalf("should not error: %v", err)
	}
	if isNovel {
		t.Error("Should return false when embedder is not configured")
	}
}
