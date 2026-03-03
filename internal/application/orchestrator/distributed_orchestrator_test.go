package orchestrator

import (
	"context"
	"testing"
	"time"

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
	return m.chatResponse, nil
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

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory)

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

func TestDistributedOrchestrator_RouteToAgent(t *testing.T) {
	mockMio := &distMockMioAgent{}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory)

	tests := []struct {
		route    string
		expected string
	}{
		{"OPS", "Shiro"},
		{"CODE", "Coder1"},
		{"CODE1", "Coder1"},
		{"CODE2", "Coder2"},
		{"CODE3", "Coder3"},
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

func TestDistributedOrchestrator_DistributedExecution(t *testing.T) {
	mockMio := &distMockMioAgent{routeResponse: "OPS"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()

	// Shiro のTransportを登録
	shiroTransport := transport.NewLocalTransport()
	defer shiroTransport.Close()
	router.RegisterAgent("Shiro", shiroTransport)

	// Mio のTransportを登録
	mioTransport := transport.NewLocalTransport()
	defer mioTransport.Close()
	router.RegisterAgent("Mio", mioTransport)

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory)

	// ShiroがメッセージをReceiveして応答を返すゴルーチン
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		msg, err := shiroTransport.Receive(ctx)
		if err != nil {
			return
		}

		response := domaintransport.NewMessage("Shiro", msg.From, msg.SessionID, msg.JobID, "task executed by Shiro")
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
