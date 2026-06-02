package viewer

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleMovieCatalogMoviesSearchAndLimit(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	h := HandleMovieCatalog(MovieCatalogOptions{DBPath: dbPath})

	req := httptest.NewRequest(http.MethodGet, "/viewer/movie-catalog?action=movies&q=ケビン&limit=99", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Available bool                    `json:"available"`
		Total     int                     `json:"total"`
		Limit     int                     `json:"limit"`
		Items     []movieCatalogMovieItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !out.Available {
		t.Fatal("expected available catalog")
	}
	if out.Limit != maxMovieCatalogLimit {
		t.Fatalf("expected capped limit %d, got %d", maxMovieCatalogLimit, out.Limit)
	}
	if out.Total != 1 || len(out.Items) != 1 || out.Items[0].Title != "マージン・コール" {
		t.Fatalf("unexpected movie search result: %+v", out)
	}
}

func TestHandleMovieCatalogPersonDetailReturnsMovieLinks(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	h := HandleMovieCatalog(MovieCatalogOptions{DBPath: dbPath})

	req := httptest.NewRequest(http.MethodGet, "/viewer/movie-catalog?action=person&id=30003", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Detail struct {
			Person movieCatalogPersonItem `json:"person"`
			Links  []movieCatalogEdgeItem `json:"links"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out.Detail.Person.Name != "ケビン・スペイシー" {
		t.Fatalf("unexpected person: %+v", out.Detail.Person)
	}
	if len(out.Detail.Links) != 1 || out.Detail.Links[0].MovieTitle != "マージン・コール" {
		t.Fatalf("expected linked movie edge, got %+v", out.Detail.Links)
	}
	if !out.Detail.Links[0].MovieFetched || !out.Detail.Links[0].PersonFetched {
		t.Fatalf("expected fetched flags on linked edge, got %+v", out.Detail.Links[0])
	}
}

func TestHandleMovieCatalogMovieDetailMarksUnfetchedPersonLinks(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO movie_people(movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
VALUES('57573','99999','出演','movie_cast','マージン・コール','未取得の人物','https://eiga.com/movie/57573/','https://eiga.com/person/99999/');
`); err != nil {
		db.Close()
		t.Fatalf("seed unfetched edge: %v", err)
	}
	db.Close()
	h := HandleMovieCatalog(MovieCatalogOptions{DBPath: dbPath})

	req := httptest.NewRequest(http.MethodGet, "/viewer/movie-catalog?action=movie&id=57573", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Detail struct {
			Links []movieCatalogEdgeItem `json:"links"`
		} `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	var found bool
	for _, link := range out.Detail.Links {
		if link.PersonID == "99999" {
			found = true
			if !link.MovieFetched || link.PersonFetched {
				t.Fatalf("expected fetched movie and unfetched person flags, got %+v", link)
			}
		}
	}
	if !found {
		t.Fatalf("expected unfetched person edge, got %+v", out.Detail.Links)
	}
}

func TestHandleMovieCatalogMissingDBIsSoftUnavailable(t *testing.T) {
	h := HandleMovieCatalog(MovieCatalogOptions{DBPath: filepath.Join(t.TempDir(), "missing.sqlite")})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/viewer/movie-catalog?action=stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var out movieCatalogResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out.Available {
		t.Fatal("missing catalog should be available=false")
	}
}

func TestResolveMovieCatalogFetchTargetByMovieName(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	url, candidates, err := resolveMovieCatalogFetchTarget(db, movieCatalogFetchRequest{Kind: "movie", Query: "マージン・コール"})
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if url != "https://eiga.com/movie/57573/" {
		t.Fatalf("unexpected url: %s", url)
	}
	if len(candidates) != 1 || candidates[0].Title != "マージン・コール" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestResolveMovieCatalogFetchTargetUsesUnfetchedEdgeMovieName(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
INSERT INTO movie_people(movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
VALUES('103262','30003','出演','person_filmography','爆弾','ケビン・スペイシー','https://eiga.com/movie/103262/','https://eiga.com/person/30003/');
`); err != nil {
		t.Fatalf("seed edge movie: %v", err)
	}

	url, candidates, err := resolveMovieCatalogFetchTarget(db, movieCatalogFetchRequest{Kind: "movie", Query: "爆弾"})
	if err != nil {
		t.Fatalf("resolve target: %v", err)
	}
	if url != "https://eiga.com/movie/103262/" {
		t.Fatalf("unexpected url: %s", url)
	}
	if len(candidates) != 1 || candidates[0].Title != "爆弾" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}
}

func TestHandleMovieCatalogFetchNoCandidatesReturnsStructuredHint(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	h := HandleMovieCatalogFetch(MovieCatalogOptions{DBPath: dbPath})

	req := httptest.NewRequest(http.MethodPost, "/viewer/movie-catalog/fetch", strings.NewReader(`{"kind":"movie","query":"爆弾","max_pages":1}`))
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var out movieCatalogFetchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out.Status != "candidates" || out.Kind != "movie" || out.Query != "爆弾" {
		t.Fatalf("unexpected structured hint response: %+v", out)
	}
	if len(out.Candidates) != 0 {
		t.Fatalf("expected no local candidates, got %+v", out.Candidates)
	}
}

func TestResolveMovieCatalogFetchTargetRejectsKindMismatch(t *testing.T) {
	dbPath := seedMovieCatalogTestDB(t)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, _, err = resolveMovieCatalogFetchTarget(db, movieCatalogFetchRequest{Kind: "person", URL: "https://eiga.com/movie/57573/"})
	if err == nil {
		t.Fatal("expected kind mismatch error")
	}
}

func seedMovieCatalogTestDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eiga_catalog.sqlite")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
CREATE TABLE movies(movie_id TEXT PRIMARY KEY, title TEXT NOT NULL, url TEXT NOT NULL, synopsis TEXT);
CREATE TABLE people(person_id TEXT PRIMARY KEY, name TEXT NOT NULL, url TEXT NOT NULL, profile_json TEXT, biography TEXT);
CREATE TABLE movie_people(
  movie_id TEXT NOT NULL,
  person_id TEXT NOT NULL,
  role TEXT NOT NULL,
  source TEXT NOT NULL,
  movie_title TEXT,
  person_name TEXT,
  movie_url TEXT,
  person_url TEXT,
  PRIMARY KEY(movie_id, person_id, role, source)
);
CREATE TABLE fetch_log(url TEXT PRIMARY KEY, status TEXT NOT NULL);
INSERT INTO movies(movie_id,title,url,synopsis) VALUES('57573','マージン・コール','https://eiga.com/movie/57573/','金融危機を描く社会派サスペンス。');
INSERT INTO people(person_id,name,url,profile_json,biography) VALUES('30003','ケビン・スペイシー','https://eiga.com/person/30003/','{"英語表記":"Kevin Spacey"}','映画俳優。');
INSERT INTO movie_people(movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
VALUES('57573','30003','出演','movie_cast','マージン・コール','ケビン・スペイシー','https://eiga.com/movie/57573/','https://eiga.com/person/30003/');
INSERT INTO fetch_log(url,status) VALUES('https://eiga.com/movie/57573/','ok');
`); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}
	return path
}
