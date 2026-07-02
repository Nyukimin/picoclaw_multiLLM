package viewer

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	HandleGameBridgeResult().ServeHTTP(rec, req)

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
}
