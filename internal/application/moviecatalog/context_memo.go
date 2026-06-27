package moviecatalog

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const defaultContextMemoLimit = 6

type ContextMemoOptions struct {
	DBPath string
	Topic  string
	Genre  string
	Limit  int
}

type ContextMemoResult struct {
	Available bool
	DBPath    string
	Query     string
	Terms     []ContextMemoTerm
}

type ContextMemoTerm struct {
	Term      string
	Meaning   string
	Relevance string
	Source    string
}

type contextMovieHit struct {
	MovieID  string
	Title    string
	URL      string
	Synopsis string
}

type contextPersonHit struct {
	PersonID string
	Name     string
	URL      string
	Role     string
	Movies   string
}

func ResolveCatalogDBPath(configured string) string {
	candidates := []string{}
	if env := strings.TrimSpace(os.Getenv("PICOCLAW_MOVIE_CATALOG_DB")); env != "" {
		candidates = append(candidates, env)
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates,
		filepath.Join("tmp", "eiga_catalog", "eiga_catalog.sqlite"),
		filepath.Join("tmp", "eiga_catalog_smoke", "eiga_catalog.sqlite"),
	)
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func LookupContextMemo(ctx context.Context, opts ContextMemoOptions) (ContextMemoResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultContextMemoLimit
	}
	dbPath := ResolveCatalogDBPath(opts.DBPath)
	result := ContextMemoResult{
		Available: dbPath != "",
		DBPath:    dbPath,
		Query:     strings.TrimSpace(strings.Join(contextMemoKeywords(opts.Topic, opts.Genre), " ")),
	}
	if dbPath == "" {
		return result, nil
	}
	keywords := contextMemoKeywords(opts.Topic, opts.Genre)
	if len(keywords) == 0 {
		return result, nil
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return result, err
	}
	defer db.Close()

	movies, err := lookupContextMemoMovies(ctx, db, keywords, limit)
	if err != nil {
		return result, err
	}
	people, err := lookupContextMemoPeople(ctx, db, keywords, limit)
	if err != nil {
		return result, err
	}
	linkedPeople, err := lookupContextMemoPeopleForMovies(ctx, db, contextMovieIDs(movies), limit)
	if err != nil {
		return result, err
	}
	people = append(linkedPeople, people...)
	terms := make([]ContextMemoTerm, 0, len(movies)+len(people))
	for _, movie := range movies {
		meaning := "映画DBにある作品"
		if synopsis := compactOneLine(movie.Synopsis, 96); synopsis != "" {
			meaning += "。あらすじ: " + synopsis
		}
		terms = append(terms, ContextMemoTerm{
			Term:      movie.Title,
			Meaning:   meaning,
			Relevance: "今回のお題と語句や雰囲気が近い参考作品。架空映画そのものとは断定せず、場面・葛藤・質感の参考として使う。",
			Source:    "movie_catalog:movie",
		})
	}
	for _, person := range people {
		meaning := "映画DBにある映画関係者"
		if role := strings.TrimSpace(person.Role); role != "" {
			meaning += "。主な役割: " + role
		}
		if movies := compactOneLine(person.Movies, 80); movies != "" {
			meaning += "。関連作: " + movies
		}
		terms = append(terms, ContextMemoTerm{
			Term:      person.Name,
			Meaning:   meaning,
			Relevance: "俳優・監督などの役割や過去作の空気を、架空映画の人物造形や演出の参考にする。",
			Source:    "movie_catalog:person",
		})
	}
	result.Terms = dedupeContextMemoTerms(terms, limit)
	return result, nil
}

