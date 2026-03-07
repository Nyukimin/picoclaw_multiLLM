package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/conversation"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

// Mock LLMProvider
type mockLLMProvider struct {
	generateFunc func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error)
}

func (m *mockLLMProvider) Generate(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	if m.generateFunc != nil {
		return m.generateFunc(ctx, req)
	}
	return llm.GenerateResponse{Content: "Mock response"}, nil
}

func (m *mockLLMProvider) Name() string {
	return "mock"
}

// Mock Classifier
type mockClassifier struct {
	classifyFunc func(ctx context.Context, t task.Task) (routing.Decision, error)
}

func (m *mockClassifier) Classify(ctx context.Context, t task.Task) (routing.Decision, error) {
	if m.classifyFunc != nil {
		return m.classifyFunc(ctx, t)
	}
	return routing.NewDecision(routing.RouteCHAT, 0.8, "Mock classification"), nil
}

// Mock RuleDictionary
type mockRuleDictionary struct {
	matchFunc func(t task.Task) (routing.Route, float64, bool)
}

func (m *mockRuleDictionary) Match(t task.Task) (routing.Route, float64, bool) {
	if m.matchFunc != nil {
		return m.matchFunc(t)
	}
	return "", 0.0, false
}


func TestMioAgentDecideAction_ExplicitCommand(t *testing.T) {
	mio := NewMioAgent(
		&mockLLMProvider{},
		&mockClassifier{},
		&mockRuleDictionary{},
		&mockToolRunner{},
		&mockMCPClient{},
		nil, // conversationEngine=nil（テスト環境）
	)

	tests := []struct {
		message      string
		expectedRoute routing.Route
	}{
		{"/chat hello", routing.RouteCHAT},
		{"/code3 implement feature", routing.RouteCODE3},
		{"/plan create project", routing.RoutePLAN},
		{"/analyze logs", routing.RouteANALYZE},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			jobID := task.NewJobID()
			testTask := task.NewTask(jobID, tt.message, "line", "U123")

			decision, err := mio.DecideAction(context.Background(), testTask)
			if err != nil {
				t.Fatalf("DecideAction failed: %v", err)
			}

			if decision.Route != tt.expectedRoute {
				t.Errorf("Expected route %s, got %s", tt.expectedRoute, decision.Route)
			}

			if decision.Confidence != 1.0 {
				t.Errorf("Expected confidence 1.0 for explicit command, got %f", decision.Confidence)
			}
		})
	}
}

func TestMioAgentDecideAction_RuleDictionary(t *testing.T) {
	ruleDictionary := &mockRuleDictionary{
		matchFunc: func(t task.Task) (routing.Route, float64, bool) {
			if t.UserMessage() == "ファイルを作成" {
				return routing.RouteCODE, 0.95, true
			}
			return "", 0.0, false
		},
	}

	mio := NewMioAgent(
		&mockLLMProvider{},
		&mockClassifier{},
		ruleDictionary,
		&mockToolRunner{},
		&mockMCPClient{},
		nil, // conversationEngine=nil（テスト環境）
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "ファイルを作成", "line", "U123")

	decision, err := mio.DecideAction(context.Background(), testTask)
	if err != nil {
		t.Fatalf("DecideAction failed: %v", err)
	}

	if decision.Route != routing.RouteCODE {
		t.Errorf("Expected route CODE, got %s", decision.Route)
	}

	if decision.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", decision.Confidence)
	}
}

func TestMioAgentDecideAction_DefaultChatWhenNoRuleMatch(t *testing.T) {
	// ルール辞書にマッチしない場合、LLM分類器をスキップしてCHATにフォールバック
	classifierCalled := false
	classifier := &mockClassifier{
		classifyFunc: func(ctx context.Context, tk task.Task) (routing.Decision, error) {
			classifierCalled = true
			return routing.Decision{}, nil
		},
	}

	mio := NewMioAgent(
		&mockLLMProvider{},
		classifier,
		&mockRuleDictionary{},
		&mockToolRunner{},
		&mockMCPClient{},
		nil, // conversationEngine=nil（テスト環境）
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "こんにちは", "line", "U123")

	decision, err := mio.DecideAction(context.Background(), testTask)
	if err != nil {
		t.Fatalf("DecideAction failed: %v", err)
	}

	if decision.Route != routing.RouteCHAT {
		t.Errorf("Expected route CHAT, got %s", decision.Route)
	}

	if decision.Confidence != 0.7 {
		t.Errorf("Expected confidence 0.7, got %f", decision.Confidence)
	}

	if classifierCalled {
		t.Error("Classifier should not be called when defaulting to CHAT")
	}
}

