package viewer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type GameBridgeEvent struct {
	EventID           string            `json:"event_id"`
	CandidateMemoryID string            `json:"candidate_memory_id"`
	GameID            string            `json:"game_id"`
	SessionID         string            `json:"session_id"`
	Turn              int               `json:"turn"`
	Persona           string            `json:"persona"`
	Decision          GameBrainDecision `json:"decision"`
	ExecutedActions   []string          `json:"executed_actions"`
	Result            map[string]any    `json:"result"`
	MemoryState       string            `json:"memory_state"`
	Promoted          bool              `json:"promoted"`
	CreatedAt         string            `json:"created_at"`
}

type GameBridgeResultWriter interface {
	SaveGameBridgeResult(context.Context, GameResultRequest) (GameBridgeEvent, error)
}

type GameBridgeRecallReader interface {
	RecentGameBridgeEvents(context.Context, string, string, int) ([]GameBridgeEvent, error)
}

type GameBridgeStore struct {
	path string
	mu   sync.Mutex
}

func NewGameBridgeStore(path string) *GameBridgeStore {
	return &GameBridgeStore{path: strings.TrimSpace(path)}
}

func (s *GameBridgeStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *GameBridgeStore) SaveGameBridgeResult(_ context.Context, req GameResultRequest) (GameBridgeEvent, error) {
	if s == nil || s.path == "" {
		return GameBridgeEvent{}, fmt.Errorf("game bridge store path is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	eventID := gameBridgeEventID(req.GameID, req.SessionID, req.Turn)
	if existing, ok, err := s.findByEventIDLocked(eventID); err != nil {
		return GameBridgeEvent{}, err
	} else if ok {
		return existing, nil
	}

	event := GameBridgeEvent{
		EventID:           eventID,
		CandidateMemoryID: eventID + ":candidate",
		GameID:            strings.TrimSpace(req.GameID),
		SessionID:         strings.TrimSpace(req.SessionID),
		Turn:              req.Turn,
		Persona:           strings.TrimSpace(req.Persona),
		Decision:          req.Decision,
		ExecutedActions:   append([]string(nil), req.ExecutedActions...),
		Result:            cloneGameBridgeResult(req.Result),
		MemoryState:       "candidate",
		Promoted:          false,
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.appendLocked(event); err != nil {
		return GameBridgeEvent{}, err
	}
	return event, nil
}

func (s *GameBridgeStore) RecentGameBridgeEvents(_ context.Context, gameID string, sessionID string, limit int) ([]GameBridgeEvent, error) {
	if s == nil || s.path == "" {
		return []GameBridgeEvent{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	events, err := s.readAllLocked()
	if err != nil {
		return nil, err
	}
	gameID = strings.TrimSpace(gameID)
	sessionID = strings.TrimSpace(sessionID)
	filtered := make([]GameBridgeEvent, 0)
	for _, event := range events {
		if strings.EqualFold(event.GameID, gameID) && event.SessionID == sessionID {
			filtered = append(filtered, event)
		}
	}
	for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
		filtered[i], filtered[j] = filtered[j], filtered[i]
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (s *GameBridgeStore) findByEventIDLocked(eventID string) (GameBridgeEvent, bool, error) {
	events, err := s.readAllLocked()
	if err != nil {
		return GameBridgeEvent{}, false, err
	}
	for _, event := range events {
		if event.EventID == eventID {
			return event, true, nil
		}
	}
	return GameBridgeEvent{}, false, nil
}

func (s *GameBridgeStore) readAllLocked() ([]GameBridgeEvent, error) {
	file, err := os.Open(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []GameBridgeEvent{}, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	events := make([]GameBridgeEvent, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event GameBridgeEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.EventID == "" {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s *GameBridgeStore) appendLocked(event GameBridgeEvent) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return nil
}

func cloneGameBridgeResult(result map[string]any) map[string]any {
	if result == nil {
		return nil
	}
	cloned := make(map[string]any, len(result))
	for key, value := range result {
		cloned[key] = value
	}
	return cloned
}
