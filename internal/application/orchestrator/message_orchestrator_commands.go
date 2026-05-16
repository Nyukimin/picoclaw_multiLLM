package orchestrator

import (
	"context"
	"fmt"
)

func (o *MessageOrchestrator) handlePreRoutingChatCommand(ctx context.Context, req ProcessMessageRequest) (ProcessMessageResponse, bool, error) {
	cmdResult, err := o.mio.HandleChatCommand(ctx, req.ChatID, req.UserMessage)
	if err != nil {
		return ProcessMessageResponse{}, false, fmt.Errorf("chat command failed: %w", err)
	}
	if !cmdResult.Handled {
		return ProcessMessageResponse{}, false, nil
	}
	o.emit("agent.response", "mio", "user", cmdResult.Response, "CHAT", "", req.SessionID, req.Channel, req.ChatID)
	return buildChatCommandResponse(cmdResult.Response), true, nil
}