func lookupContextMemoMovies(ctx context.Context, db *sql.DB, keywords []string, limit int) ([]contextMovieHit, error) {
	if !catalogTableExists(db, "movies") {
		return nil, nil
	}
	where, args := likeWhere("m.title", "m.synopsis", keywords)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, "%"+keywords[0]+"%", limit)
	rows, err := db.QueryContext(ctx, `
SELECT m.movie_id, m.title, COALESCE(m.url, ''), COALESCE(m.synopsis, '')
FROM movies m
WHERE `+where+`
ORDER BY
  CASE WHEN m.title LIKE ? THEN 0 ELSE 1 END,
  LENGTH(COALESCE(m.synopsis, '')) DESC,
  m.title
LIMIT ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contextMovieHit
	for rows.Next() {
		var hit contextMovieHit
		if err := rows.Scan(&hit.MovieID, &hit.Title, &hit.URL, &hit.Synopsis); err != nil {
			return nil, err
		}
		if strings.TrimSpace(hit.Title) != "" {
			out = append(out, hit)
		}
	}
	return out, rows.Err()
}

func lookupContextMemoPeople(ctx context.Context, db *sql.DB, keywords []string, limit int) ([]contextPersonHit, error) {
	if !catalogTableExists(db, "movie_people") {
		return nil, nil
	}
	peopleTable := catalogTableExists(db, "people")
	nameExpr := "mp.person_name"
	bioExpr := "''"
	if peopleTable {
		nameExpr = "COALESCE(p.name, mp.person_name)"
		bioExpr = "COALESCE(p.biography, '')"
	}
	where, args := likeWhere(nameExpr, bioExpr, keywords)
	movieWhere, movieArgs := likeWhere("mp.movie_title", "mp.role", keywords)
	where = "(" + where + " OR " + movieWhere + ")"
	args = append(args, movieArgs...)
	args = append(args, limit)
	join := ""
	if peopleTable {
		join = "LEFT JOIN people p ON p.person_id = mp.person_id"
	}
	rows, err := db.QueryContext(ctx, `
SELECT mp.person_id,
       COALESCE(`+nameExpr+`, ''),
       COALESCE(MAX(mp.person_url), ''),
       COALESCE(MIN(mp.role), ''),
       COALESCE(GROUP_CONCAT(DISTINCT mp.movie_title), '')
FROM movie_people mp
`+join+`
WHERE `+where+`
GROUP BY mp.person_id, `+nameExpr+`
ORDER BY COUNT(DISTINCT mp.movie_id) DESC, COALESCE(`+nameExpr+`, '')
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contextPersonHit
	for rows.Next() {
		var hit contextPersonHit
		if err := rows.Scan(&hit.PersonID, &hit.Name, &hit.URL, &hit.Role, &hit.Movies); err != nil {
			return nil, err
		}
		if strings.TrimSpace(hit.Name) != "" {
			out = append(out, hit)
		}
	}
	return out, rows.Err()
}

func lookupContextMemoPeopleForMovies(ctx context.Context, db *sql.DB, movieIDs []string, limit int) ([]contextPersonHit, error) {
	if len(movieIDs) == 0 || !catalogTableExists(db, "movie_people") {
		return nil, nil
	}
	placeholders := make([]string, 0, len(movieIDs))
	args := make([]any, 0, len(movieIDs)+1)
	for _, id := range movieIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	if len(placeholders) == 0 {
		return nil, nil
	}
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, `
SELECT person_id,
       COALESCE(person_name, ''),
       COALESCE(MAX(person_url), ''),
       COALESCE(MIN(role), ''),
       COALESCE(GROUP_CONCAT(DISTINCT movie_title), '')
FROM movie_people
WHERE movie_id IN (`+strings.Join(placeholders, ",")+`)
GROUP BY person_id, person_name
ORDER BY
  CASE WHEN MIN(role) IN ('監督', '出演') THEN 0 ELSE 1 END,
  MIN(role),
  person_name
LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contextPersonHit
	for rows.Next() {
		var hit contextPersonHit
		if err := rows.Scan(&hit.PersonID, &hit.Name, &hit.URL, &hit.Role, &hit.Movies); err != nil {
			return nil, err
		}
		if strings.TrimSpace(hit.Name) != "" {
			out = append(out, hit)
		}
	}
	return out, rows.Err()
}

func contextMemoKeywords(topic string, genre string) []string {
	text := strings.TrimSpace(topic)
	text = strings.TrimSuffix(strings.TrimPrefix(text, "「"), "」ってどんな映画？")
	replacer := strings.NewReplacer("「", " ", "」", " ", "『", " ", "』", " ", "ってどんな映画？", " ", "の", " ", "と", " ", "を", " ", "に", " ", "で", " ", "が", " ", "は", " ", "、", " ", "。", " ", "？", " ", "?", " ")
	fields := strings.Fields(replacer.Replace(text + " " + strings.TrimSpace(genre)))
	seen := map[string]struct{}{}
	var out []string
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if utf8.RuneCountInString(field) < 2 {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return utf8.RuneCountInString(out[i]) > utf8.RuneCountInString(out[j])
	})
	if len(out) > 8 {
		return out[:8]
	}
	return out
}

func contextMovieIDs(movies []contextMovieHit) []string {
	out := make([]string, 0, len(movies))
	for _, movie := range movies {
		if id := strings.TrimSpace(movie.MovieID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func likeWhere(columnA string, columnB string, keywords []string) (string, []any) {
	conds := make([]string, 0, len(keywords))
	args := make([]any, 0, len(keywords)*2)
	for _, keyword := range keywords {
		conds = append(conds, fmt.Sprintf("(%s LIKE ? OR %s LIKE ?)", columnA, columnB))
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	if len(conds) == 0 {
		return "1=0", nil
	}
	return strings.Join(conds, " OR "), args
}

func dedupeContextMemoTerms(terms []ContextMemoTerm, limit int) []ContextMemoTerm {
	seen := map[string]struct{}{}
	out := make([]ContextMemoTerm, 0, len(terms))
	for _, term := range terms {
		key := strings.TrimSpace(term.Source) + ":" + strings.TrimSpace(term.Term)
		if strings.TrimSpace(term.Term) == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, term)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func compactOneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:limit])) + "..."
}

func catalogTableExists(db *sql.DB, table string) bool {
	if db == nil || strings.TrimSpace(table) == "" {
		return false
	}
	var n int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&n)
	return err == nil && n > 0
}
