package viewer

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

// EventHub broadcasts orchestrator events to connected SSE clients.
// Implements orchestrator.EventListener.
type EventHub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	history []orchestrator.OrchestratorEvent
	maxHist int
}

// NewEventHub creates a new EventHub with the given history capacity.
func NewEventHub(maxHistory int) *EventHub {
	return &EventHub{
		clients: make(map[chan []byte]struct{}),
		maxHist: maxHistory,
	}
}

// OnEvent implements orchestrator.EventListener.
func (h *EventHub) OnEvent(ev orchestrator.OrchestratorEvent) {
	data, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[EventHub] OnEvent: marshal error: %v", err)
		return
	}

	h.mu.Lock()
	clientCount := len(h.clients)
	h.history = append(h.history, ev)
	if len(h.history) > h.maxHist {
		h.history = h.history[len(h.history)-h.maxHist:]
	}
	h.mu.Unlock()

	log.Printf("[EventHub] OnEvent: eventType=%s from=%s to=%s clients=%d", ev.Type, ev.From, ev.To, clientCount)
	h.broadcast(data)
}

func (h *EventHub) broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// client too slow, drop event
		}
	}
}

// Subscribe registers a new SSE client and returns its event channel.
func (h *EventHub) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	clientCount := len(h.clients)
	h.mu.Unlock()
	log.Printf("[EventHub] Subscribe: new client connected (total clients=%d)", clientCount)
	return ch
}

// Unsubscribe removes a client and closes its channel.
func (h *EventHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	clientCount := len(h.clients)
	h.mu.Unlock()
	close(ch)
	log.Printf("[EventHub] Unsubscribe: client disconnected (remaining clients=%d)", clientCount)
}

// History returns a copy of recent events.
func (h *EventHub) History() []orchestrator.OrchestratorEvent {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]orchestrator.OrchestratorEvent, len(h.history))
	copy(result, h.history)
	return result
}
