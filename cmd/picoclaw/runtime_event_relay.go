package main

import (
	"log"
	"strings"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/adapter/viewer"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

type idleAwareEventListener struct {
	hub      *viewer.EventHub
	monitor  *viewer.MonitorStore
	archive  *viewer.EventLogStore
	mu       sync.RWMutex
	idleChat *idlechat.IdleChatOrchestrator
}

func (l *idleAwareEventListener) SetIdleChat(idle *idlechat.IdleChatOrchestrator) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.idleChat = idle
}

func (l *idleAwareEventListener) OnEvent(ev orchestrator.OrchestratorEvent) {
	if l.archive != nil {
		if err := l.archive.Append(ev); err != nil {
			log.Printf("WARN: failed to append viewer event log: %v", err)
		}
	}
	l.hub.OnEvent(ev)
	if l.monitor != nil {
		l.monitor.OnEvent(ev)
	}
	if !shouldStopIdleChatByEvent(ev) {
		return
	}
	l.mu.RLock()
	idle := l.idleChat
	l.mu.RUnlock()
	if idle != nil {
		idle.NotifyActivity()
	}
}

func shouldStopIdleChatByEvent(ev orchestrator.OrchestratorEvent) bool {
	if strings.EqualFold(ev.Route, "IDLECHAT") {
		return false
	}
	if ev.Type == "tts.audio_chunk" || strings.EqualFold(ev.From, "tts") {
		return false
	}
	if ev.Type == "message.received" {
		return true
	}
	if ev.Type == "entry.stage" {
		stage := strings.ToLower(strings.TrimSpace(ev.Content))
		return stage == "received" || stage == "planning"
	}
	return false
}
