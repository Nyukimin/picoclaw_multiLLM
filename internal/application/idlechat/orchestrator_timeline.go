package idlechat

import (
	"fmt"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func (o *IdleChatOrchestrator) emitTimelineEvent(ev TimelineEvent) <-chan struct{} {
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
