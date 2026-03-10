package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/agent"
	domainnode "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/node"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

// distMockMioAgent はDistributedOrchestrator テスト用のMioAgent
type distMockMioAgent struct {
	chatResponse  string
	routeResponse string // "CHAT", "OPS", etc.
	lastChatInput string
}

func (m *distMockMioAgent) DecideAction(ctx context.Context, t task.Task) (routing.Decision, error) {
	route := routing.RouteCHAT
	if m.routeResponse != "" {
		route = routeFromString(m.routeResponse)
	}
	return routing.Decision{
		Route:      route,
		Confidence: 0.9,
	}, nil
}

func (m *distMockMioAgent) Chat(ctx context.Context, t task.Task) (string, error) {
	m.lastChatInput = t.UserMessage()
	return m.chatResponse, nil
}

func (m *distMockMioAgent) HandleChatCommand(ctx context.Context, sessionID string, message string) (agent.ChatCommandResult, error) {
	return agent.ChatCommandResult{Handled: false}, nil
}

// distMockSessionRepo はDistributedOrchestrator テスト用のSessionRepo
type distMockSessionRepo struct {
	sessions map[string]*session.Session
}

func (m *distMockSessionRepo) Save(ctx context.Context, sess *session.Session) error {
	if m.sessions == nil {
		m.sessions = make(map[string]*session.Session)
	}
	m.sessions[sess.ID()] = sess
	return nil
}

func (m *distMockSessionRepo) Load(ctx context.Context, id string) (*session.Session, error) {
	if m.sessions == nil {
		return nil, session.ErrSessionNotFound
	}
	sess, exists := m.sessions[id]
	if !exists {
		return nil, session.ErrSessionNotFound
	}
	return sess, nil
}

func (m *distMockSessionRepo) Exists(ctx context.Context, id string) (bool, error) {
	if m.sessions == nil {
		return false, nil
	}
	_, exists := m.sessions[id]
	return exists, nil
}

func (m *distMockSessionRepo) Delete(ctx context.Context, id string) error {
	if m.sessions != nil {
		delete(m.sessions, id)
	}
	return nil
}

// routeFromString はテスト用のルート文字列→Route変換
func routeFromString(s string) routing.Route {
	switch s {
	case "CHAT":
		return routing.RouteCHAT
	case "OPS":
		return routing.RouteOPS
	case "CODE":
		return routing.RouteCODE
	case "CODE1":
		return routing.RouteCODE1
	case "CODE2":
		return routing.RouteCODE2
	case "CODE3":
		return routing.RouteCODE3
	case "PLAN":
		return routing.RoutePLAN
	case "ANALYZE":
		return routing.RouteANALYZE
	case "RESEARCH":
		return routing.RouteRESEARCH
	default:
		return routing.RouteCHAT
	}
}

func TestDistributedOrchestrator_ProcessMessage_LocalRoute(t *testing.T) {
	mockMio := &distMockMioAgent{chatResponse: "Hello from Mio!"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

	resp, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "hello",
	})

	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Response != "Hello from Mio!" {
		t.Errorf("Expected 'Hello from Mio!', got '%s'", resp.Response)
	}
}

func TestDistributedOrchestrator_AttributionGuardOnUserChat(t *testing.T) {
	mockMio := &distMockMioAgent{chatResponse: "ok"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()
	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

	// first turn
	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "guard-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "最初の質問",
	})
	if err != nil {
		t.Fatalf("first ProcessMessage failed: %v", err)
	}

	// second turn should include attribution guard context from memory
	_, err = orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "guard-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "続きの質問",
	})
	if err != nil {
		t.Fatalf("second ProcessMessage failed: %v", err)
	}

	if !strings.Contains(mockMio.lastChatInput, "【発言帰属ガード】") {
		t.Fatalf("expected guarded chat input, got: %s", mockMio.lastChatInput)
	}
	if !strings.Contains(mockMio.lastChatInput, "【ユーザー依頼】\n続きの質問") {
		t.Fatalf("expected original user request section, got: %s", mockMio.lastChatInput)
	}
}

