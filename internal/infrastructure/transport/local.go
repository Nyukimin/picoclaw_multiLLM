package transport

import (
	"context"
	"fmt"
	"sync"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

const defaultChannelCapacity = 100

// LocalTransport はローカル（同一プロセス内）のAgent間通信
type LocalTransport struct {
	inbound  chan domaintransport.Message
	outbound chan domaintransport.Message
	done     chan struct{}
	mu       sync.Mutex
	closed   bool
}

// NewLocalTransport は新しいLocalTransportを作成
func NewLocalTransport() *LocalTransport {
	return &LocalTransport{
		inbound:  make(chan domaintransport.Message, defaultChannelCapacity),
		outbound: make(chan domaintransport.Message, defaultChannelCapacity),
		done:     make(chan struct{}),
	}
}

// Send はメッセージを送信（outboundチャネルに書き込み）
func (t *LocalTransport) Send(ctx context.Context, msg domaintransport.Message) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return fmt.Errorf("transport is closed")
	}
	t.mu.Unlock()

	select {
	case t.outbound <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return fmt.Errorf("transport is closed")
	}
}

// Receive はメッセージを受信（inboundチャネルから読み取り）
func (t *LocalTransport) Receive(ctx context.Context) (domaintransport.Message, error) {
	select {
	case msg, ok := <-t.inbound:
		if !ok {
			return domaintransport.Message{}, fmt.Errorf("transport is closed")
		}
		return msg, nil
	case <-ctx.Done():
		return domaintransport.Message{}, ctx.Err()
	case <-t.done:
		return domaintransport.Message{}, fmt.Errorf("transport is closed")
	}
}

// Close はTransportを閉じる
func (t *LocalTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil // 冪等
	}

	t.closed = true
	close(t.done)
	return nil
}

// IsHealthy はTransportの健全性を返す
func (t *LocalTransport) IsHealthy() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.closed
}

// GetOutboundChannel はoutboundチャネルを返す（MessageRouter用）
func (t *LocalTransport) GetOutboundChannel() <-chan domaintransport.Message {
	return t.outbound
}

// PutInboundMessage はinboundチャネルにメッセージを投入（ノンブロッキング）
func (t *LocalTransport) PutInboundMessage(msg domaintransport.Message) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return fmt.Errorf("transport is closed")
	}
	t.mu.Unlock()

	select {
	case t.inbound <- msg:
		return nil
	default:
		return fmt.Errorf("inbound channel full for agent, message dropped")
	}
}
