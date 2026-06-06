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

func TestHandleHobbyGraphInteractionCreatesItemInteractionAndObservation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "hobby_graph.sqlite")
	h := HandleHobbyGraphInteraction(HobbyGraphOptions{DBPath: dbPath})
	body := `{
		"category":"manga",
		"item_type":"work",
		"title":"ダンジョン飯",
		"interaction_type":"read",
		"occurred_at":"2026-06-06",
		"source":"manual",
		"source_batch_id":"manual_20260606",
		"rating":5,
		"note":"アニメ版も気になる"
	}`
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodPost, "/viewer/hobby-graph/interaction", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("run %d expected 200, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
		var out hobbyGraphInteractionResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("run %d invalid json: %v", i+1, err)
		}
		if !out.Available || out.DBPath != dbPath {
			t.Fatalf("run %d unexpected response identity: %+v", i+1, out)
		}
		if out.Item.Category != "manga" || out.Item.ItemType != "work" || out.Item.Title != "ダンジョン飯" || out.Item.NormalizedTitle != "ダンジョン飯" {
			t.Fatalf("run %d unexpected item: %+v", i+1, out.Item)
		}
		if out.Interaction.ItemID != out.Item.ItemID || out.Interaction.InteractionType != "read" || out.Interaction.Rating == nil || *out.Interaction.Rating != 5 {
			t.Fatalf("run %d unexpected interaction: %+v", i+1, out.Interaction)
		}
		if out.Observation.Status != "resolved" || out.Observation.ResolvedItemID != out.Item.ItemID {
			t.Fatalf("run %d unexpected observation: %+v", i+1, out.Observation)
		}
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	for table, want := range map[string]int{
		"hobby_items":              1,
		"hobby_interactions":       1,
		"hobby_title_observations": 1,
	} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("table %s count=%d, want %d", table, count, want)
		}
	}
	var itemID, category, interactionType, originalTitle, status, resolvedItemID string
	if err := db.QueryRow(`
SELECT i.item_id, i.category, e.interaction_type, e.original_title, o.status, o.resolved_item_id
FROM hobby_items i
JOIN hobby_interactions e ON e.item_id = i.item_id
JOIN hobby_title_observations o ON o.resolved_item_id = i.item_id
LIMIT 1`).Scan(&itemID, &category, &interactionType, &originalTitle, &status, &resolvedItemID); err != nil {
		t.Fatalf("query saved graph rows: %v", err)
	}
	if itemID == "" || resolvedItemID != itemID || category != "manga" || interactionType != "read" || originalTitle != "ダンジョン飯" || status != "resolved" {
		t.Fatalf("unexpected saved rows item=%q category=%q interaction=%q title=%q status=%q resolved=%q", itemID, category, interactionType, originalTitle, status, resolvedItemID)
	}
}

func TestHandleHobbyGraphInteractionRejectsInvalidRequest(t *testing.T) {
	h := HandleHobbyGraphInteraction(HobbyGraphOptions{DBPath: filepath.Join(t.TempDir(), "hobby_graph.sqlite")})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/viewer/hobby-graph/interaction", strings.NewReader(`{"category":"manga","rating":9}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid hobby graph interaction request") {
		t.Fatalf("unexpected body: %q", rec.Body.String())
	}
}

func TestHandleHobbyGraphInteractionRejectsInvalidMethod(t *testing.T) {
	h := HandleHobbyGraphInteraction(HobbyGraphOptions{DBPath: filepath.Join(t.TempDir(), "hobby_graph.sqlite")})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/viewer/hobby-graph/interaction", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func hobbyGraphTableExists(db *sql.DB, name string) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count)
	return err == nil && count > 0
}
