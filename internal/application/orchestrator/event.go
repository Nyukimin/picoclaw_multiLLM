package orchestrator

import "time"

var jst = time.FixedZone("JST", 9*60*60)

// EventListener receives orchestrator events for external monitoring
type EventListener interface {
	OnEvent(ev OrchestratorEvent)
}

// OrchestratorEvent represents a significant event in message processing
type OrchestratorEvent struct {
	Seq       int64  `json:"seq,omitempty"`        // monotonic event sequence (set by EventHub)
	Type      string `json:"type"`                 // message.received, routing.decision, agent.start, agent.response
	From      string `json:"from"`                 // source agent
	To        string `json:"to,omitempty"`         // target agent
	Content   string `json:"content"`              // message content
	Route     string `json:"route,omitempty"`      // routing category
	JobID     string `json:"job_id,omitempty"`     // task identifier
	SessionID string `json:"session_id,omitempty"` // session identifier
	Channel   string `json:"channel,omitempty"`    // channel identifier
	ChatID    string `json:"chat_id,omitempty"`    // chat identifier
	Timestamp string `json:"timestamp"`
}

// NewEvent creates a new OrchestratorEvent with the current timestamp
func NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) OrchestratorEvent {
	return OrchestratorEvent{
		Type:      eventType,
		From:      from,
		To:        to,
		Content:   content,
		Route:     route,
		JobID:     jobID,
		SessionID: sessionID,
		Channel:   channel,
		ChatID:    chatID,
		Timestamp: time.Now().In(jst).Format(time.RFC3339),
	}
}
