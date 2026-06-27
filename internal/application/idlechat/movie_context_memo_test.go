package idlechat

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	_ "github.com/mattn/go-sqlite3"
)

func TestEnrichTopicContextAttachesMovieCatalogMemo(t *testing.T) {
	dbPath := seedIdleMovieContextDB(t)
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil)
	o.SetMovieCatalogDBPath(dbPath)

	result := o.enrichTopicContext(context.Background(), TopicGenerationResult{
		Topic:    "「金融危機の会議室」ってどんな映画？",
		Category: TopicCategoryMovie,
		Strategy: string(StrategyMovie),
		Seed: TopicSeed{
			Category: TopicCategoryMovie,
			Genre1:   "金融",
		},
	})
	if len(result.ContextTerms) == 0 {
		t.Fatal("expected movie context terms")
	}
	contextText := formatTopicGenerationContext(result)
	for _, want := range []string{"【関連メモ】", "マージン・コール", "ケビン・スペイシー", "発話本文へ出さない"} {
		if !strings.Contains(contextText, want) {
			t.Fatalf("session context missing %q:\n%s", want, contextText)
		}
	}
}

func TestEnrichTopicContextSkipsNonMovie(t *testing.T) {
	o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.7, nil)
	result := o.enrichTopicContext(context.Background(), TopicGenerationResult{
		Topic:    "金融危機の会議室",
		Category: TopicCategorySingle,
		Strategy: string(StrategySingleGenre),
	})
	if len(result.ContextTerms) != 0 {
		t.Fatalf("non-movie should not get movie context terms: %+v", result.ContextTerms)
	}
}

func seedIdleMovieContextDB(t *testing.T) string {
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
