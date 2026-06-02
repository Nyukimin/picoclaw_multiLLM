package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const defaultMovieCatalogLimit = 25
const maxMovieCatalogLimit = 50
const maxMovieCatalogFetchPages = 20

type MovieCatalogOptions struct {
	DBPath string
}

type movieCatalogResponse struct {
	Available bool   `json:"available"`
	DBPath    string `json:"db_path"`
	Action    string `json:"action"`
	Total     int    `json:"total,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
	Items     any    `json:"items,omitempty"`
	Detail    any    `json:"detail,omitempty"`
	Stats     any    `json:"stats,omitempty"`
	Error     string `json:"error,omitempty"`
}

type movieCatalogMovieItem struct {
	MovieID     string `json:"movie_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Synopsis    string `json:"synopsis"`
	PeopleCount int    `json:"people_count"`
}

type movieCatalogPersonItem struct {
	PersonID   string `json:"person_id"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Profile    string `json:"profile"`
	Biography  string `json:"biography"`
	MovieCount int    `json:"movie_count"`
}

type movieCatalogEdgeItem struct {
	MovieID       string `json:"movie_id"`
	MovieTitle    string `json:"movie_title"`
	MovieURL      string `json:"movie_url"`
	MovieFetched  bool   `json:"movie_fetched"`
	PersonID      string `json:"person_id"`
	PersonName    string `json:"person_name"`
	PersonURL     string `json:"person_url"`
	PersonFetched bool   `json:"person_fetched"`
	Role          string `json:"role"`
	Source        string `json:"source"`
}

type movieCatalogFetchRequest struct {
	Kind                     string `json:"kind"`
	Query                    string `json:"query"`
	URL                      string `json:"url"`
	MaxPages                 int    `json:"max_pages"`
	FollowLinks              bool   `json:"follow_links"`
	IncludePersonFilmography bool   `json:"include_person_filmography"`
}

type movieCatalogFetchCandidate struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type movieCatalogFetchResponse struct {
	Available  bool                         `json:"available"`
	DBPath     string                       `json:"db_path"`
	Status     string                       `json:"status"`
	Kind       string                       `json:"kind,omitempty"`
	Query      string                       `json:"query,omitempty"`
	URL        string                       `json:"url,omitempty"`
	Command    []string                     `json:"command,omitempty"`
	Stdout     string                       `json:"stdout,omitempty"`
	Stderr     string                       `json:"stderr,omitempty"`
	Candidates []movieCatalogFetchCandidate `json:"candidates,omitempty"`
	Error      string                       `json:"error,omitempty"`
}

func HandleMovieCatalog(opts MovieCatalogOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dbPath := resolveMovieCatalogDBPath(opts.DBPath)
		if dbPath == "" {
			writeMovieCatalogJSON(w, movieCatalogResponse{
				Available: false,
				DBPath:    strings.TrimSpace(opts.DBPath),
				Action:    actionOrDefault(r.URL.Query().Get("action")),
				Error:     "movie catalog database not found",
			})
			return
		}
		db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
		if err != nil {
			http.Error(w, "failed to open movie catalog", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		action := actionOrDefault(r.URL.Query().Get("action"))
		limit, offset, err := movieCatalogPageParams(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		resp := movieCatalogResponse{Available: true, DBPath: dbPath, Action: action, Limit: limit, Offset: offset}
		switch action {
		case "stats":
			resp.Stats, err = movieCatalogStats(db)
		case "movies":
			resp.Total, resp.Items, err = movieCatalogMovies(db, r, limit, offset)
		case "people":
			resp.Total, resp.Items, err = movieCatalogPeople(db, r, limit, offset)
		case "movie":
			resp.Detail, err = movieCatalogMovieDetail(db, r.URL.Query().Get("id"))
		case "person":
			resp.Detail, err = movieCatalogPersonDetail(db, r.URL.Query().Get("id"))
		default:
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
		if err != nil {
			http.Error(w, "failed to load movie catalog", http.StatusInternalServerError)
			return
		}
		writeMovieCatalogJSON(w, resp)
	}
}

func HandleMovieCatalogFetch(opts MovieCatalogOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req movieCatalogFetchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		req.Kind = normalizeMovieCatalogKind(req.Kind)
		if req.Kind == "" {
			http.Error(w, "kind must be movie or person", http.StatusBadRequest)
			return
		}
		maxPages := req.MaxPages
		if maxPages <= 0 {
			maxPages = 5
		}
		if maxPages > maxMovieCatalogFetchPages {
			maxPages = maxMovieCatalogFetchPages
		}

		dbPath := resolveMovieCatalogWritableDBPath(opts.DBPath)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			http.Error(w, "failed to create movie catalog directory", http.StatusInternalServerError)
			return
		}
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			http.Error(w, "failed to open movie catalog", http.StatusInternalServerError)
			return
		}
		defer db.Close()

		targetURL, candidates, err := resolveMovieCatalogFetchTarget(db, req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errMovieCatalogFetchCandidates) {
				status = http.StatusConflict
			}
			log.Printf("[MovieCatalog] fetch target unresolved kind=%s query=%q candidates=%d err=%v", req.Kind, strings.TrimSpace(req.Query), len(candidates), err)
			writeMovieCatalogFetchJSONStatus(w, status, movieCatalogFetchResponse{
				Available:  true,
				DBPath:     dbPath,
				Status:     "candidates",
				Kind:       req.Kind,
				Query:      strings.TrimSpace(req.Query),
				Candidates: candidates,
				Error:      err.Error(),
			})
			return
		}
		if targetURL == "" {
			http.Error(w, "url or name query is required", http.StatusBadRequest)
			return
		}

		cmdArgs := movieCatalogFetchCommandArgs(targetURL, dbPath, maxPages, req.FollowLinks, req.IncludePersonFilmography)
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(maxPages*8+20)*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "python3", cmdArgs...)
		cmd.Dir = "."
		out, runErr := cmd.CombinedOutput()
		resp := movieCatalogFetchResponse{
			Available: true,
			DBPath:    dbPath,
			URL:       targetURL,
			Command:   append([]string{"python3"}, cmdArgs...),
			Stdout:    string(out),
			Status:    "ok",
		}
		if runErr != nil {
			resp.Status = "error"
			resp.Error = runErr.Error()
			log.Printf("[MovieCatalog] fetch failed url=%s max_pages=%d err=%v output=%s", targetURL, maxPages, runErr, strings.TrimSpace(string(out)))
			writeMovieCatalogFetchJSONStatus(w, http.StatusInternalServerError, resp)
			return
		}
		writeMovieCatalogFetchJSON(w, resp)
	}
}

func resolveMovieCatalogDBPath(configured string) string {
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

func resolveMovieCatalogWritableDBPath(configured string) string {
	if resolved := resolveMovieCatalogDBPath(configured); resolved != "" {
		return resolved
	}
	if env := strings.TrimSpace(os.Getenv("PICOCLAW_MOVIE_CATALOG_DB")); env != "" {
		return env
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	return filepath.Join("tmp", "eiga_catalog", "eiga_catalog.sqlite")
}

var (
	errMovieCatalogFetchCandidates = errors.New("local candidates are ambiguous or unavailable; choose a candidate or paste a movie/person URL")
	movieCatalogURLPattern         = regexp.MustCompile(`^https://eiga\.com/(movie|person)/(\d+)/?$`)
)

func normalizeMovieCatalogKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movie", "movies", "映画":
		return "movie"
	case "person", "people", "人物":
		return "person"
	default:
		return ""
	}
}

func resolveMovieCatalogFetchTarget(db *sql.DB, req movieCatalogFetchRequest) (string, []movieCatalogFetchCandidate, error) {
	if rawURL := strings.TrimSpace(req.URL); rawURL != "" {
		url := normalizeMovieCatalogFetchURL(rawURL)
		if url == "" {
			return "", nil, fmt.Errorf("only https://eiga.com/movie/{id}/ or https://eiga.com/person/{id}/ URLs are supported")
		}
		if !strings.Contains(url, "/"+req.Kind+"/") {
			return "", nil, fmt.Errorf("url kind does not match selected kind")
		}
		return url, nil, nil
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return "", nil, fmt.Errorf("url or name query is required")
	}
	candidates, err := movieCatalogFetchCandidates(db, req.Kind, query, 10)
	if err != nil {
		return "", nil, err
	}
	if len(candidates) != 1 {
		return "", candidates, errMovieCatalogFetchCandidates
	}
	return candidates[0].URL, candidates, nil
}

func normalizeMovieCatalogFetchURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "http://eiga.com/") {
		raw = "https://" + strings.TrimPrefix(raw, "http://")
	}
	match := movieCatalogURLPattern.FindStringSubmatch(raw)
	if match == nil {
		return ""
	}
	return fmt.Sprintf("https://eiga.com/%s/%s/", match[1], match[2])
}

