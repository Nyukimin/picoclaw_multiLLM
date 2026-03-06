package conversation

import (
	"context"
	"fmt"
	"testing"

	domconv "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
)

// === Mocks ===

type mockManager struct {
	recallFunc          func(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error)
	storeFunc           func(ctx context.Context, sessionID string, msg domconv.Message) error
	getActiveThreadFunc func(ctx context.Context, sessionID string) (*domconv.Thread, error)
	flushThreadFunc     func(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error)
	createThreadFunc    func(ctx context.Context, sessionID, domain string) (*domconv.Thread, error)
}

func (m *mockManager) Recall(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error) {
	if m.recallFunc != nil {
		return m.recallFunc(ctx, sessionID, query, topK)
	}
	return nil, nil
}

func (m *mockManager) Store(ctx context.Context, sessionID string, msg domconv.Message) error {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, sessionID, msg)
	}
	return nil
}

func (m *mockManager) FlushThread(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
	if m.flushThreadFunc != nil {
		return m.flushThreadFunc(ctx, threadID)
	}
	return &domconv.ThreadSummary{}, nil
}

func (m *mockManager) GetActiveThread(ctx context.Context, sessionID string) (*domconv.Thread, error) {
	if m.getActiveThreadFunc != nil {
		return m.getActiveThreadFunc(ctx, sessionID)
	}
	return domconv.NewThread(sessionID, "general"), nil
}

func (m *mockManager) CreateThread(ctx context.Context, sessionID, domain string) (*domconv.Thread, error) {
	if m.createThreadFunc != nil {
		return m.createThreadFunc(ctx, sessionID, domain)
	}
	return domconv.NewThread(sessionID, domain), nil
}

func (m *mockManager) IsNovelInformation(ctx context.Context, msg domconv.Message) (bool, float32, error) {
	return true, 0.5, nil
}

func (m *mockManager) GetAgentStatus(ctx context.Context, agentName string) (*domconv.AgentStatus, error) {
	return nil, nil
}

func (m *mockManager) UpdateAgentStatus(ctx context.Context, status *domconv.AgentStatus) error {
	return nil
}

type mockDetector struct {
	result domconv.ThreadBoundaryResult
}

func (m *mockDetector) Detect(currentThread *domconv.Thread, newMessage, newDomain string) domconv.ThreadBoundaryResult {
	return m.result
}

type mockExtractor struct {
	result *domconv.ProfileExtractionResult
	err    error
}

func (m *mockExtractor) Extract(ctx context.Context, thread *domconv.Thread, existing domconv.UserProfile) (*domconv.ProfileExtractionResult, error) {
	return m.result, m.err
}

// === Tests ===

func TestBeginTurn_EmptyRecall(t *testing.T) {
	mgr := &mockManager{}
	persona := domconv.NewMioPersona("test prompt")
	engine := NewRealConversationEngine(mgr, persona)

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if pack.Persona.Name != "ミオ" {
		t.Errorf("Persona.Name: want 'ミオ', got %q", pack.Persona.Name)
	}
	if len(pack.ShortContext) != 0 {
		t.Errorf("ShortContext should be empty, got %d", len(pack.ShortContext))
	}
}

