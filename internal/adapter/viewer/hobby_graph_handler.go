package viewer

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type HobbyGraphOptions struct {
	DBPath string
}

type hobbyGraphResponse struct {
	Available bool           `json:"available"`
	DBPath    string         `json:"db_path"`
	Action    string         `json:"action"`
	Stats     map[string]int `json:"stats,omitempty"`
	Error     string         `json:"error,omitempty"`
}

var hobbyGraphTables = []string{
	"hobby_items",
	"hobby_relations",
	"hobby_interactions",
	"hobby_title_observations",
	"hobby_preference_signals",
	"hobby_topic_candidates",
	"hobby_collection_runs",
	"hobby_collection_targets",
}

func HandleHobbyGraph(opts HobbyGraphOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		action := strings.TrimSpace(r.URL.Query().Get("action"))
		if action == "" {
			action = "stats"
		}
		if action != "stats" {
			http.Error(w, "unsupported action", http.StatusBadRequest)
			return
		}
		dbPath := resolveHobbyGraphDBPath(opts.DBPath)
		if dbPath == "" {
			writeHobbyGraphJSON(w, hobbyGraphResponse{
				Available: false,
				DBPath:    strings.TrimSpace(opts.DBPath),
				Action:    action,
				Error:     "hobby graph database not found",
			})
			return
		}
		db, err := sql.Open("sqlite3", "file:"+dbPath+"?mode=ro")
		if err != nil {
			http.Error(w, "failed to open hobby graph", http.StatusInternalServerError)
			return
		}
		defer db.Close()
		stats, err := hobbyGraphStats(db)
		if err != nil {
			http.Error(w, "failed to load hobby graph", http.StatusInternalServerError)
			return
		}
		writeHobbyGraphJSON(w, hobbyGraphResponse{
			Available: true,
			DBPath:    dbPath,
			Action:    action,
			Stats:     stats,
		})
	}
}

func HandleHobbyGraphBootstrap(opts HobbyGraphOptions) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dbPath := resolveHobbyGraphWritableDBPath(opts.DBPath)
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			http.Error(w, "failed to create hobby graph directory", http.StatusInternalServerError)
			return
		}
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			http.Error(w, "failed to open hobby graph", http.StatusInternalServerError)
			return
		}
		defer db.Close()
		if err := ensureHobbyGraphTables(r.Context(), db); err != nil {
			http.Error(w, "failed to bootstrap hobby graph", http.StatusInternalServerError)
			return
		}
		stats, err := hobbyGraphStats(db)
		if err != nil {
			http.Error(w, "failed to load hobby graph", http.StatusInternalServerError)
			return
		}
		writeHobbyGraphJSON(w, hobbyGraphResponse{
			Available: true,
			DBPath:    dbPath,
			Action:    "bootstrap",
			Stats:     stats,
		})
	}
}

func resolveHobbyGraphDBPath(configured string) string {
	candidates := []string{}
	if env := strings.TrimSpace(os.Getenv("PICOCLAW_HOBBY_GRAPH_DB")); env != "" {
		candidates = append(candidates, env)
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, filepath.Join("tmp", "hobby_graph", "hobby_graph.sqlite"))
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

func resolveHobbyGraphWritableDBPath(configured string) string {
	if resolved := resolveHobbyGraphDBPath(configured); resolved != "" {
		return resolved
	}
	if env := strings.TrimSpace(os.Getenv("PICOCLAW_HOBBY_GRAPH_DB")); env != "" {
		return env
	}
	if configured = strings.TrimSpace(configured); configured != "" {
		return configured
	}
	return filepath.Join("tmp", "hobby_graph", "hobby_graph.sqlite")
}

func ensureHobbyGraphTables(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS hobby_items (
  item_id TEXT PRIMARY KEY,
  category TEXT NOT NULL,
  item_type TEXT NOT NULL,
  title TEXT NOT NULL,
  normalized_title TEXT NOT NULL,
  subtitle TEXT,
  canonical_source TEXT,
  canonical_url TEXT,
  external_ids_json TEXT NOT NULL DEFAULT '{}',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS hobby_relations (
  relation_id TEXT PRIMARY KEY,
  from_item_id TEXT NOT NULL,
  to_item_id TEXT NOT NULL,
  relation_type TEXT NOT NULL,
  source TEXT NOT NULL,
  evidence_url TEXT,
  evidence_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS hobby_interactions (
  interaction_id TEXT PRIMARY KEY,
  item_id TEXT,
  category TEXT NOT NULL,
  interaction_type TEXT NOT NULL,
  original_title TEXT NOT NULL,
  occurred_at TEXT,
  source TEXT NOT NULL,
  source_batch_id TEXT,
  rating REAL,
  note TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS hobby_title_observations (
  observation_id TEXT PRIMARY KEY,
  category TEXT NOT NULL,
  original_title TEXT NOT NULL,
  normalized_title TEXT NOT NULL,
  source TEXT NOT NULL,
  source_batch_id TEXT,
  status TEXT NOT NULL DEFAULT 'unresolved',
  resolved_item_id TEXT,
  resolution_note TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  resolved_at TEXT
);
CREATE TABLE IF NOT EXISTS hobby_preference_signals (
  signal_id TEXT PRIMARY KEY,
  category TEXT,
  signal_type TEXT NOT NULL,
  target_item_id TEXT,
  target_label TEXT NOT NULL,
  weight REAL NOT NULL DEFAULT 1.0,
  evidence_json TEXT NOT NULL,
  generated_by TEXT NOT NULL,
  generated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS hobby_topic_candidates (
  candidate_id TEXT PRIMARY KEY,
  category TEXT,
  topic_type TEXT NOT NULL,
  target_item_id TEXT,
  title TEXT NOT NULL,
  reason TEXT NOT NULL,
  evidence_json TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'candidate',
  generated_by TEXT NOT NULL,
  generated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  used_at TEXT
);
CREATE TABLE IF NOT EXISTS hobby_collection_runs (
  run_id TEXT PRIMARY KEY,
  category TEXT,
  reason TEXT NOT NULL,
  trigger_source TEXT NOT NULL,
  trigger_id TEXT,
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  summary TEXT
);
CREATE TABLE IF NOT EXISTS hobby_collection_targets (
  run_id TEXT NOT NULL,
  target_url TEXT NOT NULL,
  target_kind TEXT NOT NULL,
  target_id TEXT,
  reason TEXT NOT NULL,
  parent_kind TEXT,
  parent_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  fetched_at TEXT,
  error TEXT,
  PRIMARY KEY(run_id, target_url)
)`)
	return err
}

func hobbyGraphStats(db *sql.DB) (map[string]int, error) {
	out := map[string]int{}
	for _, table := range hobbyGraphTables {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
			return nil, err
		}
		out[table] = n
	}
	return out, nil
}

func writeHobbyGraphJSON(w http.ResponseWriter, payload hobbyGraphResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
