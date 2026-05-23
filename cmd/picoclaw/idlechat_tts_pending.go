package main

import "sync"

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

func waitIdleChatTopicGate(idleSessionID string) {
	idleChatTTSPendingMu.Lock()
	ch := idleChatTopicGate[idleSessionID]
	idleChatTTSPendingMu.Unlock()
	if ch == nil {
		return
	}
	<-ch
}