func TestBeginTurn_WithShortContext(t *testing.T) {
	mgr := &mockManager{
		recallFunc: func(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error) {
			return []domconv.Message{
				{Speaker: domconv.SpeakerUser, Msg: "prev question"},
				{Speaker: domconv.SpeakerMio, Msg: "prev answer"},
			}, nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if len(pack.ShortContext) != 2 {
		t.Fatalf("ShortContext: want 2, got %d", len(pack.ShortContext))
	}
	if pack.ShortContext[0].Msg != "prev question" {
		t.Errorf("ShortContext[0]: want 'prev question', got %q", pack.ShortContext[0].Msg)
	}
}

func TestBeginTurn_WithMidSummaries(t *testing.T) {
	mgr := &mockManager{
		recallFunc: func(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error) {
			return []domconv.Message{
				{Speaker: domconv.SpeakerSystem, Msg: "[Summary] Discussed Go testing"},
			}, nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if len(pack.MidSummaries) != 1 {
		t.Fatalf("MidSummaries: want 1, got %d", len(pack.MidSummaries))
	}
	if pack.MidSummaries[0].Summary != "Discussed Go testing" {
		t.Errorf("MidSummaries[0].Summary: want 'Discussed Go testing', got %q", pack.MidSummaries[0].Summary)
	}
}

func TestBeginTurn_WithLongFacts(t *testing.T) {
	mgr := &mockManager{
		recallFunc: func(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error) {
			return []domconv.Message{
				{Speaker: domconv.SpeakerSystem, Msg: "[LongTermMemory] User prefers Go"},
			}, nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if len(pack.LongFacts) != 1 {
		t.Fatalf("LongFacts: want 1, got %d", len(pack.LongFacts))
	}
	if pack.LongFacts[0] != "User prefers Go" {
		t.Errorf("LongFacts[0]: want 'User prefers Go', got %q", pack.LongFacts[0])
	}
}

func TestBeginTurn_RecallError_GracefulDegradation(t *testing.T) {
	mgr := &mockManager{
		recallFunc: func(ctx context.Context, sessionID, query string, topK int) ([]domconv.Message, error) {
			return nil, fmt.Errorf("redis down")
		},
	}
	persona := domconv.NewMioPersona("test")
	engine := NewRealConversationEngine(mgr, persona)

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn should succeed even on recall error: %v", err)
	}
	if pack.Persona.Name != "ミオ" {
		t.Error("Persona should still be set")
	}
	if len(pack.ShortContext) != 0 {
		t.Error("ShortContext should be empty on recall failure")
	}
}

func TestBeginTurn_WithUserProfile(t *testing.T) {
	mgr := &mockManager{}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})
	// Pre-populate profile cache
	profile := domconv.NewUserProfile("s1")
	profile.Merge(map[string]string{"lang": "Go"}, nil)
	engine.profiles["s1"] = profile

	pack, err := engine.BeginTurn(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("BeginTurn failed: %v", err)
	}
	if pack.UserProfile.Preferences["lang"] != "Go" {
		t.Error("UserProfile should be loaded from cache")
	}
}

func TestEndTurn_BasicStore(t *testing.T) {
	stored := []string{}
	mgr := &mockManager{
		storeFunc: func(ctx context.Context, sessionID string, msg domconv.Message) error {
			stored = append(stored, string(msg.Speaker)+":"+msg.Msg)
			return nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	err := engine.EndTurn(context.Background(), "s1", "hello", "hi there")
	if err != nil {
		t.Fatalf("EndTurn failed: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("expected 2 stores, got %d", len(stored))
	}
	if stored[0] != "user:hello" {
		t.Errorf("stored[0]: want 'user:hello', got %q", stored[0])
	}
	if stored[1] != "mio:hi there" {
		t.Errorf("stored[1]: want 'mio:hi there', got %q", stored[1])
	}
}

func TestEndTurn_WithDetector_NoBoundary(t *testing.T) {
	flushCalled := false
	mgr := &mockManager{
		flushThreadFunc: func(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
			flushCalled = true
			return &domconv.ThreadSummary{}, nil
		},
	}
	detector := &mockDetector{
		result: domconv.ThreadBoundaryResult{ShouldCreateNew: false},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{}).WithDetector(detector)

	err := engine.EndTurn(context.Background(), "s1", "hello", "hi")
	if err != nil {
		t.Fatalf("EndTurn failed: %v", err)
	}
	if flushCalled {
		t.Error("FlushThread should NOT be called when no boundary detected")
	}
}

func TestEndTurn_WithDetector_Boundary(t *testing.T) {
	flushCalled := false
	createCalled := false
	mgr := &mockManager{
		flushThreadFunc: func(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
			flushCalled = true
			return &domconv.ThreadSummary{}, nil
		},
		createThreadFunc: func(ctx context.Context, sessionID, domain string) (*domconv.Thread, error) {
			createCalled = true
			return domconv.NewThread(sessionID, domain), nil
		},
	}
	detector := &mockDetector{
		result: domconv.ThreadBoundaryResult{ShouldCreateNew: true, Reason: domconv.BoundaryKeyword},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{}).WithDetector(detector)

	err := engine.EndTurn(context.Background(), "s1", "new topic", "response")
	if err != nil {
		t.Fatalf("EndTurn failed: %v", err)
	}
	if !flushCalled {
		t.Error("FlushThread should be called when boundary detected")
	}
	if !createCalled {
		t.Error("CreateThread should be called after flush")
	}
}

func TestEndTurn_WithProfileExtractor(t *testing.T) {
	mgr := &mockManager{}
	extractor := &mockExtractor{
		result: &domconv.ProfileExtractionResult{
			NewPreferences: map[string]string{"lang": "Go"},
			NewFacts:       []string{"developer"},
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{}).WithProfileExtractor(extractor)

	err := engine.EndTurn(context.Background(), "s1", "hello", "hi")
	if err != nil {
		t.Fatalf("EndTurn failed: %v", err)
	}
	profile, ok := engine.profiles["s1"]
	if !ok {
		t.Fatal("profile should be cached")
	}
	if profile.Preferences["lang"] != "Go" {
		t.Errorf("profile lang: want 'Go', got %q", profile.Preferences["lang"])
	}
	if len(profile.Facts) != 1 || profile.Facts[0] != "developer" {
		t.Errorf("profile facts: want ['developer'], got %v", profile.Facts)
	}
}

func TestEndTurn_ProfileExtractorError(t *testing.T) {
	mgr := &mockManager{}
	extractor := &mockExtractor{
		err: fmt.Errorf("LLM failed"),
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{}).WithProfileExtractor(extractor)

	// Should not return error — profile extraction is best-effort
	err := engine.EndTurn(context.Background(), "s1", "hello", "hi")
	if err != nil {
		t.Fatalf("EndTurn should succeed even on extractor error: %v", err)
	}
}

func TestGetPersona(t *testing.T) {
	persona := domconv.NewMioPersona("custom prompt")
	engine := NewRealConversationEngine(&mockManager{}, persona)
	got := engine.GetPersona()
	if got.Name != "ミオ" {
		t.Errorf("Name: want 'ミオ', got %q", got.Name)
	}
	if got.SystemPrompt != "custom prompt" {
		t.Errorf("SystemPrompt: want 'custom prompt', got %q", got.SystemPrompt)
	}
}

func TestFlushCurrentThread_Success(t *testing.T) {
	flushCalled := false
	createCalled := false
	mgr := &mockManager{
		flushThreadFunc: func(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
			flushCalled = true
			return &domconv.ThreadSummary{}, nil
		},
		createThreadFunc: func(ctx context.Context, sessionID, domain string) (*domconv.Thread, error) {
			createCalled = true
			return domconv.NewThread(sessionID, domain), nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	err := engine.FlushCurrentThread(context.Background(), "s1")
	if err != nil {
		t.Fatalf("FlushCurrentThread failed: %v", err)
	}
	if !flushCalled {
		t.Error("FlushThread should be called")
	}
	if !createCalled {
		t.Error("CreateThread should be called after flush")
	}
}

func TestFlushCurrentThread_NoActiveThread(t *testing.T) {
	mgr := &mockManager{
		getActiveThreadFunc: func(ctx context.Context, sessionID string) (*domconv.Thread, error) {
			return nil, fmt.Errorf("no active thread")
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	err := engine.FlushCurrentThread(context.Background(), "s1")
	if err == nil {
		t.Error("FlushCurrentThread should fail when no active thread")
	}
}

func TestGetStatus_WithActiveThread(t *testing.T) {
	thread := domconv.NewThread("s1", "programming")
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerUser, "msg1", nil))
	thread.AddMessage(domconv.NewMessage(domconv.SpeakerMio, "msg2", nil))

	mgr := &mockManager{
		getActiveThreadFunc: func(ctx context.Context, sessionID string) (*domconv.Thread, error) {
			return thread, nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	status, err := engine.GetStatus(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	if status.SessionID != "s1" {
		t.Errorf("SessionID: want 's1', got %q", status.SessionID)
	}
	if status.ThreadDomain != "programming" {
		t.Errorf("ThreadDomain: want 'programming', got %q", status.ThreadDomain)
	}
	if status.TurnCount != 2 {
		t.Errorf("TurnCount: want 2, got %d", status.TurnCount)
	}
}

func TestGetStatus_NoActiveThread(t *testing.T) {
	mgr := &mockManager{
		getActiveThreadFunc: func(ctx context.Context, sessionID string) (*domconv.Thread, error) {
			return nil, fmt.Errorf("not found")
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	status, err := engine.GetStatus(context.Background(), "s1")
	if err != nil {
		t.Fatalf("GetStatus should not fail: %v", err)
	}
	if status.SessionID != "s1" {
		t.Errorf("SessionID: want 's1', got %q", status.SessionID)
	}
	if status.TurnCount != 0 {
		t.Errorf("TurnCount should be 0, got %d", status.TurnCount)
	}
}

func TestResetSession(t *testing.T) {
	flushCalled := false
	createDomain := ""
	mgr := &mockManager{
		flushThreadFunc: func(ctx context.Context, threadID int64) (*domconv.ThreadSummary, error) {
			flushCalled = true
			return &domconv.ThreadSummary{}, nil
		},
		createThreadFunc: func(ctx context.Context, sessionID, domain string) (*domconv.Thread, error) {
			createDomain = domain
			return domconv.NewThread(sessionID, domain), nil
		},
	}
	engine := NewRealConversationEngine(mgr, domconv.PersonaState{})

	err := engine.ResetSession(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ResetSession failed: %v", err)
	}
	if !flushCalled {
		t.Error("FlushThread should be called during reset")
	}
	if createDomain != "general" {
		t.Errorf("new thread domain should be 'general', got %q", createDomain)
	}
}
