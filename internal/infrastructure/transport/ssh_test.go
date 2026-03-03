package transport

import (
	"context"
	"testing"
	"time"

	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func TestNewSSHTransport(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/home/user/.ssh/id_ed25519", "worker")

	if st.host != "192.168.1.100:22" {
		t.Errorf("Expected host '192.168.1.100:22', got '%s'", st.host)
	}
	if st.user != "picoclaw" {
		t.Errorf("Expected user 'picoclaw', got '%s'", st.user)
	}
	if st.agentType != "worker" {
		t.Errorf("Expected agentType 'worker', got '%s'", st.agentType)
	}
	if st.closed {
		t.Error("Should not be closed initially")
	}
}

func TestSSHTransport_IsHealthy_BeforeConnect(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/path/to/key", "worker")
	defer st.Close()

	// 接続前はnot healthy
	if st.IsHealthy() {
		t.Error("Should not be healthy before Connect()")
	}
}

func TestSSHTransport_Close(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/path/to/key", "worker")

	if err := st.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if st.IsHealthy() {
		t.Error("Should not be healthy after Close()")
	}

	// Double close should be idempotent
	if err := st.Close(); err != nil {
		t.Fatalf("Second close failed: %v", err)
	}
}

func TestSSHTransport_SendAfterClose(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/path/to/key", "worker")
	st.Close()

	msg := domaintransport.NewMessage("A", "B", "s1", "j1", "test")
	err := st.Send(context.Background(), msg)
	if err == nil {
		t.Error("Expected error on send after close")
	}
}

func TestSSHTransport_ReceiveAfterClose(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/path/to/key", "worker")
	st.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := st.Receive(ctx)
	if err == nil {
		t.Error("Expected error on receive after close")
	}
}

func TestSSHTransport_ConnectFailsWithBadKey(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/nonexistent/key", "worker")
	defer st.Close()

	err := st.Connect()
	if err == nil {
		t.Error("Expected error with non-existent key file")
	}
}

func TestSSHTransport_SendWithoutConnection(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "picoclaw", "/path/to/key", "worker")
	defer st.Close()

	msg := domaintransport.NewMessage("A", "B", "s1", "j1", "test")
	err := st.Send(context.Background(), msg)
	if err == nil {
		t.Error("Expected error on send without connection")
	}
}