func TestMioAgentChat(t *testing.T) {
	llmProvider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{
				Content:      "こんにちは！何かお手伝いできますか？",
				TokensUsed:   20,
				FinishReason: "stop",
			}, nil
		},
	}

	mio := NewMioAgent(
		llmProvider,
		&mockClassifier{},
		&mockRuleDictionary{},
		&mockToolRunner{},
		&mockMCPClient{},
		nil, // conversationEngine=nil（テスト環境）
	)

	jobID := task.NewJobID()
	testTask := task.NewTask(jobID, "こんにちは", "line", "U123")

	response, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if response != "こんにちは！何かお手伝いできますか？" {
		t.Errorf("Unexpected chat response: %s", response)
	}
}

// === mockConversationEngine ===

type mockConversationEngine struct {
	beginTurnFunc func(ctx context.Context, sessionID string, userMessage string) (*conversation.RecallPack, error)
	endTurnFunc   func(ctx context.Context, sessionID string, userMessage string, response string) error
	persona       conversation.PersonaState
}

func (m *mockConversationEngine) BeginTurn(ctx context.Context, sessionID string, userMessage string) (*conversation.RecallPack, error) {
	if m.beginTurnFunc != nil {
		return m.beginTurnFunc(ctx, sessionID, userMessage)
	}
	return &conversation.RecallPack{Persona: m.persona}, nil
}

func (m *mockConversationEngine) EndTurn(ctx context.Context, sessionID string, userMessage string, response string) error {
	if m.endTurnFunc != nil {
		return m.endTurnFunc(ctx, sessionID, userMessage, response)
	}
	return nil
}

func (m *mockConversationEngine) GetPersona() conversation.PersonaState { return m.persona }
func (m *mockConversationEngine) FlushCurrentThread(ctx context.Context, sessionID string) error {
	return nil
}
func (m *mockConversationEngine) GetStatus(ctx context.Context, sessionID string) (*conversation.ConversationStatus, error) {
	return &conversation.ConversationStatus{}, nil
}
func (m *mockConversationEngine) ResetSession(ctx context.Context, sessionID string) error {
	return nil
}

// === Phase 1C: ConversationEngine integration tests ===

func TestMioAgent_Chat_WithConversationEngine(t *testing.T) {
	beginCalled := false
	endCalled := false

	engine := &mockConversationEngine{
		beginTurnFunc: func(ctx context.Context, sessionID, msg string) (*conversation.RecallPack, error) {
			beginCalled = true
			return &conversation.RecallPack{
				Persona: conversation.PersonaState{SystemPrompt: "You are Mio."},
				ShortContext: []conversation.Message{
					{Speaker: conversation.SpeakerUser, Msg: "previous msg"},
				},
			}, nil
		},
		endTurnFunc: func(ctx context.Context, sessionID, msg, resp string) error {
			endCalled = true
			return nil
		},
	}

	var capturedReq llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			capturedReq = req
			return llm.GenerateResponse{Content: "response"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, engine)
	testTask := task.NewTask(task.NewJobID(), "hello", "line", "U123")

	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if resp != "response" {
		t.Errorf("response: want 'response', got %q", resp)
	}
	if !beginCalled {
		t.Error("BeginTurn should have been called")
	}
	if !endCalled {
		t.Error("EndTurn should have been called")
	}
	// Verify RecallPack was injected: system prompt + short context + user message
	if len(capturedReq.Messages) < 3 {
		t.Fatalf("expected at least 3 messages, got %d", len(capturedReq.Messages))
	}
	if capturedReq.Messages[0].Role != "system" {
		t.Errorf("msg[0] role: want 'system', got %q", capturedReq.Messages[0].Role)
	}
}

func TestMioAgent_Chat_ConversationEngine_BeginTurnError(t *testing.T) {
	engine := &mockConversationEngine{
		beginTurnFunc: func(ctx context.Context, sessionID, msg string) (*conversation.RecallPack, error) {
			return nil, fmt.Errorf("redis down")
		},
	}

	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{Content: "fallback response"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, engine)
	testTask := task.NewTask(task.NewJobID(), "hello", "line", "U123")

	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat should succeed even when BeginTurn fails: %v", err)
	}
	if resp != "fallback response" {
		t.Errorf("response: want 'fallback response', got %q", resp)
	}
}

