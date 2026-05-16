package main

import "sync"

var (
	idleChatTTSPendingMu sync.Mutex
	idleChatTTSPending   = map[string]chan struct{}{}
	// Topic announcement must finish before the first agent line for the same idle session.
	idleChatTopicGate  = map[string]chan struct{}{}
	idleChatTopicByTTS = map[string]string{}
)

func registerIdleChatTTSPending(sessionID string) <-chan struct{} {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	ch := make(chan struct{})
	idleChatTTSPending[sessionID] = ch
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

func notifyIdleChatTTSCompleted(sessionID string) {
	idleChatTTSPendingMu.Lock()
	ch, ok := idleChatTTSPending[sessionID]
	if ok {
		delete(idleChatTTSPending, sessionID)
	}
	idleSessionID, topicOK := idleChatTopicByTTS[sessionID]
	if topicOK {
		delete(idleChatTopicByTTS, sessionID)
	}
	var topicCh chan struct{}
	if topicOK {
		topicCh = idleChatTopicGate[idleSessionID]
		delete(idleChatTopicGate, idleSessionID)
	}
	idleChatTTSPendingMu.Unlock()
	if ok {
		close(ch)
	}
	if topicCh != nil {
		// Unblock queued agent speech once the topic announcement session is fully completed.
		close(topicCh)
	}
}

func clearIdleChatTTSPending(sessionID string) {
	idleChatTTSPendingMu.Lock()
	delete(idleChatTTSPending, sessionID)
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
