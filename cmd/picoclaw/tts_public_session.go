package main

import (
	"strconv"
	"strings"
	"sync"
)

type ttsPublicSessionRoute struct {
	publicSessionID string
	responseID      string
	messageID       string
	turnIndex       int
	utteranceID     string
	generation      uint64
	timedOut        bool
	chunkIndexes    map[int]int
}

type ttsPublicSessionSnapshot struct {
	RouteCount               int `json:"route_count"`
	StaleRouteCount          int `json:"stale_route_count"`
	NextChunkSessionCount    int `json:"next_chunk_session_count"`
	NextResponseSessionCount int `json:"next_response_session_count"`
}

var (
	ttsPublicSessionMu     sync.Mutex
	ttsPublicSessionRoutes = map[string]*ttsPublicSessionRoute{}
	ttsPublicNextChunk     = map[string]int{}
	ttsPublicNextResponse  = map[string]int{}
	ttsPublicGeneration    uint64
)

func registerTTSPublicSession(internalSessionID, publicSessionID, responseID string) {
	registerTTSPublicSessionWithMessage(internalSessionID, publicSessionID, responseID, "", 0)
}

func registerTTSPublicSessionWithMessage(internalSessionID, publicSessionID, responseID, messageID string, turnIndex int) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	publicSessionID = strings.TrimSpace(publicSessionID)
	if internalSessionID == "" || publicSessionID == "" || internalSessionID == publicSessionID {
		return
	}
	messageID = strings.TrimSpace(messageID)
	utteranceID := responseID
	if messageID != "" {
		utteranceID = messageID + ":utt:0000"
	}
	ttsPublicSessionMu.Lock()
	ttsPublicSessionRoutes[internalSessionID] = &ttsPublicSessionRoute{
		publicSessionID: publicSessionID,
		responseID:      strings.TrimSpace(responseID),
		messageID:       messageID,
		turnIndex:       turnIndex,
		utteranceID:     strings.TrimSpace(utteranceID),
		generation:      ttsPublicGeneration,
		chunkIndexes:    map[int]int{},
	}
	ttsPublicSessionMu.Unlock()
}

func resetTTSPublicSessionRoutesForIdleChat() {
	ttsPublicSessionMu.Lock()
	ttsPublicGeneration++
	ttsPublicNextChunk = map[string]int{}
	ttsPublicNextResponse = map[string]int{}
	ttsPublicSessionMu.Unlock()
}

func isStaleTTSPublicSession(internalSessionID string) bool {
	internalSessionID = strings.TrimSpace(internalSessionID)
	if internalSessionID == "" {
		return false
	}
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	route := ttsPublicSessionRoutes[internalSessionID]
	return route != nil && (route.generation != ttsPublicGeneration || route.timedOut)
}

func markTTSPublicSessionTimedOut(publicSessionID, messageID string, turnIndex int, allForSession bool) []string {
	publicSessionID = strings.TrimSpace(publicSessionID)
	messageID = strings.TrimSpace(messageID)
	if publicSessionID == "" {
		return nil
	}
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	matched := make([]string, 0, 1)
	for internalSessionID, route := range ttsPublicSessionRoutes {
		if route == nil || strings.TrimSpace(route.publicSessionID) != publicSessionID {
			continue
		}
		if !allForSession {
			if messageID != "" {
				if strings.TrimSpace(route.messageID) != messageID {
					continue
				}
			} else if turnIndex >= 0 && route.turnIndex != turnIndex {
				continue
			} else {
				continue
			}
		}
		route.timedOut = true
		matched = append(matched, internalSessionID)
	}
	return matched
}

func resolveTTSPublicChunk(internalSessionID string, internalChunkIndex int) (string, int) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	route := ttsPublicSessionRoutes[internalSessionID]
	if route == nil || strings.TrimSpace(route.publicSessionID) == "" {
		return internalSessionID, internalChunkIndex
	}
	if publicChunkIndex, ok := route.chunkIndexes[internalChunkIndex]; ok {
		return route.publicSessionID, publicChunkIndex
	}
	publicChunkIndex := ttsPublicNextChunk[route.publicSessionID]
	ttsPublicNextChunk[route.publicSessionID] = publicChunkIndex + 1
	route.chunkIndexes[internalChunkIndex] = publicChunkIndex
	return route.publicSessionID, publicChunkIndex
}

func resolveTTSPublicSession(internalSessionID string) string {
	internalSessionID = strings.TrimSpace(internalSessionID)
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	if route := ttsPublicSessionRoutes[internalSessionID]; route != nil && strings.TrimSpace(route.publicSessionID) != "" {
		return route.publicSessionID
	}
	return internalSessionID
}

func resolveTTSPublicResponse(internalSessionID string) string {
	internalSessionID = strings.TrimSpace(internalSessionID)
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	if route := ttsPublicSessionRoutes[internalSessionID]; route != nil {
		return strings.TrimSpace(route.responseID)
	}
	return ""
}

func resolveTTSPublicMessage(internalSessionID string) (string, int, string) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	if route := ttsPublicSessionRoutes[internalSessionID]; route != nil {
		return strings.TrimSpace(route.messageID), route.turnIndex, strings.TrimSpace(route.utteranceID)
	}
	return "", 0, ""
}

func clearTTSPublicSession(internalSessionID string) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	if internalSessionID == "" {
		return
	}
	ttsPublicSessionMu.Lock()
	delete(ttsPublicSessionRoutes, internalSessionID)
	ttsPublicSessionMu.Unlock()
}

func clearTTSPublicSessionByResponse(responseID string) {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return
	}
	ttsPublicSessionMu.Lock()
	for internalSessionID, route := range ttsPublicSessionRoutes {
		if route != nil && strings.TrimSpace(route.responseID) == responseID {
			delete(ttsPublicSessionRoutes, internalSessionID)
			break
		}
	}
	ttsPublicSessionMu.Unlock()
}

func nextTTSPublicResponseID(publicSessionID string) string {
	publicSessionID = strings.TrimSpace(publicSessionID)
	if publicSessionID == "" {
		return ""
	}
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	next := ttsPublicNextResponse[publicSessionID]
	ttsPublicNextResponse[publicSessionID] = next + 1
	return publicSessionID + ":" + formatFixed4(next)
}

func formatFixed4(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 10 {
		return "000" + string(rune('0'+n))
	}
	if n < 100 {
		return "00" + strconv.Itoa(n)
	}
	if n < 1000 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func isIdleChatPublicSession(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	return strings.HasPrefix(sessionID, "idle-") ||
		strings.HasPrefix(sessionID, "forecast-") ||
		strings.HasPrefix(sessionID, "story-") ||
		strings.HasPrefix(sessionID, "story-simple-")
}

func snapshotTTSPublicSessions() ttsPublicSessionSnapshot {
	ttsPublicSessionMu.Lock()
	defer ttsPublicSessionMu.Unlock()
	currentRoutes := 0
	staleRoutes := 0
	for _, route := range ttsPublicSessionRoutes {
		if route == nil || route.generation != ttsPublicGeneration || route.timedOut {
			staleRoutes++
			continue
		}
		currentRoutes++
	}
	return ttsPublicSessionSnapshot{
		RouteCount:               currentRoutes,
		StaleRouteCount:          staleRoutes,
		NextChunkSessionCount:    len(ttsPublicNextChunk),
		NextResponseSessionCount: len(ttsPublicNextResponse),
	}
}