func TestMioAgent_Chat_ConversationEngine_EndTurnError(t *testing.T) {
	engine := &mockConversationEngine{
		endTurnFunc: func(ctx context.Context, sessionID, msg, resp string) error {
			return fmt.Errorf("storage failure")
		},
	}

	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{Content: "my response"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, engine)
	testTask := task.NewTask(task.NewJobID(), "hello", "line", "U123")

	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat should succeed even when EndTurn fails: %v", err)
	}
	if resp != "my response" {
		t.Errorf("response should still be returned: want 'my response', got %q", resp)
	}
}

// === Web search tests ===

func TestMioAgent_Chat_WebSearchTriggered(t *testing.T) {
	searchCalled := false
	toolRunner := &mockToolRunner{
		executeFunc: func(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
			if toolName == "web_search" {
				searchCalled = true
				return "search results", nil
			}
			return "", nil
		},
	}

	var capturedReq llm.GenerateRequest
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			capturedReq = req
			return llm.GenerateResponse{Content: "answer"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, toolRunner, &mockMCPClient{}, nil)
	testTask := task.NewTask(task.NewJobID(), "Go言語について教えて", "line", "U123")

	_, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if !searchCalled {
		t.Error("web_search should have been called for message with '教えて'")
	}
	// Verify search results injected into messages
	hasSearchContext := false
	for _, msg := range capturedReq.Messages {
		if strings.Contains(msg.Content, "Web検索の結果") {
			hasSearchContext = true
			break
		}
	}
	if !hasSearchContext {
		t.Error("search results should be injected into LLM messages")
	}
}

func TestMioAgent_Chat_WebSearchNotTriggered(t *testing.T) {
	searchCalled := false
	toolRunner := &mockToolRunner{
		executeFunc: func(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
			if toolName == "web_search" {
				searchCalled = true
			}
			return "", nil
		},
	}

	provider := &mockLLMProvider{}
	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, toolRunner, &mockMCPClient{}, nil)
	testTask := task.NewTask(task.NewJobID(), "こんにちは", "line", "U123")

	_, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if searchCalled {
		t.Error("web_search should NOT be called for simple greeting")
	}
}

func TestMioAgent_Chat_WebSearchError(t *testing.T) {
	toolRunner := &mockToolRunner{
		executeFunc: func(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
			return "", fmt.Errorf("API error")
		},
	}

	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{Content: "response without search"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, toolRunner, &mockMCPClient{}, nil)
	testTask := task.NewTask(task.NewJobID(), "最新のニュースを検索して", "line", "U123")

	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat should succeed even when web search fails: %v", err)
	}
	if resp != "response without search" {
		t.Errorf("response: want 'response without search', got %q", resp)
	}
}

// === LLM error test ===

func TestMioAgent_Chat_LLMError(t *testing.T) {
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{}, fmt.Errorf("LLM unavailable")
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	testTask := task.NewTask(task.NewJobID(), "hello", "line", "U123")

	_, err := mio.Chat(context.Background(), testTask)
	if err == nil {
		t.Fatal("Chat should return error when LLM fails")
	}
	if !strings.Contains(err.Error(), "LLM unavailable") {
		t.Errorf("error should contain 'LLM unavailable', got: %v", err)
	}
}

// === Command parsing tests ===

func TestParseExplicitCommand_AllRoutes(t *testing.T) {
	mio := NewMioAgent(&mockLLMProvider{}, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)

	tests := []struct {
		message string
		route   routing.Route
	}{
		{"/chat hello", routing.RouteCHAT},
		{"/plan create project", routing.RoutePLAN},
		{"/analyze logs", routing.RouteANALYZE},
		{"/ops deploy", routing.RouteOPS},
		{"/research topic", routing.RouteRESEARCH},
		{"/code fix bug", routing.RouteCODE},
		{"/code1 design spec", routing.RouteCODE1},
		{"/code2 implement feature", routing.RouteCODE2},
		{"/code3 review code", routing.RouteCODE3},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			result := mio.parseExplicitCommand(tt.message)
			if result != tt.route {
				t.Errorf("parseExplicitCommand(%q): want %s, got %s", tt.message, tt.route, result)
			}
		})
	}
}

func TestParseExplicitCommand_PrefixOverlap(t *testing.T) {
	mio := NewMioAgent(&mockLLMProvider{}, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)

	// /code3 should match CODE3, not CODE
	result := mio.parseExplicitCommand("/code3 task")
	if result != routing.RouteCODE3 {
		t.Errorf("/code3 should match CODE3, got %s", result)
	}
}

