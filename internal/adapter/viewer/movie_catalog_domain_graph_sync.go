package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

const (
	defaultMovieDomainGraphSyncLimit = 200
	maxMovieDomainGraphSyncLimit     = 500
)

type MovieDomainGraphAssertionStore interface {
	DomainGraphAssertions(ctx context.Context, q conversationpersistence.DomainGraphAssertionQuery) (int, []conversationpersistence.L1DomainGraphAssertion, error)
}

type movieDomainGraphSyncResult struct {
	Available   bool           `json:"available"`
	DBPath      string         `json:"db_path"`
	Domain      string         `json:"domain"`
	EntityType  string         `json:"entity_type"`
	Checked     int            `json:"checked"`
	Upserted    int            `json:"upserted"`
	Skipped     int            `json:"skipped"`
	MovieIDs    []string       `json:"movie_ids"`
	SkipReasons map[string]int `json:"skip_reasons"`
}

type movieCatalogWorkUpsert struct {
	MovieID  string
	Title    string
	URL      string
	Synopsis string
}

func HandleMovieDomainGraphSync(opts MovieCatalogOptions, store MovieDomainGraphAssertionStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if store == nil {
			http.Error(w, "movie domain graph sync unavailable", http.StatusServiceUnavailable)
			return
		}
		limit, err := movieDomainGraphSyncLimit(r)
		if err != nil {
			http.Error(w, "invalid movie domain graph sync request", http.StatusBadRequest)
			return
		}
		dbPath := resolveMovieCatalogWritableDBPath(opts.DBPath)
		if strings.TrimSpace(dbPath) == "" {
			http.Error(w, "movie domain graph sync unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			http.Error(w, "movie domain graph sync unavailable", http.StatusServiceUnavailable)
			return
		}
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			http.Error(w, "movie domain graph sync unavailable", http.StatusServiceUnavailable)
			return
		}
		defer db.Close()

		_, items, err := store.DomainGraphAssertions(r.Context(), conversationpersistence.DomainGraphAssertionQuery{
			Domain:           "movie",
			EntityType:       "work",
			ValidationStatus: conversationpersistence.L1StagingStatusValidated,
			Limit:            limit,
		})
		if err != nil {
			http.Error(w, "failed to sync movie domain graph assertions", http.StatusInternalServerError)
			return
		}
		result, err := syncMovieDomainGraphAssertions(r.Context(), db, items)
		if err != nil {
			http.Error(w, "failed to sync movie domain graph assertions", http.StatusInternalServerError)
			return
		}
		result.Available = true
		result.DBPath = dbPath
		result.Domain = "movie"
		result.EntityType = "work"
		writeMovieDomainGraphSyncJSON(w, result)
	}
}

func movieDomainGraphSyncLimit(r *http.Request) (int, error) {
	limit := defaultMovieDomainGraphSyncLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid limit")
		}
		if n > maxMovieDomainGraphSyncLimit {
			n = maxMovieDomainGraphSyncLimit
		}
		limit = n
	}
	return limit, nil
}

func syncMovieDomainGraphAssertions(ctx context.Context, db *sql.DB, items []conversationpersistence.L1DomainGraphAssertion) (movieDomainGraphSyncResult, error) {
	result := movieDomainGraphSyncResult{
		Checked:     len(items),
		MovieIDs:    []string{},
		SkipReasons: map[string]int{},
	}
	if err := ensureMovieCatalogWorkTables(ctx, db); err != nil {
		return result, err
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	for _, item := range items {
		work, skipReason := movieCatalogWorkFromAssertion(item)
		if skipReason != "" {
			result.Skipped++
			result.SkipReasons[skipReason]++
			continue
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO movies(movie_id, title, url, synopsis)
VALUES(?, ?, ?, ?)
ON CONFLICT(movie_id) DO UPDATE SET
	title = excluded.title,
	url = excluded.url,
	synopsis = excluded.synopsis
`, work.MovieID, work.Title, work.URL, work.Synopsis); err != nil {
			return result, err
		}
		result.Upserted++
		result.MovieIDs = append(result.MovieIDs, work.MovieID)
	}
	return result, nil
}

func ensureMovieCatalogWorkTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS movies(
	movie_id TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	url TEXT NOT NULL,
	synopsis TEXT
)`)
	return err
}

func movieCatalogWorkFromAssertion(item conversationpersistence.L1DomainGraphAssertion) (movieCatalogWorkUpsert, string) {
	entityID := strings.TrimSpace(item.EntityID)
	if entityID == "" {
		return movieCatalogWorkUpsert{}, "missing_entity_id"
	}
	summary := strings.TrimSpace(item.Summary)
	sourceURL := strings.TrimSpace(item.SourceURL)
	if summary == "" && sourceURL == "" {
		return movieCatalogWorkUpsert{}, "empty_work_payload"
	}
	title := movieCatalogEvidenceString(item.Evidence, "title")
	if title == "" {
		title = movieCatalogEvidenceString(item.Evidence, "movie_title")
	}
	if title == "" {
		title = summary
	}
	if title == "" {
		title = entityID
	}
	url := sourceURL
	if url == "" {
		url = movieCatalogEvidenceString(item.Evidence, "source_url")
	}
	return movieCatalogWorkUpsert{
		MovieID:  entityID,
		Title:    title,
		URL:      url,
		Synopsis: summary,
	}, ""
}

func movieCatalogEvidenceString(evidence map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if evidence == nil {
			return ""
		}
		value, ok := evidence[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func writeMovieDomainGraphSyncJSON(w http.ResponseWriter, payload movieDomainGraphSyncResult) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
