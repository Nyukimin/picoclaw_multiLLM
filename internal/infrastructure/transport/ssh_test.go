package transport

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// === Step 2: getHostKeyCallback / receiveLoop / tryReconnect テスト ===

func TestSSHTransport_GetHostKeyCallback_NoKnownHosts(t *testing.T) {
	// HOME を known_hosts が無いtmpdirに変更
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	defer st.Close()

	callback, err := st.getHostKeyCallback()
	if err != nil {
		t.Fatalf("getHostKeyCallback should not error: %v", err)
	}
	if callback == nil {
		t.Error("callback should not be nil (InsecureIgnoreHostKey)")
	}
}

func TestSSHTransport_GetHostKeyCallback_WithKnownHosts(t *testing.T) {
	// HOME をtmpdirに変更し、.ssh/known_hosts を作成
	tmpHome := t.TempDir()
	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)

	// 有効な known_hosts を作成（空でもパース可能）
	knownHostsPath := filepath.Join(sshDir, "known_hosts")
	os.WriteFile(knownHostsPath, []byte(""), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	defer st.Close()

	callback, err := st.getHostKeyCallback()
	if err != nil {
		t.Fatalf("getHostKeyCallback should succeed with empty known_hosts: %v", err)
	}
	if callback == nil {
		t.Error("callback should not be nil")
	}
}

func TestSSHTransport_ReceiveLoop(t *testing.T) {
	pr, pw := io.Pipe()

	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	st.stdout = pr
	st.receiveLoopDone = make(chan struct{})

	go st.receiveLoop()

	// JSONメッセージを送信
	msg := domaintransport.NewMessage("A", "B", "s1", "j1", "hello-receive-loop")
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	pw.Write(data)

	// 受信確認
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	received, err := st.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if received.Content != "hello-receive-loop" {
		t.Errorf("Expected 'hello-receive-loop', got '%s'", received.Content)
	}

	// クリーンアップ
	pw.Close()
	close(st.done)
	<-st.receiveLoopDone
}

func TestSSHTransport_ReceiveLoop_InvalidJSON(t *testing.T) {
	pr, pw := io.Pipe()

	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	st.stdout = pr
	st.receiveLoopDone = make(chan struct{})

	go st.receiveLoop()

	// 不正JSON → スキップされる
	pw.Write([]byte("this is not json\n"))

	// 有効なメッセージ → 受信される
	msg := domaintransport.NewMessage("A", "B", "s1", "j1", "valid-after-invalid")
	data, _ := json.Marshal(msg)
	data = append(data, '\n')
	pw.Write(data)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	received, err := st.Receive(ctx)
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if received.Content != "valid-after-invalid" {
		t.Errorf("Expected 'valid-after-invalid', got '%s'", received.Content)
	}

	pw.Close()
	close(st.done)
	<-st.receiveLoopDone
}

func TestSSHTransport_ReceiveLoop_DoneSignal(t *testing.T) {
	pr, pw := io.Pipe()

	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	st.stdout = pr
	st.receiveLoopDone = make(chan struct{})

	go st.receiveLoop()

	// done をクローズしてループ終了
	close(st.done)
	pw.Close()

	// receiveLoopDone が閉じられるのを待つ
	select {
	case <-st.receiveLoopDone:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("receiveLoop did not terminate after done signal")
	}
}

func TestSSHTransport_TryReconnect_AlreadyClosed(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	st.closed = true

	// closed状態では tryReconnect は即座にリターン
	st.tryReconnect()

	// reconnecting フラグが立っていないことを確認
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.reconnecting {
		t.Error("reconnecting should not be set when already closed")
	}
}

func TestSSHTransport_TryReconnect_AlreadyReconnecting(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	st.reconnecting = true

	// reconnecting中は tryReconnect は即座にリターン
	st.tryReconnect()

	// まだreconnecting状態のはず（変更なし）
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.reconnecting {
		t.Error("reconnecting should remain true")
	}
}

func TestSSHTransport_IsHealthy_WhileReconnecting(t *testing.T) {
	st := NewSSHTransport("192.168.1.100:22", "user", "/path/to/key", "worker")
	defer st.Close()

	st.reconnecting = true

	if st.IsHealthy() {
		t.Error("Should not be healthy while reconnecting")
	}
}

func TestSSHTransport_StrictHostKey_NoKnownHosts(t *testing.T) {
	// HOME を known_hosts が無いtmpdirに変更
	tmpHome := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	st := NewSSHTransportStrict("192.168.1.100:22", "user", "/path/to/key", "worker", true)
	defer st.Close()

	_, err := st.getHostKeyCallback()
	if err == nil {
		t.Error("Expected error with strict_host_key=true and no known_hosts")
	}
	if !strings.Contains(err.Error(), "strict_host_key=true") {
		t.Errorf("Error should mention strict_host_key, got: %v", err)
	}
}

func TestSSHTransport_StrictHostKey_WithKnownHosts(t *testing.T) {
	tmpHome := t.TempDir()
	sshDir := filepath.Join(tmpHome, ".ssh")
	os.MkdirAll(sshDir, 0700)
	os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(""), 0600)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	st := NewSSHTransportStrict("192.168.1.100:22", "user", "/path/to/key", "worker", true)
	defer st.Close()

	callback, err := st.getHostKeyCallback()
	if err != nil {
		t.Fatalf("Should succeed with known_hosts present: %v", err)
	}
	if callback == nil {
		t.Error("callback should not be nil")
	}
}

func TestNewSSHTransportStrict(t *testing.T) {
	st := NewSSHTransportStrict("host:22", "user", "/key", "worker", true)
	if !st.strictHostKey {
		t.Error("strictHostKey should be true")
	}

	st2 := NewSSHTransportStrict("host:22", "user", "/key", "worker", false)
	if st2.strictHostKey {
		t.Error("strictHostKey should be false")
	}
}

func TestSSHTransport_ReceiveLoop_ChannelFull(t *testing.T) {
	pr, pw := io.Pipe()

	// バッファサイズ1のinboundチャネルで作成
	st := &SSHTransport{
		host:            "192.168.1.100:22",
		user:            "user",
		keyPath:         "/path/to/key",
		agentType:       "worker",
		inbound:         make(chan domaintransport.Message, 1),
		receiveLoopDone: make(chan struct{}),
		done:            make(chan struct{}),
		stdout:          pr,
	}

	go st.receiveLoop()

	// 2メッセージ送信 → 1つ目は受信、2つ目はチャネルフル（ドロップ）
	for i := 0; i < 2; i++ {
		msg := domaintransport.NewMessage("A", "B", "s1", "j1", "msg")
		data, _ := json.Marshal(msg)
		data = append(data, '\n')
		pw.Write(data)
	}

	// 少し待ってチャネルフルのログが出るのを確認
	time.Sleep(100 * time.Millisecond)

	// 1件は受信できるはず
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := st.Receive(ctx)
	if err != nil {
		t.Fatalf("Should receive at least 1 message: %v", err)
	}

	pw.Close()
	close(st.done)
	<-st.receiveLoopDone
}