func movieCatalogFetchCandidates(db *sql.DB, kind string, query string, limit int) ([]movieCatalogFetchCandidate, error) {
	like := "%" + query + "%"
	if kind == "movie" {
		rows, err := db.Query(`
SELECT movie_id, title, url
FROM (
  SELECT movie_id, title, url, MAX(fetched) AS fetched
  FROM (
    SELECT movie_id, title, url, 1 AS fetched FROM movies WHERE title LIKE ?
    UNION ALL
    SELECT movie_id, movie_title AS title, movie_url AS url, 0 AS fetched
    FROM movie_people
    WHERE movie_title LIKE ? AND COALESCE(movie_url, '') != ''
  )
  GROUP BY movie_id, title, url
)
ORDER BY CASE WHEN title = ? THEN 0 ELSE 1 END, fetched DESC, title
LIMIT ?`, like, like, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []movieCatalogFetchCandidate{}
		for rows.Next() {
			var item movieCatalogFetchCandidate
			item.Kind = "movie"
			if err := rows.Scan(&item.ID, &item.Title, &item.URL); err != nil {
				return nil, err
			}
			out = append(out, item)
		}
		return out, rows.Err()
	}
	rows, err := db.Query(`
SELECT person_id, name, url
FROM (
  SELECT person_id, name, url, MAX(fetched) AS fetched
  FROM (
    SELECT person_id, name, url, 1 AS fetched FROM people WHERE name LIKE ?
    UNION ALL
    SELECT person_id, person_name AS name, person_url AS url, 0 AS fetched
    FROM movie_people
    WHERE person_name LIKE ? AND COALESCE(person_url, '') != ''
  )
  GROUP BY person_id, name, url
)
ORDER BY CASE WHEN name = ? THEN 0 ELSE 1 END, fetched DESC, name
LIMIT ?`, like, like, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []movieCatalogFetchCandidate{}
	for rows.Next() {
		var item movieCatalogFetchCandidate
		item.Kind = "person"
		if err := rows.Scan(&item.ID, &item.Name, &item.URL); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func movieCatalogFetchCommandArgs(targetURL string, dbPath string, maxPages int, followLinks bool, includeFilmography bool) []string {
	outDir := filepath.Dir(dbPath)
	jsonlPath := filepath.Join(outDir, "eiga_catalog.jsonl")
	args := []string{
		filepath.Join("tools", "eiga_catalog", "eiga_catalog.py"),
		"--seed-url", targetURL,
		"--max-pages", strconv.Itoa(maxPages),
		"--delay", "2",
		"--db", dbPath,
		"--jsonl", jsonlPath,
	}
	if followLinks {
		args = append(args, "--follow-links")
	}
	if includeFilmography {
		args = append(args, "--include-person-filmography")
	}
	return args
}

func actionOrDefault(action string) string {
	action = strings.TrimSpace(action)
	if action == "" {
		return "movies"
	}
	return action
}

func movieCatalogPageParams(r *http.Request) (int, int, error) {
	limit := defaultMovieCatalogLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return 0, 0, fmt.Errorf("invalid limit")
		}
		if n > maxMovieCatalogLimit {
			n = maxMovieCatalogLimit
		}
		limit = n
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return 0, 0, fmt.Errorf("invalid offset")
		}
		offset = n
	}
	return limit, offset, nil
}

func movieCatalogStats(db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	for _, table := range []string{"movies", "people", "movie_people", "fetch_log"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return nil, err
		}
		out[table] = n
	}
	return out, nil
}