func TestDistributedOrchestrator_RouteToAgent(t *testing.T) {
	mockMio := &distMockMioAgent{}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

	tests := []struct {
		route    string
		expected string
	}{
		{"OPS", "shiro"},
		{"CODE", "shiro"},
		{"CODE1", "shiro"},
		{"CODE2", "shiro"},
		{"CODE3", "shiro"},
		{"CHAT", ""},
		{"PLAN", ""},
		{"ANALYZE", ""},
		{"RESEARCH", ""},
	}

	for _, tt := range tests {
		result := orch.routeToAgent(routeFromString(tt.route))
		if result != tt.expected {
			t.Errorf("routeToAgent(%s) = '%s', want '%s'", tt.route, result, tt.expected)
		}
	}
}

func TestDistributedOrchestrator_RouteToCoder_ConnectionAware(t *testing.T) {
	mockMio := &distMockMioAgent{}
	mockRepo := &distMockSessionRepo{}

	t.Run("CODE_falls_back_to_connected_coder", func(t *testing.T) {
		router := transport.NewMessageRouter()
		defer router.Stop()
		router.RegisterAgent("coder2", transport.NewLocalTransport())
		memory := session.NewCentralMemory()

		orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)
		if got := orch.routeToCoder(routing.RouteCODE); got != "coder2" {
			t.Fatalf("routeToCoder(CODE) = %q, want coder2", got)
		}
	})

	t.Run("CODE_uses_ssh_connected_coder", func(t *testing.T) {
		router := transport.NewMessageRouter()
		defer router.Stop()
		memory := session.NewCentralMemory()

		sshTransports := map[string]domaintransport.Transport{
			"coder3": &distMockTransport{},
		}
		orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, sshTransports)
		if got := orch.routeToCoder(routing.RouteCODE); got != "coder3" {
			t.Fatalf("routeToCoder(CODE) = %q, want coder3", got)
		}
	})

	t.Run("explicit_route_requires_its_own_coder_connection", func(t *testing.T) {
		router := transport.NewMessageRouter()
		defer router.Stop()
		router.RegisterAgent("coder2", transport.NewLocalTransport())
		memory := session.NewCentralMemory()
		orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

		if got := orch.routeToCoder(routing.RouteCODE1); got != "" {
			t.Fatalf("routeToCoder(CODE1) = %q, want empty", got)
		}
		if got := orch.routeToCoder(routing.RouteCODE2); got != "coder2" {
			t.Fatalf("routeToCoder(CODE2) = %q, want coder2", got)
		}
		if got := orch.routeToCoder(routing.RouteCODE3); got != "" {
			t.Fatalf("routeToCoder(CODE3) = %q, want empty", got)
		}
	})
}

func TestDistributedOrchestrator_RouteToCoderForMessage_UsesCapability(t *testing.T) {
	mockMio := &distMockMioAgent{}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	router.RegisterAgent("coder1", transport.NewLocalTransport())
	router.RegisterAgent("coder2", transport.NewLocalTransport())
	router.RegisterAgent("coder3", transport.NewLocalTransport())
	memory := session.NewCentralMemory()

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)
	orch.SetNodeCapabilities(map[string]domainnode.Capability{
		"coder1": {NodeID: "coder1", HasAudioOut: false},
		"coder2": {NodeID: "coder2", HasAudioOut: true},
		"coder3": {NodeID: "coder3", HasAudioOut: true},
	})

	got := orch.routeToCoderForMessage(routing.RouteCODE, "TTSを実装して")
	if got != "coder3" {
		t.Fatalf("routeToCoderForMessage(CODE,TTS) = %q, want coder3", got)
	}
}

// distMockTransport はSSH経路テスト用のmock Transport
type distMockTransport struct {
	sentMessages []domaintransport.Message
	response     domaintransport.Message
	closed       bool
}

func (m *distMockTransport) Send(ctx context.Context, msg domaintransport.Message) error {
	m.sentMessages = append(m.sentMessages, msg)
	return nil
}

