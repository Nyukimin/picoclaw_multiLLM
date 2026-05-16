package orchestrator

import "log"

type messageEventPort struct {
	listener EventListener
}

func newMessageEventPort(listener EventListener) *messageEventPort {
	return &messageEventPort{listener: listener}
}

func (p *messageEventPort) SetListener(listener EventListener) {
	p.listener = listener
}

func (p *messageEventPort) Emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if p.listener == nil {
		log.Printf("[MessageOrch] emit SKIPPED: no listener (eventType=%s from=%s to=%s)", eventType, from, to)
		return
	}
	log.Printf("[MessageOrch] emit: eventType=%s from=%s to=%s route=%s jobID=%s", eventType, from, to, route, jobID)
	p.listener.OnEvent(NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID))
}

func (p *messageEventPort) EmitMessageReceived(req ProcessMessageRequest) {
	p.Emit("message.received", "user", "mio", req.UserMessage, "", "", req.SessionID, req.Channel, req.ChatID)
}

func (o *MessageOrchestrator) emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	o.events.Emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID)
}

func (o *MessageOrchestrator) emitMessageReceived(req ProcessMessageRequest) {
	o.events.EmitMessageReceived(req)
}
