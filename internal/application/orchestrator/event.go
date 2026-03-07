package orchestrator

import "time"

// EventListener receives orchestrator events for external monitoring
type EventListener interface {
	OnEvent(ev OrchestratorEvent)
}

// OrchestratorEvent represents a significant event in message processing
type OrchestratorEvent struct {
	Type      string `json:"type"`                // message.received, routing.decision, agent.start, agent.response
	From      string `json:"from"`                // source agent
	To        string `json:"to,omitempty"`         // target agent
	Content   string `json:"content"`             // message content
	Route     string `json:"route,omitempty"`      // routing category
	JobID     string `json:"job_id,omitempty"`     // task identifier
	Timestamp string `json:"timestamp"`
}

// NewEvent creates a new OrchestratorEvent with the current timestamp
func NewEvent(eventType, from, to, content, route, jobID string) OrchestratorEvent {
	return OrchestratorEvent{
		Type:      eventType,
		From:      from,
		To:        to,
		Content:   content,
		Route:     route,
		JobID:     jobID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
