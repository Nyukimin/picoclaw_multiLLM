package moviecatalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestLookupContextMemoReturnsMovieAndPersonTerms(t *testing.T) {
	dbPath := seedContextMemoDB(t)

	result, err := LookupContextMemo(context.Background(), ContextMemoOptions{
		DBPath: dbPath,
		Topic:  "「金融危機の会議室」ってどんな映画？",
		Genre:  "金融",
		Limit:  4,
	})
	if err != nil {
		t.Fatalf("lookup context memo: %v", err)
	}
	if !result.Available {
		t.Fatal("movie catalog should be available")
	}
	if len(result.Terms) == 0 {
		t.Fatal("expected context memo terms")
	}
	joined := ""
	for _, term := range result.Terms {
		joined += term.Term + " " + term.Meaning + " " + term.Relevance + "\n"
	}
	for _, want := range []string{"マージン・コール", "ケビン・スペイシー"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("memo terms do not contain %q:\n%s", want, joined)
		}
	}
}

func TestLookupContextMemoMissingDBIsSoftUnavailable(t *testing.T) {
	result, err := LookupContextMemo(context.Background(), ContextMemoOptions{
		DBPath: filepath.Join(t.TempDir(), "missing.sqlite"),
		Topic:  "「金融危機の会議室」ってどんな映画？",
	})
	if err != nil {
		t.Fatalf("missing DB should not error: %v", err)
	}
	if result.Available {
		t.Fatalf("missing DB should be unavailable: %+v", result)
	}
	if len(result.Terms) != 0 {
		t.Fatalf("missing DB terms = %+v, want none", result.Terms)
	}
}

func seedContextMemoDB(t *testing.T) string {
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
INSERT INTO movies(movie_id,title,url,synopsis)
VALUES('57573','マージン・コール','https://eiga.com/movie/57573/','金融危機の前夜、投資銀行の会議室でリスクをめぐる判断が迫られる。');
INSERT INTO people(person_id,name,url,profile_json,biography)
VALUES('30003','ケビン・スペイシー','https://eiga.com/person/30003/','{}','金融危機を題材にした作品にも出演する俳優。');
INSERT INTO movie_people(movie_id,person_id,role,source,movie_title,person_name,movie_url,person_url)
VALUES('57573','30003','出演','movie_cast','マージン・コール','ケビン・スペイシー','https://eiga.com/movie/57573/','https://eiga.com/person/30003/');
`); err != nil {
		t.Fatalf("seed sqlite: %v", err)
	}
	return path
}