func TestParseExplicitCommand_EmptyMessage(t *testing.T) {
	mio := NewMioAgent(&mockLLMProvider{}, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	result := mio.parseExplicitCommand("")
	if result != "" {
		t.Errorf("empty message should return empty route, got %s", result)
	}
}

func TestParseExplicitCommand_NoCommand(t *testing.T) {
	mio := NewMioAgent(&mockLLMProvider{}, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	result := mio.parseExplicitCommand("hello world")
	if result != "" {
		t.Errorf("non-command message should return empty route, got %s", result)
	}
}

func TestParseExplicitCommand_LeadingSpaces(t *testing.T) {
	mio := NewMioAgent(&mockLLMProvider{}, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	result := mio.parseExplicitCommand("  /chat hello")
	if result != routing.RouteCHAT {
		t.Errorf("leading spaces should be trimmed, got %s", result)
	}
}

// === cleanSearchQuery tests ===

func TestCleanSearchQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Go言語について教えて", "Go言語"},
		{"最新のニュースを検索して", "最新のニュースして"}, // "を検索" removed first, "して" remains
		{"Rustとは", "Rust"},
		{"hello", "hello"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cleanSearchQuery(tt.input)
			if got != tt.want {
				t.Errorf("cleanSearchQuery(%q): want %q, got %q", tt.input, tt.want, got)
			}
		})
	}
}

func TestInferDomain(t *testing.T) {
	tests := []struct {
		query string
		want  string
	}{
		// プログラミング関連
		{"Rustについて教えて", "programming"},
		{"Pythonのコード例", "programming"},
		{"Goの関数", "programming"},
		{"JavaScriptの変数", "programming"},
		{"アルゴリズムを調べて", "programming"},

		// エンターテイメント関連
		{"最新の映画", "entertainment"},
		{"人気のアニメ", "entertainment"},
		{"ゲームのレビュー", "entertainment"},
		{"音楽について", "entertainment"},

		// 料理関連
		{"カレーのレシピ", "cooking"},
		{"食材の選び方", "cooking"},
		{"レストラン情報", "cooking"},

		// 科学関連
		{"量子力学について", "science"},
		{"AIの技術", "science"},
		{"機械学習のアルゴリズム", "programming"}, // programming が優先

		// 一般
		{"天気について", "general"},
		{"ニュース", "general"},
		{"こんにちは", "general"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := inferDomain(tt.query)
			if got != tt.want {
				t.Errorf("inferDomain(%q): want %q, got %q", tt.query, tt.want, got)
			}
		})
	}
}

func TestWithConversationManager(t *testing.T) {
	provider := &mockLLMProvider{}
	classifier := &mockClassifier{}
	ruleDict := &mockRuleDictionary{}
	toolRunner := &mockToolRunner{}
	mcpClient := &mockMCPClient{}

	mio := NewMioAgent(provider, classifier, ruleDict, toolRunner, mcpClient, nil)

	// WithConversationManager should return the same agent instance
	mockConvMgr := &mockConversationManager{}
	result := mio.WithConversationManager(mockConvMgr)

	if result != mio {
		t.Error("WithConversationManager should return the same agent instance")
	}

	// Verify the manager was set by checking if Process can use it
	// (This is indirectly verified through integration tests)
}

// mockConversationManager は ConversationManager のモック
type mockConversationManager struct {
	saveWebSearchCalled bool
	searchKBCalled      bool
}

func (m *mockConversationManager) SaveWebSearchToKB(ctx context.Context, domain string, query string, results []WebSearchResult) error {
	m.saveWebSearchCalled = true
	return nil
}

func (m *mockConversationManager) SearchKB(ctx context.Context, domain string, query string, topK int) ([]*conversation.Document, error) {
	m.searchKBCalled = true
	return []*conversation.Document{}, nil
}

// === Persona self-edit tests ===

// mockPersonaEditor はPersonaEditorのモック
type mockPersonaEditor struct {
	content    string
	readErr    error
	writeErr   error
	writeCalls int
	lastWrite  string
}

func (m *mockPersonaEditor) ReadPersona() (string, error) {
	if m.readErr != nil {
		return "", m.readErr
	}
	return m.content, nil
}

func (m *mockPersonaEditor) WritePersona(content string) error {
	m.writeCalls++
	m.lastWrite = content
	if m.writeErr != nil {
		return m.writeErr
	}
	m.content = content
	return nil
}

