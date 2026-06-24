package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/infrastructure/transport"
)

func TestBuildSharedRoleContextFormatsUnifiedMemory(t *testing.T) {
	memory := session.NewCentralMemory()
	msg := domaintransport.NewMessage("worker", "mio", "sess-1", "job-1", "修正対象は RenCrow_CMD です")
	msg.Type = domaintransport.MessageTypeResult
	memory.RecordMessage(msg)

	contextBlock := buildSharedRoleContext(memory, 8)
	if !strings.Contains(contextBlock, sharedRoleContextHeader) {
		t.Fatalf("missing shared context header: %q", contextBlock)
	}
	if !strings.Contains(contextBlock, "worker -> mio: 修正対象は RenCrow_CMD です") {
		t.Fatalf("missing unified memory entry: %q", contextBlock)
	}
}

func TestDistributedAnalyzeRouteInjectsSharedRoleContext(t *testing.T) {
	mockMio := &distMockMioAgent{chatResponse: "chat fallback", routeResponse: "ANALYZE"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()
	memory.RecordMessage(domaintransport.NewMessage("worker", "mio", "sess-1", "job-1", "RenCrow_CLI の作業ディレクトリは RenCrow_CMD"))
	heavy := &mockHeavyAgent{response: "heavy response"}

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)
	orch.SetHeavyAgent(heavy)

	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "sess-1",
		Channel:     "viewer",
		ChatID:      "viewer-user",
		UserMessage: "次の問題を分析して",
	})
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if !strings.Contains(heavy.lastInput, sharedRoleContextHeader) {
		t.Fatalf("heavy did not receive shared context: %q", heavy.lastInput)
	}
	if !strings.Contains(heavy.lastInput, "RenCrow_CLI の作業ディレクトリは RenCrow_CMD") {
		t.Fatalf("heavy did not receive prior memory: %q", heavy.lastInput)
	}
	if !strings.Contains(heavy.lastInput, "現在の依頼:\n次の問題を分析して") {
		t.Fatalf("heavy did not preserve current request: %q", heavy.lastInput)
	}
}

func TestDistributedWildRouteInjectsSharedRoleContext(t *testing.T) {
	mockMio := &distMockMioAgent{chatResponse: "chat fallback", routeResponse: "WILD"}
	mockRepo := &distMockSessionRepo{}
	router := transport.NewMessageRouter()
	defer router.Stop()
	memory := session.NewCentralMemory()
	memory.RecordMessage(domaintransport.NewMessage("heavy", "mio", "sess-1", "job-1", "前回の方針は暗い表現を避ける"))
	wild := &distMockWildAgent{response: "wild response"}

	orch := NewDistributedOrchestrator(mockRepo, mockMio, router, memory, nil)
	orch.SetWildAgent(wild)

	_, err := orch.ProcessMessage(context.Background(), ProcessMessageRequest{
		SessionID:   "sess-1",
		Channel:     "viewer",
		ChatID:      "viewer-user",
		UserMessage: "/wild 画像案を出して",
	})
	if err != nil {
		t.Fatalf("ProcessMessage failed: %v", err)
	}
	if !strings.Contains(wild.lastInput, sharedRoleContextHeader) {
		t.Fatalf("wild did not receive shared context: %q", wild.lastInput)
	}
	if !strings.Contains(wild.lastInput, "前回の方針は暗い表現を避ける") {
		t.Fatalf("wild did not receive prior memory: %q", wild.lastInput)
	}
	if !strings.Contains(wild.lastInput, "現在の依頼:\n/wild 画像案を出して") {
		t.Fatalf("wild did not preserve current request: %q", wild.lastInput)
	}
}

func TestDistributedCodeMessageInjectsSharedRoleContext(t *testing.T) {
	memory := session.NewCentralMemory()
	memory.RecordMessage(domaintransport.NewMessage("heavy", "mio", "sess-1", "job-1", "原因は CODE2 の検証契約です"))
	tk := task.NewTask(task.NewJobID(), "CODE: テストを直して", "viewer", "viewer-user")
	coord := newDistributedCodeExecutionCoordinator(
		memory,
		func(eventType, from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(from, to, content, route, jobID, sessionID, channel, chatID string) {},
		func(route routing.Route, userMessage string) string { return "coder2" },
		func() map[string]interface{} { return nil },
		func() int { return 0 },
		nil,
		nil,
	)

	msg := coord.buildCoderMessage("coder2", "sess-1", "job-2", withSharedRoleContextText(tk.UserMessage(), memory), routing.RouteCODE2, tk, 0)
	if !strings.Contains(msg.Content, sharedRoleContextHeader) {
		t.Fatalf("coder did not receive shared context: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "原因は CODE2 の検証契約です") {
		t.Fatalf("coder did not receive heavy memory: %q", msg.Content)
	}
	if !strings.Contains(msg.Content, "現在の依頼:\nCODE: テストを直して") {
		t.Fatalf("coder did not preserve current request: %q", msg.Content)
	}
}
