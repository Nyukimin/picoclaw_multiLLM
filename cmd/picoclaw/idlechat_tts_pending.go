package main

import (
	"sort"
	"sync"
)

type idleChatTTSPendingSnapshot struct {
	PendingSessionCount  int      `json:"pending_session_count"`
	PendingResponseCount int      `json:"pending_response_count"`
	PendingSessionIDs    []string `json:"pending_session_ids"`
	PendingResponseIDs   []string `json:"pending_response_ids"`
	TopicGateCount       int      `json:"topic_gate_count"`
	TopicRouteCount      int      `json:"topic_route_count"`
}

var (
	idleChatTTSPendingMu         sync.Mutex
	idleChatTTSPending           = map[string]chan struct{}{}
	idleChatTTSPendingByResponse = map[string]chan struct{}{}
	// Topic announcement must finish before the first agent line for the same idle session.
	idleChatTopicGate  = map[string]chan struct{}{}
	idleChatTopicByTTS = map[string]string{}
)

func registerIdleChatTTSPending(sessionID, responseID string) <-chan struct{} {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	ch := make(chan struct{})
	idleChatTTSPending[sessionID] = ch
	if responseID != "" {
		idleChatTTSPendingByResponse[responseID] = ch
	}
	return ch
}

func registerIdleChatTopicGate(idleSessionID, ttsSessionID string) {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	if _, ok := idleChatTopicGate[idleSessionID]; !ok {
		idleChatTopicGate[idleSessionID] = make(chan struct{})
	}
	idleChatTopicByTTS[ttsSessionID] = idleSessionID
}

func notifyIdleChatTTSPlaybackCompleted(responseID string) bool {
	idleChatTTSPendingMu.Lock()
	ch, ok := idleChatTTSPendingByResponse[responseID]
	var topicCh chan struct{}
	if ok {
		delete(idleChatTTSPendingByResponse, responseID)
		for sessionID, sessionCh := range idleChatTTSPending {
			if sessionCh == ch {
				delete(idleChatTTSPending, sessionID)
				if idleSessionID, topicOK := idleChatTopicByTTS[sessionID]; topicOK {
					delete(idleChatTopicByTTS, sessionID)
					topicCh = idleChatTopicGate[idleSessionID]
					delete(idleChatTopicGate, idleSessionID)
				}
				break
			}
		}
	}
	idleChatTTSPendingMu.Unlock()
	if ok {
		close(ch)
	}
	if topicCh != nil {
		// Unblock queued agent speech once the topic announcement has actually finished playback.
		close(topicCh)
	}
	if ok {
		clearTTSPublicSessionByResponse(responseID)
	}
	return ok
}

func clearIdleChatTTSPending(sessionID string) {
	idleChatTTSPendingMu.Lock()
	if ch, ok := idleChatTTSPending[sessionID]; ok {
		delete(idleChatTTSPending, sessionID)
		for responseID, responseCh := range idleChatTTSPendingByResponse {
			if responseCh == ch {
				delete(idleChatTTSPendingByResponse, responseID)
			}
		}
		close(ch)
	}
	if idleSessionID, ok := idleChatTopicByTTS[sessionID]; ok {
		delete(idleChatTopicByTTS, sessionID)
		if topicCh := idleChatTopicGate[idleSessionID]; topicCh != nil {
			delete(idleChatTopicGate, idleSessionID)
			close(topicCh)
		}
	}
	idleChatTTSPendingMu.Unlock()
}

func clearIdleChatTTSPendingByChan(target <-chan struct{}) {
	idleChatTTSPendingMu.Lock()
	var topicCh chan struct{}
	for sessionID, ch := range idleChatTTSPending {
		if (<-chan struct{})(ch) == target {
			delete(idleChatTTSPending, sessionID)
			for responseID, responseCh := range idleChatTTSPendingByResponse {
				if responseCh == ch {
					delete(idleChatTTSPendingByResponse, responseID)
				}
			}
			if idleSessionID, ok := idleChatTopicByTTS[sessionID]; ok {
				delete(idleChatTopicByTTS, sessionID)
				topicCh = idleChatTopicGate[idleSessionID]
				delete(idleChatTopicGate, idleSessionID)
			}
			break
		}
	}
	idleChatTTSPendingMu.Unlock()
	if topicCh != nil {
		close(topicCh)
	}
}

func clearAllIdleChatTTSPending() {
	idleChatTTSPendingMu.Lock()
	pending := make([]chan struct{}, 0, len(idleChatTTSPending))
	seen := map[chan struct{}]struct{}{}
	for _, ch := range idleChatTTSPending {
		if _, ok := seen[ch]; ok {
			continue
		}
		seen[ch] = struct{}{}
		pending = append(pending, ch)
	}
	topicGates := make([]chan struct{}, 0, len(idleChatTopicGate))
	for _, ch := range idleChatTopicGate {
		topicGates = append(topicGates, ch)
	}
	idleChatTTSPending = map[string]chan struct{}{}
	idleChatTTSPendingByResponse = map[string]chan struct{}{}
	idleChatTopicGate = map[string]chan struct{}{}
	idleChatTopicByTTS = map[string]string{}
	idleChatTTSPendingMu.Unlock()

	for _, ch := range pending {
		close(ch)
	}
	for _, ch := range topicGates {
		close(ch)
	}
}

func waitIdleChatTopicGate(idleSessionID string) {
	idleChatTTSPendingMu.Lock()
	ch := idleChatTopicGate[idleSessionID]
	idleChatTTSPendingMu.Unlock()
	if ch == nil {
		return
	}
	<-ch
}

func snapshotIdleChatTTSPending() idleChatTTSPendingSnapshot {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	sessionIDs := make([]string, 0, len(idleChatTTSPending))
	for sessionID := range idleChatTTSPending {
		sessionIDs = append(sessionIDs, sessionID)
	}
	sort.Strings(sessionIDs)
	responseIDs := make([]string, 0, len(idleChatTTSPendingByResponse))
	for responseID := range idleChatTTSPendingByResponse {
		responseIDs = append(responseIDs, responseID)
	}
	sort.Strings(responseIDs)
	return idleChatTTSPendingSnapshot{
		PendingSessionCount:  len(idleChatTTSPending),
		PendingResponseCount: len(idleChatTTSPendingByResponse),
		PendingSessionIDs:    sessionIDs,
		PendingResponseIDs:   responseIDs,
		TopicGateCount:       len(idleChatTopicGate),
		TopicRouteCount:      len(idleChatTopicByTTS),
	}
}
