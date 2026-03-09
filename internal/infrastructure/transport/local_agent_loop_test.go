package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

type stubLocalAgentHandler struct {
	response domaintransport.Message
	err      error
	delay    time.Duration
}

func (s *stubLocalAgentHandler) HandleMessage(ctx context.Context, _ domaintransport.Message) (domaintransport.Message, error) {
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return domaintransport.Message{}, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	if s.err != nil {
		return domaintransport.Message{}, s.err
	}
	return s.response, nil
}

func TestStartLocalAgentLoop_RoutesResponseViaRouter(t *testing.T) {
	router := NewMessageRouter()
	defer router.Stop()

	mio := NewLocalTransport()
	coder1 := NewLocalTransport()
	router.RegisterAgent("mio", mio)
	router.RegisterAgent("coder1", coder1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartLocalAgentLoop(ctx, "coder1", coder1, &stubLocalAgentHandler{
		response: domaintransport.Message{
			Type:    domaintransport.MessageTypeResult,
			Content: "ok",
		},
	}, 2*time.Second)

	in := domaintransport.NewMessage("mio", "coder1", "s1", "j1", "do work")
	in.Type = domaintransport.MessageTypeTask
	if err := coder1.PutInboundMessage(in); err != nil {
		t.Fatalf("put inbound: %v", err)
	}

	recvCtx, recvCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer recvCancel()
	got, err := mio.Receive(recvCtx)
	if err != nil {
		t.Fatalf("mio receive failed: %v", err)
	}
	if got.From != "coder1" {
		t.Fatalf("unexpected from: %s", got.From)
	}
	if got.To != "mio" {
		t.Fatalf("unexpected to: %s", got.To)
	}
	if got.Type != domaintransport.MessageTypeResult {
		t.Fatalf("unexpected type: %s", got.Type)
	}
	if got.Content != "ok" {
		t.Fatalf("unexpected content: %s", got.Content)
	}
}

func TestStartLocalAgentLoop_ConvertsHandlerError(t *testing.T) {
	router := NewMessageRouter()
	defer router.Stop()

	mio := NewLocalTransport()
	coder1 := NewLocalTransport()
	router.RegisterAgent("mio", mio)
	router.RegisterAgent("coder1", coder1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartLocalAgentLoop(ctx, "coder1", coder1, &stubLocalAgentHandler{
		err: errors.New("boom"),
	}, 2*time.Second)

	in := domaintransport.NewMessage("mio", "coder1", "s1", "j1", "do work")
	in.Type = domaintransport.MessageTypeTask
	if err := coder1.PutInboundMessage(in); err != nil {
		t.Fatalf("put inbound: %v", err)
	}

	recvCtx, recvCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer recvCancel()
	got, err := mio.Receive(recvCtx)
	if err != nil {
		t.Fatalf("mio receive failed: %v", err)
	}
	if got.Type != domaintransport.MessageTypeError {
		t.Fatalf("expected error message, got %s", got.Type)
	}
	if got.To != "mio" || got.From != "coder1" {
		t.Fatalf("unexpected routing from=%s to=%s", got.From, got.To)
	}
}

func TestStartLocalAgentLoop_HandlerTimeout(t *testing.T) {
	router := NewMessageRouter()
	defer router.Stop()

	mio := NewLocalTransport()
	coder1 := NewLocalTransport()
	router.RegisterAgent("mio", mio)
	router.RegisterAgent("coder1", coder1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartLocalAgentLoop(ctx, "coder1", coder1, &stubLocalAgentHandler{
		response: domaintransport.Message{Type: domaintransport.MessageTypeResult, Content: "late"},
		delay:    80 * time.Millisecond,
	}, 10*time.Millisecond)

	in := domaintransport.NewMessage("mio", "coder1", "s1", "j1", "do work")
	in.Type = domaintransport.MessageTypeTask
	if err := coder1.PutInboundMessage(in); err != nil {
		t.Fatalf("put inbound: %v", err)
	}

	recvCtx, recvCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer recvCancel()
	got, err := mio.Receive(recvCtx)
	if err != nil {
		t.Fatalf("mio receive failed: %v", err)
	}
	if got.Type != domaintransport.MessageTypeError {
		t.Fatalf("expected timeout error message, got %s", got.Type)
	}
}
