package transport

import (
	"context"
	"fmt"
	"log"
	"time"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

// LocalAgentHandler handles one inbound message and returns one response message.
type LocalAgentHandler interface {
	HandleMessage(ctx context.Context, msg domaintransport.Message) (domaintransport.Message, error)
}

// StartLocalAgentLoop starts a goroutine that consumes inbound messages from local transport
// and emits responses to outbound channel so MessageRouter can deliver them.
func StartLocalAgentLoop(ctx context.Context, agentName string, t *LocalTransport, h LocalAgentHandler, handlerTimeout time.Duration) {
	go func() {
		for {
			msg, err := t.Receive(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Printf("[LocalAgentLoop:%s] receive failed: %v", agentName, err)
				}
				return
			}

			msgCtx := ctx
			cancel := func() {}
			if handlerTimeout > 0 {
				msgCtx, cancel = context.WithTimeout(ctx, handlerTimeout)
			}
			resp, err := h.HandleMessage(msgCtx, msg)
			cancel()
			if err != nil {
				resp = domaintransport.NewErrorMessage(
					agentName,
					msg.From,
					msg.SessionID,
					msg.JobID,
					fmt.Sprintf("handler error: %v", err),
				)
			}

			if resp.From == "" {
				resp.From = agentName
			}
			if resp.To == "" {
				resp.To = msg.From
			}
			if resp.SessionID == "" {
				resp.SessionID = msg.SessionID
			}
			if resp.JobID == "" {
				resp.JobID = msg.JobID
			}

			if err := t.Send(ctx, resp); err != nil {
				if ctx.Err() == nil {
					log.Printf("[LocalAgentLoop:%s] send failed: %v", agentName, err)
				}
				return
			}
		}
	}()
}
