package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	conversationpersistence "github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/persistence/conversation"
)

type movieDomainGraphStoreStub struct {
	total int
	items []conversationpersistence.L1DomainGraphAssertion
	query conversationpersistence.DomainGraphAssertionQuery
	err   error
}

func (s *movieDomainGraphStoreStub) DomainGraphAssertions(ctx context.Context, q conversationpersistence.DomainGraphAssertionQuery) (int, []conversationpersistence.L1DomainGraphAssertion, error) {
	s.query = q
	if s.err != nil {
		return 0, nil, s.err
	}
	return s.total, s.items, nil
}

func TestHandleMovieDomainGraphSyncUpsertsMovieWorks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "eiga_catalog.sqlite")
	now := time.Now().UTC()
	store := &movieDomainGraphStoreStub{
		total: 2,
		items: []conversationpersistence.L1DomainGraphAssertion{
			{
				ID:               "dg:movie:1",
				Domain:           "movie",
				EntityType:       "work",
				EntityID:         "movie:1",
				SourceURL:        "https://example.com/movie/1",
				Summary:          "Movie summary",
				ValidationStatus: conversationpersistence.L1StagingStatusValidated,
				Evidence: map[string]interface{}{
					"title": "Evidence Title",
				},
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:               "dg:movie:skip",
				Domain:           "movie",
				EntityType:       "work",
				SourceURL:        "https://example.com/movie/skip",
				Summary:          "Skip summary",
				ValidationStatus: conversationpersistence.L1StagingStatusValidated,
				CreatedAt:        now,
				UpdatedAt:        now,
			},
		},
	}
	h := HandleMovieDomainGraphSync(MovieCatalogOptions{DBPath: dbPath}, store)

	req := httptest.NewRequest(http.MethodPost, "/viewer/movie-catalog/domain-graph-sync?limit=10", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if store.query.Domain != "movie" || store.query.EntityType != "work" || store.query.ValidationStatus != conversationpersistence.L1StagingStatusValidated || store.query.Limit != 10 {
		t.Fatalf("unexpected query: %+v", store.query)
	}
	var out movieDomainGraphSyncResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !out.Available || out.DBPath != dbPath || out.Domain != "movie" || out.EntityType != "work" {
		t.Fatalf("unexpected response identity: %+v", out)
	}
	if out.Checked != 2 || out.Upserted != 1 || out.Skipped != 1 {
		t.Fatalf("unexpected counts: %+v", out)
	}
	if len(out.MovieIDs) != 1 || out.MovieIDs[0] != "movie:1" {
		t.Fatalf("unexpected movie ids: %+v", out.MovieIDs)
	}
	if out.SkipReasons["missing_entity_id"] != 1 {
		t.Fatalf("unexpected skip reasons: %+v", out.SkipReasons)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	var title, url, synopsis string
	if err := db.QueryRow("SELECT title, url, COALESCE(synopsis, '') FROM movies WHERE movie_id = ?", "movie:1").Scan(&title, &url, &synopsis); err != nil {
		t.Fatalf("query synced movie: %v", err)
	}
	if title != "Evidence Title" || url != "https://example.com/movie/1" || synopsis != "Movie summary" {
		t.Fatalf("unexpected synced movie: title=%q url=%q synopsis=%q", title, url, synopsis)
	}
}

func TestHandleMovieDomainGraphSyncUnavailable(t *testing.T) {
	h := HandleMovieDomainGraphSync(MovieCatalogOptions{DBPath: filepath.Join(t.TempDir(), "eiga_catalog.sqlite")}, nil)
	req := httptest.NewRequest(http.MethodPost, "/viewer/movie-catalog/domain-graph-sync", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "movie domain graph sync unavailable") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestHandleMovieDomainGraphSyncRejectsInvalidMethod(t *testing.T) {
	h := HandleMovieDomainGraphSync(MovieCatalogOptions{DBPath: filepath.Join(t.TempDir(), "eiga_catalog.sqlite")}, &movieDomainGraphStoreStub{})
	req := httptest.NewRequest(http.MethodGet, "/viewer/movie-catalog/domain-graph-sync", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleMovieDomainGraphSyncRejectsInvalidLimit(t *testing.T) {
	h := HandleMovieDomainGraphSync(MovieCatalogOptions{DBPath: filepath.Join(t.TempDir(), "eiga_catalog.sqlite")}, &movieDomainGraphStoreStub{})
	req := httptest.NewRequest(http.MethodPost, "/viewer/movie-catalog/domain-graph-sync?limit=-1", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid movie domain graph sync request") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}