func TestDetectPersonaEditIntent(t *testing.T) {
	tests := []struct {
		message string
		want    bool
	}{
		// Should trigger (topic + action)
		{"口調をカジュアルにして", true},
		{"敬語やめて", true},
		{"ペルソナを修正して", true},
		{"話し方を変えて", true},
		{"キャラを調整して", true},
		{"もっとカジュアルにして", true},
		{"テンションを変えて", true},
		{"語尾を直して", true},
		// Should NOT trigger (topic only, no action)
		{"口調はどんな感じ？", false},
		{"ペルソナって何？", false},
		// Should NOT trigger (action only, no topic)
		{"設定を変えて", false},
		{"ファイルを修正して", false},
		// Should NOT trigger (unrelated)
		{"こんにちは", false},
		{"天気を教えて", false},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			got := detectPersonaEditIntent(tt.message)
			if got != tt.want {
				t.Errorf("detectPersonaEditIntent(%q): want %v, got %v", tt.message, tt.want, got)
			}
		})
	}
}

func TestMioAgent_Chat_PersonaEdit(t *testing.T) {
	editor := &mockPersonaEditor{
		content: "旧ペルソナ設定",
	}

	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			// LLM should receive prompt with current persona + user request
			if !strings.Contains(req.Messages[0].Content, "旧ペルソナ設定") {
				t.Error("LLM prompt should contain current persona")
			}
			return llm.GenerateResponse{Content: "新ペルソナ設定"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	mio = mio.WithPersonaEditor(editor)

	testTask := task.NewTask(task.NewJobID(), "口調をカジュアルにして", "line", "U123")
	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if !strings.Contains(resp, "ペルソナ設定を更新") {
		t.Errorf("response should confirm persona update, got %q", resp)
	}
	if editor.writeCalls != 1 {
		t.Errorf("WritePersona should be called once, called %d times", editor.writeCalls)
	}
	if editor.lastWrite != "新ペルソナ設定" {
		t.Errorf("written persona: want '新ペルソナ設定', got %q", editor.lastWrite)
	}
}

func TestMioAgent_Chat_PersonaEditFallback(t *testing.T) {
	// PersonaEditor が nil の場合は通常の会話として処理
	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{Content: "通常の応答"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	// PersonaEditor is nil (not set)

	testTask := task.NewTask(task.NewJobID(), "口調をカジュアルにして", "line", "U123")
	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat should succeed without PersonaEditor: %v", err)
	}
	if resp != "通常の応答" {
		t.Errorf("should fall back to normal chat, got %q", resp)
	}
}

func TestMioAgent_Chat_PersonaEditReadError(t *testing.T) {
	editor := &mockPersonaEditor{
		readErr: fmt.Errorf("file not found"),
	}

	provider := &mockLLMProvider{
		generateFunc: func(ctx context.Context, req llm.GenerateRequest) (llm.GenerateResponse, error) {
			return llm.GenerateResponse{Content: "通常の応答"}, nil
		},
	}

	mio := NewMioAgent(provider, &mockClassifier{}, &mockRuleDictionary{}, &mockToolRunner{}, &mockMCPClient{}, nil)
	mio = mio.WithPersonaEditor(editor)

	testTask := task.NewTask(task.NewJobID(), "口調をカジュアルにして", "line", "U123")
	resp, err := mio.Chat(context.Background(), testTask)
	if err != nil {
		t.Fatalf("Chat should succeed with persona read error (fallback): %v", err)
	}
	// Should fall back to normal chat
	if resp != "通常の応答" {
		t.Errorf("should fall back to normal chat, got %q", resp)
	}
}

func TestGetStringField(t *testing.T) {
	tests := []struct {
		name     string
		m        map[string]any
		key      string
		expected string
	}{
		{
			name:     "valid string field",
			m:        map[string]any{"name": "test"},
			key:      "name",
			expected: "test",
		},
		{
			name:     "missing field",
			m:        map[string]any{"other": "value"},
			key:      "name",
			expected: "",
		},
		{
			name:     "non-string field",
			m:        map[string]any{"count": 123},
			key:      "count",
			expected: "",
		},
		{
			name:     "nil map",
			m:        nil,
			key:      "name",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStringField(tt.m, tt.key)
			if got != tt.expected {
				t.Errorf("getStringField(%v, %q): want %q, got %q", tt.m, tt.key, tt.expected, got)
			}
		})
	}
}

