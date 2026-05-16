package orchestrator

import domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"

type distributedEventPort struct {
	listener EventListener
}

func newDistributedEventPort(listener EventListener) *distributedEventPort {
	return &distributedEventPort{listener: listener}
}

func (p *distributedEventPort) SetListener(listener EventListener) {
	p.listener = listener
}

func (p *distributedEventPort) Emit(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {
	if p.listener == nil {
		return
	}
	p.listener.OnEvent(NewEvent(eventType, from, to, content, route, jobID, sessionID, channel, chatID))
}

func (p *distributedEventPort) EmitNote(from, to, content, route, jobID, sessionID, channel, chatID string) {
	p.Emit("agent.note", from, to, content, route, jobID, sessionID, channel, chatID)
}

func (p *distributedEventPort) EmitProgress(eventType, from, to, content string, msg domaintransport.Message) {
	route, channel, chatID := routeAndChannelFromMessage(msg)
	p.Emit(eventType, from, to, content, route, msg.JobID, msg.SessionID, channel, chatID)
}