func (m *distMockTransport) Receive(ctx context.Context) (domaintransport.Message, error) {
	return m.response, nil
}

func (m *distMockTransport) Close() error {
	m.closed = true
	return nil
}

func (m *distMockTransport) IsHealthy() bool {
	return !m.closed
}

func TestDistributedOrchestrator_SSHExecution(t *testing.T) {
	mockMio := &distMockMioAgent{routeResponse: "CODE3"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	// Mio/Shiro のローカルTransportを登録（CODE3はShiro経由）
	mioTransport := transport.NewLocalTransport()
	defer mioTransport.Close()
	router.RegisterAgent("mio", mioTransport)
	shiroTransport := transport.NewLocalTransport()
	defer shiroTransport.Close()
	router.RegisterAgent("shiro", shiroTransport)

	// Coder3 のSSH Transport（mock）
	mockSSH := &distMockTransport{
		response: domaintransport.Message{
			From:    "coder3",
			To:      "shiro",
			Content: "code generated by Coder3 via SSH",
			Type:    domaintransport.MessageTypeResult,
		},
	}

	sshTransports := map[string]domaintransport.Transport{
		"coder3": mockSSH,
	}

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, sshTransports)

	// Shiroが最終応答を返す
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		msg, err := shiroTransport.Receive(ctx)
		if err != nil {
			return
		}
		response := domaintransport.NewMessage("shiro", msg.From, msg.SessionID, msg.JobID, "shiro finalized code task")
		response.Type = domaintransport.MessageTypeResult
		_ = mioTransport.PutInboundMessage(response)
	}()

	resp, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "generate code",
	})

	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Response != "shiro finalized code task" {
		t.Errorf("Expected 'shiro finalized code task', got '%s'", resp.Response)
	}

	// SSH Transport経由で送信されたことを確認
	if len(mockSSH.sentMessages) != 1 {
		t.Fatalf("Expected 1 sent message, got %d", len(mockSSH.sentMessages))
	}

	if mockSSH.sentMessages[0].To != "coder3" {
		t.Errorf("Expected message To='Coder3', got '%s'", mockSSH.sentMessages[0].To)
	}
}

func TestDistributedOrchestrator_DistributedExecution(t *testing.T) {
	mockMio := &distMockMioAgent{routeResponse: "OPS"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	// Shiro のTransportを登録
	shiroTransport := transport.NewLocalTransport()
	defer shiroTransport.Close()
	router.RegisterAgent("shiro", shiroTransport)

	// Mio のTransportを登録
	mioTransport := transport.NewLocalTransport()
	defer mioTransport.Close()
	router.RegisterAgent("mio", mioTransport)

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

	// ShiroがメッセージをReceiveして応答を返すゴルーチン
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		msg, err := shiroTransport.Receive(ctx)
		if err != nil {
			return
		}

		response := domaintransport.NewMessage("shiro", msg.From, msg.SessionID, msg.JobID, "task executed by Shiro")
		response.Type = domaintransport.MessageTypeResult
		mioTransport.PutInboundMessage(response)
	}()

	resp, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "run script",
	})

	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}

	if resp.Response != "task executed by Shiro" {
		t.Errorf("Expected 'task executed by Shiro', got '%s'", resp.Response)
	}

	if memory.AgentCount() < 2 {
		t.Errorf("Expected at least 2 agents in memory, got %d", memory.AgentCount())
	}
}

func TestDistributedOrchestrator_ProcessMessage_CodeRoute_UnconnectedExplicitCoder(t *testing.T) {
	mockMio := &distMockMioAgent{routeResponse: "CODE1"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)

	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "test-session",
		Channel:     "line",
		ChatID:      "U123",
		UserMessage: "generate code",
	})
	if err == nil {
		t.Fatal("expected error for unconnected CODE1 coder")
	}
	if !strings.Contains(err.Error(), "no coder mapped for route CODE1") {
		t.Fatalf("unexpected error: %v", err)
	}
}
