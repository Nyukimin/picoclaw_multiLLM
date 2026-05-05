package viewer

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type MemoryLayerHotStore interface {
	RecentBySession(ctx context.Context, sessionID string, limit int) ([]conversationpersistence.L1MemoryEvent, error)
	RecentByNamespace(ctx context.Context, namespace string, limit int) ([]conversationpersistence.L1MemoryEvent, error)
	RecentByState(ctx context.Context, memoryState string, limit int) ([]conversationpersistence.L1MemoryEvent, error)
}

type MemoryLayerColdStore interface {
	GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]*domconv.ThreadSummary, error)
	SearchByDomain(ctx context.Context, domain string, limit int) ([]*domconv.ThreadSummary, error)
	ListKBDocuments(ctx context.Context, domain string, limit int) ([]*domconv.Document, error)
}

func HandleMemoryLayers(hot MemoryLayerHotStore, cold MemoryLayerColdStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if hot == nil {
			http.Error(w, "memory layers unavailable", http.StatusServiceUnavailable)
			return
		}
		limit, err := parseViewerLimit(r.URL.Query().Get("limit"), 12, 50)
		if err != nil {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return
		}
		sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
		namespace := strings.TrimSpace(r.URL.Query().Get("namespace"))
		domain := strings.TrimSpace(r.URL.Query().Get("domain"))

		out := map[string]any{
			"session_id": sessionID,
			"namespace":  namespace,
			"domain":     domain,
			"l0":         []conversationpersistence.L1MemoryEvent{},
			"l1":         []conversationpersistence.L1MemoryEvent{},
			"l2":         []*domconv.ThreadSummary{},
			"l3":         []conversationpersistence.L1MemoryEvent{},
			"l3_qdrant":  []*domconv.Document{},
		}
		if sessionID != "" {
			l0, err := hot.RecentBySession(r.Context(), sessionID, limit)
			if err != nil {
				http.Error(w, "failed to load l0 memory", http.StatusInternalServerError)
				return
			}
			out["l0"] = l0
		}
		if namespace != "" {
			l1, err := hot.RecentByNamespace(r.Context(), namespace, limit)
			if err != nil {
				http.Error(w, "failed to load l1 memory", http.StatusInternalServerError)
				return
			}
			out["l1"] = l1
		}
		if cold != nil {
			var l2 []*domconv.ThreadSummary
			if sessionID != "" {
				history, err := cold.GetSessionHistory(r.Context(), sessionID, limit)
				if err != nil {
					http.Error(w, "failed to load l2 session memory", http.StatusInternalServerError)
					return
				}
				l2 = append(l2, history...)
			}
			if domain != "" {
				byDomain, err := cold.SearchByDomain(r.Context(), domain, limit)
				if err != nil {
					http.Error(w, "failed to load l2 domain memory", http.StatusInternalServerError)
					return
				}
				l2 = append(l2, byDomain...)
				kbDocs, err := cold.ListKBDocuments(r.Context(), domain, limit)
				if err != nil {
					http.Error(w, "failed to load l3 qdrant memory", http.StatusInternalServerError)
					return
				}
				out["l3_qdrant"] = kbDocs
			}
			out["l2"] = l2
		}
		l3, err := hot.RecentByState(r.Context(), conversationpersistence.MemoryStateConfirmed, limit)
		if err != nil {
			http.Error(w, "failed to load l3 memory", http.StatusInternalServerError)
			return
		}
		out["l3"] = l3

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}
