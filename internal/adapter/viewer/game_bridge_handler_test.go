package viewer

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestHandleGameBridgeStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/viewer/games/status", nil)

	HandleGameBridgeStatus(GameBridgeStatusOptions{
		ConversationEngineEnabled: true,
		L1StoreEnabled:            true,
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["decision_mode"] != "deterministic_stub" {
		t.Fatalf("decision_mode=%v", got["decision_mode"])
	}
	if got["conversation_engine_enabled"] != true || got["l1_store_enabled"] != true {
		t.Fatalf("runtime flags not reflected: %+v", got)
	}
}

func TestHandleGameBridgeDecisionRejectsInvalidRequest(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/decision", bytes.NewBufferString(`{"game_id":"survival_garden"}`))

	HandleGameBridgeDecision().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleGameBridgeDecisionReturnsAvailableAction(t *testing.T) {
	body := map[string]any{
		"game_id":    "survival_garden",
		"session_id": "sg_test",
		"turn":       12,
		"persona":    "mio",
		"observation": map[string]any{
			"time": "day_3_evening",
			"status": map[string]any{
				"hunger":  62,
				"thirst":  28,
				"fatigue": 71,
			},
			"visible_events": []string{"fish_seen", "rain_clouds"},
		},
		"available_actions": []string{"fish", "drink", "return_to_camp", "rest"},
		"request":           "choose_next_action",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/decision", bytes.NewReader(payload))

	HandleGameBridgeDecision().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got GameBrainDecision
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Intent != "return_to_camp" {
		t.Fatalf("intent=%q want return_to_camp", got.Intent)
	}
	for _, step := range got.ActionPlan {
		switch step.Action {
		case "fish", "drink", "return_to_camp", "rest":
		default:
			t.Fatalf("unavailable action in plan: %q", step.Action)
		}
	}
}

func TestGameActionStepUsesCommonArgsKey(t *testing.T) {
	payload, err := json.Marshal(GameActionStep{
		Action: "move",
		Target: "river",
		Args:   map[string]any{"pace": "safe"},
	})
	if err != nil {
		t.Fatalf("marshal action step: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode action step: %v", err)
	}
	if _, ok := got["args"]; !ok {
		t.Fatalf("missing args key: %s", string(payload))
	}
	if _, ok := got["parameters"]; ok {
		t.Fatalf("unexpected parameters key: %s", string(payload))
	}
}

func TestHandleGameBridgeResultAcceptsCandidateResult(t *testing.T) {
	store := NewGameBridgeStore(filepath.Join(t.TempDir(), "game_bridge_events.jsonl"))
	body := map[string]any{
		"game_id":          "survival_garden",
		"session_id":       "sg_test",
		"turn":             2,
		"persona":          "mio",
		"executed_actions": []string{"drink", "return_to_camp"},
		"result": map[string]any{
			"success": true,
			"event":   "returned_before_rain",
		},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/result", bytes.NewReader(payload))

	HandleGameBridgeResult(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got["memory_state"] != "candidate" {
		t.Fatalf("memory_state=%v", got["memory_state"])
	}
	if got["event_id"] == "" {
		t.Fatalf("event_id is empty")
	}
	events, err := store.RecentGameBridgeEvents(context.Background(), "survival_garden", "sg_test", 10)
	if err != nil {
		t.Fatalf("RecentGameBridgeEvents returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%d want 1", len(events))
	}
	if events[0].MemoryState != "candidate" || events[0].Promoted {
		t.Fatalf("unexpected persisted event: %+v", events[0])
	}
}

func TestHandleGameBridgeDecisionIncludesRecentCandidateMemoryRefs(t *testing.T) {
	store := NewGameBridgeStore(filepath.Join(t.TempDir(), "game_bridge_events.jsonl"))
	_, err := store.SaveGameBridgeResult(context.Background(), GameResultRequest{
		GameID:          "survival_garden",
		SessionID:       "sg_test",
		Turn:            1,
		Persona:         "mio",
		ExecutedActions: []string{"drink"},
		Result:          map[string]any{"success": true, "event": "drank_water"},
	})
	if err != nil {
		t.Fatalf("SaveGameBridgeResult returned error: %v", err)
	}

	body := map[string]any{
		"game_id":    "survival_garden",
		"session_id": "sg_test",
		"turn":       2,
		"persona":    "mio",
		"observation": map[string]any{
			"time":           "day_1_day",
			"status":         map[string]any{"hunger": 10, "thirst": 10, "fatigue": 10},
			"visible_events": []string{},
		},
		"available_actions": []string{"drink", "rest"},
		"request":           "choose_next_action",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/decision", bytes.NewReader(payload))

	HandleGameBridgeDecision(GameBridgeDecisionOptions{RecallReader: store}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got GameBrainDecision
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.MemoryRefs) != 1 {
		t.Fatalf("memory_refs=%v want one candidate ref", got.MemoryRefs)
	}
	if got.MemoryRefs[0] != "game:survival_garden:sg_test:turn_1:candidate" {
		t.Fatalf("memory_refs=%v", got.MemoryRefs)
	}
}

func TestHandleGameBridgeDecisionUsesGeneratorAndMergesRecallRefs(t *testing.T) {
	store := NewGameBridgeStore(filepath.Join(t.TempDir(), "game_bridge_events.jsonl"))
	_, err := store.SaveGameBridgeResult(context.Background(), GameResultRequest{
		GameID:          "survival_garden",
		SessionID:       "sg_test",
		Turn:            1,
		Persona:         "mio",
		ExecutedActions: []string{"drink"},
		Result:          map[string]any{"success": true, "event": "drank_water"},
	})
	if err != nil {
		t.Fatalf("SaveGameBridgeResult returned error: %v", err)
	}
	generator := gameDecisionGeneratorFunc(func(_ *http.Request, req GameObservationRequest, recent []GameBridgeEvent) (GameBrainDecision, error) {
		if req.SessionID != "sg_test" {
			t.Fatalf("SessionID=%q", req.SessionID)
		}
		if len(recent) != 1 {
			t.Fatalf("recent events=%d want 1", len(recent))
		}
		return GameBrainDecision{
			Persona:    "mio",
			Intent:     "drink",
			Reason:     "water is useful",
			ActionPlan: []GameActionStep{{Action: "drink"}},
			MemoryRefs: []string{"custom_ref"},
			Confidence: 0.8,
		}, nil
	})
	body := map[string]any{
		"game_id":           "survival_garden",
		"session_id":        "sg_test",
		"turn":              2,
		"persona":           "mio",
		"observation":       map[string]any{"status": map[string]any{"thirst": 10}},
		"available_actions": []string{"drink", "rest"},
		"request":           "choose_next_action",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/decision", bytes.NewReader(payload))

	HandleGameBridgeDecision(GameBridgeDecisionOptions{
		RecallReader: store,
		Generator:    generator,
	}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got GameBrainDecision
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantRefs := []string{"custom_ref", "game:survival_garden:sg_test:turn_1:candidate"}
	if len(got.MemoryRefs) != len(wantRefs) {
		t.Fatalf("memory_refs=%v want %v", got.MemoryRefs, wantRefs)
	}
	for i, want := range wantRefs {
		if got.MemoryRefs[i] != want {
			t.Fatalf("memory_refs=%v want %v", got.MemoryRefs, wantRefs)
		}
	}
}

func TestHandleGameBridgeDecisionRejectsInvalidGeneratedAction(t *testing.T) {
	generator := gameDecisionGeneratorFunc(func(_ *http.Request, _ GameObservationRequest, _ []GameBridgeEvent) (GameBrainDecision, error) {
		return GameBrainDecision{
			Persona:    "mio",
			Intent:     "fly",
			Reason:     "invalid action",
			ActionPlan: []GameActionStep{{Action: "fly"}},
			Confidence: 0.7,
		}, nil
	})
	body := map[string]any{
		"game_id":           "survival_garden",
		"session_id":        "sg_test",
		"turn":              2,
		"persona":           "mio",
		"observation":       map[string]any{"status": map[string]any{"thirst": 10}},
		"available_actions": []string{"drink", "rest"},
		"request":           "choose_next_action",
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/viewer/games/decision", bytes.NewReader(payload))

	HandleGameBridgeDecision(GameBridgeDecisionOptions{Generator: generator}).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
}

type gameDecisionGeneratorFunc func(*http.Request, GameObservationRequest, []GameBridgeEvent) (GameBrainDecision, error)

func (f gameDecisionGeneratorFunc) GenerateGameDecision(r *http.Request, req GameObservationRequest, recentEvents []GameBridgeEvent) (GameBrainDecision, error) {
	return f(r, req, recentEvents)
}
