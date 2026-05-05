package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type memoryLayerHotStoreStub struct {
	sessionID string
	namespace string
	state     string
	limit     int
}

func (s *memoryLayerHotStoreStub) RecentBySession(_ context.Context, sessionID string, limit int) ([]conversationpersistence.L1MemoryEvent, error) {
	s.sessionID = sessionID
	s.limit = limit
	return []conversationpersistence.L1MemoryEvent{{ID: "l0-1", SessionID: sessionID, Layer: "L0", Message: "current turn", CreatedAt: time.Now().UTC()}}, nil
}

func (s *memoryLayerHotStoreStub) RecentByNamespace(_ context.Context, namespace string, limit int) ([]conversationpersistence.L1MemoryEvent, error) {
	s.namespace = namespace
	s.limit = limit
	return []conversationpersistence.L1MemoryEvent{{ID: "l1-1", Namespace: namespace, Layer: "L1", Message: "today memory", CreatedAt: time.Now().UTC()}}, nil
}

func (s *memoryLayerHotStoreStub) RecentByState(_ context.Context, memoryState string, limit int) ([]conversationpersistence.L1MemoryEvent, error) {
	s.state = memoryState
	s.limit = limit
	return []conversationpersistence.L1MemoryEvent{{ID: "l3-1", MemoryState: memoryState, Layer: "L3", Message: "confirmed memory", CreatedAt: time.Now().UTC()}}, nil
}

type memoryLayerColdStoreStub struct {
	sessionID string
	domain    string
	kbDomain  string
	limit     int
	kbLimit   int
}

func (s *memoryLayerColdStoreStub) GetSessionHistory(_ context.Context, sessionID string, limit int) ([]*domconv.ThreadSummary, error) {
	s.sessionID = sessionID
	s.limit = limit
	return []*domconv.ThreadSummary{{ThreadID: 10, Domain: "chat", Summary: "monthly summary"}}, nil
}

func (s *memoryLayerColdStoreStub) SearchByDomain(_ context.Context, domain string, limit int) ([]*domconv.ThreadSummary, error) {
	s.domain = domain
	s.limit = limit
	return []*domconv.ThreadSummary{{ThreadID: 11, Domain: domain, Summary: "domain summary"}}, nil
}

func (s *memoryLayerColdStoreStub) ListKBDocuments(_ context.Context, domain string, limit int) ([]*domconv.Document, error) {
	s.kbDomain = domain
	s.kbLimit = limit
	return []*domconv.Document{{ID: "kb-1", Domain: domain, Content: "qdrant long-term knowledge"}}, nil
}

func TestHandleMemoryLayers(t *testing.T) {
	hot := &memoryLayerHotStoreStub{}
	cold := &memoryLayerColdStoreStub{}
	h := HandleMemoryLayers(hot, cold)

	req := httptest.NewRequest(http.MethodGet, "/viewer/memory/layers?session_id=session-1&namespace=user:ren&domain=movie&limit=4", nil)
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if hot.sessionID != "session-1" || hot.namespace != "user:ren" || hot.state != conversationpersistence.MemoryStateConfirmed || hot.limit != 4 {
		t.Fatalf("unexpected hot calls: %+v", hot)
	}
	if cold.sessionID != "session-1" || cold.domain != "movie" || cold.limit != 4 || cold.kbDomain != "movie" || cold.kbLimit != 4 {
		t.Fatalf("unexpected cold calls: %+v", cold)
	}

	var out struct {
		L0       []conversationpersistence.L1MemoryEvent `json:"l0"`
		L1       []conversationpersistence.L1MemoryEvent `json:"l1"`
		L2       []*domconv.ThreadSummary                `json:"l2"`
		L3       []conversationpersistence.L1MemoryEvent `json:"l3"`
		L3Qdrant []*domconv.Document                     `json:"l3_qdrant"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(out.L0) != 1 || len(out.L1) != 1 || len(out.L2) != 2 || len(out.L3) != 1 || len(out.L3Qdrant) != 1 {
		t.Fatalf("unexpected layer snapshot: %+v", out)
	}
}

func TestHandleMemoryLayersRequiresHotStore(t *testing.T) {
	h := HandleMemoryLayers(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/viewer/memory/layers", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
