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

func TestHandleHobbyGraphBootstrapCreatesCommonTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hobby_graph.sqlite")
	bootstrap := HandleHobbyGraphBootstrap(HobbyGraphOptions{DBPath: dbPath})

	req := httptest.NewRequest(http.MethodPost, "/viewer/hobby-graph/bootstrap", nil)
	rec := httptest.NewRecorder()
	bootstrap(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out hobbyGraphResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if !out.Available || out.DBPath != dbPath || out.Action != "bootstrap" {
		t.Fatalf("unexpected bootstrap response: %+v", out)
	}
	for _, table := range hobbyGraphTables {
		if out.Stats[table] != 0 {
			t.Fatalf("expected empty table %s, stats=%+v", table, out.Stats)
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	for _, table := range hobbyGraphTables {
		if !hobbyGraphTableExists(db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}

	status := HandleHobbyGraph(HobbyGraphOptions{DBPath: dbPath})
	statusRec := httptest.NewRecorder()
	status(statusRec, httptest.NewRequest(http.MethodGet, "/viewer/hobby-graph?action=stats", nil))
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", statusRec.Code, statusRec.Body.String())
	}
	var statusOut hobbyGraphResponse
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusOut); err != nil {
		t.Fatalf("invalid status json: %v", err)
	}
	if !statusOut.Available || statusOut.Action != "stats" || len(statusOut.Stats) != len(hobbyGraphTables) {
		t.Fatalf("unexpected status response: %+v", statusOut)
	}
}

func TestHandleHobbyGraphMissingDBIsSoftUnavailable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing.sqlite")
	h := HandleHobbyGraph(HobbyGraphOptions{DBPath: dbPath})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/viewer/hobby-graph?action=stats", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out hobbyGraphResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out.Available || out.DBPath != dbPath || out.Action != "stats" || out.Error != "hobby graph database not found" {
		t.Fatalf("unexpected unavailable response: %+v", out)
	}
}

func TestHandleHobbyGraphRejectsUnsupportedAction(t *testing.T) {
	h := HandleHobbyGraph(HobbyGraphOptions{DBPath: filepath.Join(t.TempDir(), "missing.sqlite")})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/viewer/hobby-graph?action=items", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported action") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestHandleHobbyGraphBootstrapRejectsInvalidMethod(t *testing.T) {
	h := HandleHobbyGraphBootstrap(HobbyGraphOptions{DBPath: filepath.Join(t.TempDir(), "hobby_graph.sqlite")})

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/viewer/hobby-graph/bootstrap", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func hobbyGraphTableExists(db *sql.DB, name string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}
