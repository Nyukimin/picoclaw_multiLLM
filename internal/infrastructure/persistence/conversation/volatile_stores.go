package conversation

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

type volatileRedisStore struct {
	mu       sync.RWMutex
	sessions map[string]*domconv.SessionConversation
	threads  map[int64]*domconv.Thread
}

func newVolatileRedisStore() *volatileRedisStore {
	return &volatileRedisStore{
		sessions: map[string]*domconv.SessionConversation{},
		threads:  map[int64]*domconv.Thread{},
	}
}

func (s *volatileRedisStore) SaveSession(_ context.Context, sess *domconv.SessionConversation) error {
	if sess == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = cloneSessionConversation(sess)
	return nil
}

func (s *volatileRedisStore) GetSession(_ context.Context, sessionID string) (*domconv.SessionConversation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, domconv.ErrSessionNotFound
	}
	return cloneSessionConversation(sess), nil
}

func (s *volatileRedisStore) DeleteSession(_ context.Context, sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	return nil
}

func (s *volatileRedisStore) ListActiveSessions(_ context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.sessions))
	for key := range s.sessions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *volatileRedisStore) SaveThread(_ context.Context, thread *domconv.Thread) error {
	if thread == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.threads[thread.ID] = cloneThread(thread)
	return nil
}

func (s *volatileRedisStore) GetThread(_ context.Context, threadID int64) (*domconv.Thread, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	thread, ok := s.threads[threadID]
	if !ok {
		return nil, domconv.ErrThreadNotFound
	}
	return cloneThread(thread), nil
}

func (s *volatileRedisStore) DeleteThread(_ context.Context, threadID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.threads, threadID)
	return nil
}

func (s *volatileRedisStore) Close() error {
	return nil
}

func cloneSessionConversation(in *domconv.SessionConversation) *domconv.SessionConversation {
	if in == nil {
		return nil
	}
	var out domconv.SessionConversation
	if data, err := json.Marshal(in); err == nil {
		if err := json.Unmarshal(data, &out); err == nil {
			return &out
		}
	}
	out = *in
	out.History = append([]domconv.ThreadSummary(nil), in.History...)
	return &out
}

func cloneThread(in *domconv.Thread) *domconv.Thread {
	if in == nil {
		return nil
	}
	var out domconv.Thread
	if data, err := json.Marshal(in); err == nil {
		if err := json.Unmarshal(data, &out); err == nil {
			return &out
		}
	}
	out = *in
	out.Turns = append([]domconv.Message(nil), in.Turns...)
	out.Targets = append([]string(nil), in.Targets...)
	if in.Cooldown != nil {
		out.Cooldown = map[string]int{}
		for key, value := range in.Cooldown {
			out.Cooldown[key] = value
		}
	}
	return &out
}

type noopDuckDBStore struct{}

func newNoopDuckDBStore() *noopDuckDBStore {
	return &noopDuckDBStore{}
}

func (s *noopDuckDBStore) SaveThreadSummary(context.Context, *domconv.ThreadSummary) error {
	return nil
}

func (s *noopDuckDBStore) GetSessionHistory(context.Context, string, int) ([]*domconv.ThreadSummary, error) {
	return []*domconv.ThreadSummary{}, nil
}

func (s *noopDuckDBStore) SearchByDomain(context.Context, string, int) ([]*domconv.ThreadSummary, error) {
	return []*domconv.ThreadSummary{}, nil
}

func (s *noopDuckDBStore) SearchKnowledgeArchiveFTS(context.Context, string, string, int) ([]L1KnowledgeItem, error) {
	return []L1KnowledgeItem{}, nil
}

func (s *noopDuckDBStore) ExportThreadSummariesParquet(context.Context, string) error {
	return nil
}

func (s *noopDuckDBStore) ExportL1ArchivesParquet(context.Context, string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (s *noopDuckDBStore) CleanupOldRecords(context.Context) (int64, error) {
	return 0, nil
}

func (s *noopDuckDBStore) ArchiveL1MemoryEvents(context.Context, []L1MemoryEvent) error {
	return nil
}

func (s *noopDuckDBStore) ArchiveL1NewsItems(context.Context, []L1NewsItem) error {
	return nil
}

func (s *noopDuckDBStore) ArchiveL1KnowledgeItems(context.Context, []L1KnowledgeItem) error {
	return nil
}

func (s *noopDuckDBStore) ArchiveL1StagingItems(context.Context, []L1StagingItem) error {
	return nil
}

func (s *noopDuckDBStore) Close() error {
	return nil
}

type noopVectorDBStore struct{}

func newNoopVectorDBStore() *noopVectorDBStore {
	return &noopVectorDBStore{}
}

func (s *noopVectorDBStore) SaveThreadSummary(context.Context, *domconv.ThreadSummary) error {
	return nil
}

func (s *noopVectorDBStore) SearchSimilar(context.Context, []float32, int) ([]*domconv.ThreadSummary, error) {
	return []*domconv.ThreadSummary{}, nil
}

func (s *noopVectorDBStore) SearchByDomain(context.Context, string, int) ([]*domconv.ThreadSummary, error) {
	return []*domconv.ThreadSummary{}, nil
}

func (s *noopVectorDBStore) IsNovelQuery(context.Context, []float32, float32) (bool, float32, error) {
	return false, 0, nil
}

func (s *noopVectorDBStore) SaveKB(context.Context, *domconv.Document) error {
	return nil
}

func (s *noopVectorDBStore) SearchKB(context.Context, string, []float32, int) ([]*domconv.Document, error) {
	return []*domconv.Document{}, nil
}

func (s *noopVectorDBStore) ListKBDocuments(context.Context, string, int) ([]*domconv.Document, error) {
	return []*domconv.Document{}, nil
}

func (s *noopVectorDBStore) GetKBCollections(context.Context) ([]string, error) {
	return []string{}, nil
}

func (s *noopVectorDBStore) GetKBStats(_ context.Context, domain string) (*KBStats, error) {
	return &KBStats{Domain: domain}, nil
}

func (s *noopVectorDBStore) DeleteOldKBDocuments(context.Context, string, time.Time) (int, error) {
	return 0, nil
}

func (s *noopVectorDBStore) Close() error {
	return nil
}

var _ redisStoreIface = (*volatileRedisStore)(nil)
var _ duckdbStoreIface = (*noopDuckDBStore)(nil)
var _ L1ArchiveStore = (*noopDuckDBStore)(nil)
var _ vectordbStoreIface = (*noopVectorDBStore)(nil)
