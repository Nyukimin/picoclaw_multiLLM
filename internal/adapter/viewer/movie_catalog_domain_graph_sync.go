package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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
	Available   bool              `json:"available"`
	DBPath      string            `json:"db_path"`
	Domain      string            `json:"domain"`
	EntityType  string            `json:"entity_type"`
	Checked     int               `json:"checked"`
	Upserted    int               `json:"upserted"`
	Skipped     int               `json:"skipped"`
	MovieIDs    []string          `json:"movie_ids"`
	SkipReasons map[string]int    `json:"skip_reasons"`
	ResolvedIDs map[string]string `json:"resolved_movie_ids,omitempty"`
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
		ResolvedIDs: map[string]string{},
	}
	if err := ensureMovieCatalogWorkTables(ctx, db); err != nil {
		return result, err
	}
	if err := ensureMovieCatalogIDAliasTables(ctx, db); err != nil {
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
		resolvedMovieID, canonicalID, err := resolveMovieCatalogWorkMovieID(ctx, db, item, work.MovieID)
		if err != nil {
			return result, err
		}
		if canonicalID != "" {
			result.ResolvedIDs[work.MovieID] = canonicalID
		}
		work.MovieID = resolvedMovieID
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

func ensureMovieCatalogIDAliasTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS movie_id_aliases(
	alias_id TEXT PRIMARY KEY,
	canonical_movie_id TEXT NOT NULL,
	source TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
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

func resolveMovieCatalogWorkMovieID(ctx context.Context, db *sql.DB, item conversationpersistence.L1DomainGraphAssertion, rawMovieID string) (string, string, error) {
	rawMovieID = strings.TrimSpace(rawMovieID)
	candidate := movieCatalogCanonicalIDCandidate(item)
	if candidate == "" {
		return rawMovieID, "", nil
	}
	exists, err := movieCatalogMovieIDExists(ctx, db, candidate)
	if err != nil {
		return rawMovieID, "", err
	}
	if !exists || candidate == rawMovieID {
		return rawMovieID, "", nil
	}
	if err := upsertMovieCatalogIDAlias(ctx, db, rawMovieID, candidate); err != nil {
		return rawMovieID, "", err
	}
	return candidate, candidate, nil
}

func movieCatalogCanonicalIDCandidate(item conversationpersistence.L1DomainGraphAssertion) string {
	entityID := strings.TrimSpace(item.EntityID)
	if strings.HasPrefix(entityID, "movie:") {
		if id := strings.TrimPrefix(entityID, "movie:"); movieCatalogNumericIDPattern.MatchString(id) {
			return id
		}
	}
	if id := movieCatalogEigaMovieIDFromURL(item.SourceURL); id != "" {
		return id
	}
	return movieCatalogEigaMovieIDFromURL(movieCatalogEvidenceString(item.Evidence, "source_url"))
}

var movieCatalogNumericIDPattern = regexp.MustCompile(`^\d+$`)
var movieCatalogEigaMovieURLPattern = regexp.MustCompile(`^https?://eiga\.com/movie/(\d+)/?$`)

func movieCatalogEigaMovieIDFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	match := movieCatalogEigaMovieURLPattern.FindStringSubmatch(rawURL)
	if match == nil {
		return ""
	}
	return match[1]
}

func movieCatalogMovieIDExists(ctx context.Context, db *sql.DB, movieID string) (bool, error) {
	var exists int
	err := db.QueryRowContext(ctx, "SELECT 1 FROM movies WHERE movie_id = ? LIMIT 1", movieID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func upsertMovieCatalogIDAlias(ctx context.Context, db *sql.DB, aliasID string, canonicalMovieID string) error {
	aliasID = strings.TrimSpace(aliasID)
	canonicalMovieID = strings.TrimSpace(canonicalMovieID)
	if aliasID == "" || canonicalMovieID == "" || aliasID == canonicalMovieID {
		return nil
	}
	_, err := db.ExecContext(ctx, `
INSERT INTO movie_id_aliases(alias_id, canonical_movie_id, source, created_at, updated_at)
VALUES(?, ?, 'domain_graph_sync', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
ON CONFLICT(alias_id) DO UPDATE SET
	canonical_movie_id = excluded.canonical_movie_id,
	source = excluded.source,
	updated_at = excluded.updated_at
`, aliasID, canonicalMovieID)
	return err
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
