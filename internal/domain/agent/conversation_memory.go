package agent

import (
	"context"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

type speakerAwareConversationEngine interface {
	EndTurnAs(ctx context.Context, sessionID string, userMessage string, response string, speaker conversation.Speaker) error
}

func endConversationTurnAs(ctx context.Context, engine conversation.ConversationEngine, sessionID, userMessage, response string, speaker conversation.Speaker) error {
	if aware, ok := engine.(speakerAwareConversationEngine); ok {
		return aware.EndTurnAs(ctx, sessionID, userMessage, response, speaker)
	}
	return engine.EndTurn(ctx, sessionID, userMessage, response)
}