func movieCatalogMovies(db *sql.DB, r *http.Request, limit int, offset int) (int, []movieCatalogMovieItem, error) {
	where, args := movieCatalogMovieWhere(r)
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM movies m "+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	args = append(args, limit, offset)
	rows, err := db.Query(`
SELECT m.movie_id, m.title, m.url, COALESCE(m.synopsis, ''),
       COUNT(DISTINCT mp.person_id) AS people_count
FROM movies m
LEFT JOIN movie_people mp ON mp.movie_id = m.movie_id
`+where+`
GROUP BY m.movie_id, m.title, m.url, m.synopsis
ORDER BY m.title
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []movieCatalogMovieItem{}
	for rows.Next() {
		var item movieCatalogMovieItem
		if err := rows.Scan(&item.MovieID, &item.Title, &item.URL, &item.Synopsis, &item.PeopleCount); err != nil {
			return 0, nil, err
		}
		items = append(items, item)
	}
	return total, items, rows.Err()
}

func movieCatalogMovieWhere(r *http.Request) (string, []any) {
	conds := []string{}
	args := []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		like := "%" + q + "%"
		conds = append(conds, `(m.title LIKE ? OR m.synopsis LIKE ? OR EXISTS (
SELECT 1 FROM movie_people qmp WHERE qmp.movie_id = m.movie_id AND qmp.person_name LIKE ?
))`)
		args = append(args, like, like, like)
	}
	if role := strings.TrimSpace(r.URL.Query().Get("role")); role != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM movie_people rmp WHERE rmp.movie_id = m.movie_id AND rmp.role = ?)")
		args = append(args, role)
	}
	if source := strings.TrimSpace(r.URL.Query().Get("source")); source != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM movie_people smp WHERE smp.movie_id = m.movie_id AND smp.source = ?)")
		args = append(args, source)
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

func movieCatalogPeople(db *sql.DB, r *http.Request, limit int, offset int) (int, []movieCatalogPersonItem, error) {
	where, args := movieCatalogPeopleWhere(r)
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM people p "+where, args...).Scan(&total); err != nil {
		return 0, nil, err
	}
	args = append(args, limit, offset)
	rows, err := db.Query(`
SELECT p.person_id, p.name, p.url, COALESCE(p.profile_json, ''), COALESCE(p.biography, ''),
       COUNT(DISTINCT mp.movie_id) AS movie_count
FROM people p
LEFT JOIN movie_people mp ON mp.person_id = p.person_id
`+where+`
GROUP BY p.person_id, p.name, p.url, p.profile_json, p.biography
ORDER BY p.name
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	items := []movieCatalogPersonItem{}
	for rows.Next() {
		var item movieCatalogPersonItem
		if err := rows.Scan(&item.PersonID, &item.Name, &item.URL, &item.Profile, &item.Biography, &item.MovieCount); err != nil {
			return 0, nil, err
		}
		items = append(items, item)
	}
	return total, items, rows.Err()
}

func movieCatalogPeopleWhere(r *http.Request) (string, []any) {
	conds := []string{}
	args := []any{}
	if q := strings.TrimSpace(r.URL.Query().Get("q")); q != "" {
		like := "%" + q + "%"
		conds = append(conds, `(p.name LIKE ? OR p.biography LIKE ? OR EXISTS (
SELECT 1 FROM movie_people qmp WHERE qmp.person_id = p.person_id AND qmp.movie_title LIKE ?
))`)
		args = append(args, like, like, like)
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

func movieCatalogMovieDetail(db *sql.DB, id string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("movie id is required")
	}
	var movie movieCatalogMovieItem
	if err := db.QueryRow("SELECT movie_id, title, url, COALESCE(synopsis, '') FROM movies WHERE movie_id = ?", id).Scan(&movie.MovieID, &movie.Title, &movie.URL, &movie.Synopsis); err != nil {
		return nil, err
	}
	edges, err := movieCatalogEdges(db, "movie_id", id, 200)
	if err != nil {
		return nil, err
	}
	return map[string]any{"movie": movie, "links": edges}, nil
}

func movieCatalogPersonDetail(db *sql.DB, id string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("person id is required")
	}
	var person movieCatalogPersonItem
	if err := db.QueryRow("SELECT person_id, name, url, COALESCE(profile_json, ''), COALESCE(biography, '') FROM people WHERE person_id = ?", id).Scan(&person.PersonID, &person.Name, &person.URL, &person.Profile, &person.Biography); err != nil {
		return nil, err
	}
	edges, err := movieCatalogEdges(db, "person_id", id, 200)
	if err != nil {
		return nil, err
	}
	person.MovieCount = len(edges)
	return map[string]any{"person": person, "links": edges}, nil
}

func movieCatalogEdges(db *sql.DB, column string, id string, limit int) ([]movieCatalogEdgeItem, error) {
	if column != "movie_id" && column != "person_id" {
		return nil, fmt.Errorf("unsupported edge column")
	}
	rows, err := db.Query(`
SELECT movie_id, COALESCE(movie_title, ''), COALESCE(movie_url, ''),
       person_id, COALESCE(person_name, ''), COALESCE(person_url, ''),
       EXISTS(SELECT 1 FROM movies m WHERE m.movie_id = movie_people.movie_id),
       EXISTS(SELECT 1 FROM people p WHERE p.person_id = movie_people.person_id),
       role, source
FROM movie_people
WHERE `+column+` = ?
ORDER BY source, role, movie_title, person_name
LIMIT ?`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []movieCatalogEdgeItem{}
	for rows.Next() {
		var item movieCatalogEdgeItem
		if err := rows.Scan(&item.MovieID, &item.MovieTitle, &item.MovieURL, &item.PersonID, &item.PersonName, &item.PersonURL, &item.MovieFetched, &item.PersonFetched, &item.Role, &item.Source); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func writeMovieCatalogJSON(w http.ResponseWriter, payload movieCatalogResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMovieCatalogFetchJSON(w http.ResponseWriter, payload movieCatalogFetchResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

func writeMovieCatalogFetchJSONStatus(w http.ResponseWriter, status int, payload movieCatalogFetchResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
