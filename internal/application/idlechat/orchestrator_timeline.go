package idlechat

import (
	"fmt"
	"log"
	"strings"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func (o *IdleChatOrchestrator) emitTimelineEvent(ev TimelineEvent) <-chan struct{} {
	if strings.HasPrefix(ev.Type, "idlechat.") && o.isInterruptedSession(ev.SessionID) {
		log.Printf("[IdleChat] stale event discarded: type=%s session=%s", ev.Type, ev.SessionID)
		return nil
	}
	o.recordPersonaTimelineEvent(ev)
	o.mu.Lock()
	emit := o.emitEvent
	o.mu.Unlock()
	if emit != nil {
		return emit(ev)
	}
	return nil
}

func (o *IdleChatOrchestrator) emitTopicToTimeline(sessionID, topic string, strategy TopicStrategy) {
	content := fmt.Sprintf("今日のお題（%s）: %s", strategy, topic)
	msg := domaintransport.NewMessage("user", "mio", sessionID, "", content)
	msg.Type = domaintransport.MessageTypeIdleChat
	o.memory.RecordMessage(msg)
	o.emitTimelineEvent(TimelineEvent{
		Type:      "idlechat.message",
		From:      "user",
		To:        "mio",
		Content:   content,
		SessionID: sessionID,
	})
}
