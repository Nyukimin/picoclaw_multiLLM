package main

import (
	"strconv"
	"strings"
	"sync"
)

type ttsPublicSessionRoute struct {
	publicSessionID string
	chunkIndexes    map[int]int
}

var (
	ttsPublicSessionMu     sync.Mutex
	ttsPublicSessionRoutes = map[string]*ttsPublicSessionRoute{}
	ttsPublicNextChunk     = map[string]int{}
	ttsPublicNextResponse  = map[string]int{}
)

func registerTTSPublicSession(internalSessionID, publicSessionID string) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	publicSessionID = strings.TrimSpace(publicSessionID)
	if internalSessionID == "" || publicSessionID == "" || internalSessionID == publicSessionID {
		return
	}
	ttsPublicSessionMu.Lock()
	ttsPublicSessionRoutes[internalSessionID] = &ttsPublicSessionRoute{
		publicSessionID: publicSessionID,
		chunkIndexes:    map[int]int{},
	}
	ttsPublicSessionMu.Unlock()
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

func clearTTSPublicSession(internalSessionID string) {
	internalSessionID = strings.TrimSpace(internalSessionID)
	if internalSessionID == "" {
		return
	}
	ttsPublicSessionMu.Lock()
	delete(ttsPublicSessionRoutes, internalSessionID)
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
