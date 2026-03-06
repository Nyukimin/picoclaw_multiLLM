package conversation

import (
	"testing"
)

func TestRecallPack_HasContext_Empty(t *testing.T) {
	rp := &RecallPack{}
	if rp.HasContext() {
		t.Error("empty RecallPack should not have context")
	}
}

func TestRecallPack_HasContext_WithShortContext(t *testing.T) {
	rp := &RecallPack{
		ShortContext: []Message{{Speaker: SpeakerUser, Msg: "hello"}},
	}
	if !rp.HasContext() {
		t.Error("RecallPack with ShortContext should have context")
	}
}

func TestRecallPack_HasContext_WithMidSummaries(t *testing.T) {
	rp := &RecallPack{
		MidSummaries: []ThreadSummary{{Summary: "test"}},
	}
	if !rp.HasContext() {
		t.Error("RecallPack with MidSummaries should have context")
	}
}

func TestRecallPack_HasContext_WithLongFacts(t *testing.T) {
	rp := &RecallPack{
		LongFacts: []string{"fact1"},
	}
	if !rp.HasContext() {
		t.Error("RecallPack with LongFacts should have context")
	}
}

func TestRecallPack_HasContext_WithKBSnippets(t *testing.T) {
	rp := &RecallPack{
		KBSnippets: []string{"snippet1"},
	}
	if !rp.HasContext() {
		t.Error("RecallPack with KBSnippets should have context")
	}
}

func TestRecallPack_ToPromptMessages_Empty(t *testing.T) {
	rp := &RecallPack{}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 0 {
		t.Errorf("empty RecallPack should produce 0 messages, got %d", len(msgs))
	}
}

func TestRecallPack_ToPromptMessages_PersonaOnly(t *testing.T) {
	rp := &RecallPack{
		Persona: PersonaState{SystemPrompt: "You are Mio."},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (system prompt), got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected role 'system', got %q", msgs[0].Role)
	}
	if msgs[0].Content != "You are Mio." {
		t.Errorf("expected content 'You are Mio.', got %q", msgs[0].Content)
	}
}

func TestRecallPack_ToPromptMessages_WithUserProfile(t *testing.T) {
	rp := &RecallPack{
		Persona: PersonaState{SystemPrompt: "You are Mio."},
		UserProfile: UserProfile{
			Preferences: map[string]string{"lang": "Go"},
			Facts:       []string{},
		},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 system message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected role 'system', got %q", msgs[0].Role)
	}
	// SystemPrompt + UserProfile
	if !contains(msgs[0].Content, "You are Mio.") {
		t.Error("system prompt should contain persona")
	}
	if !contains(msgs[0].Content, "lang: Go") {
		t.Error("system prompt should contain user profile preferences")
	}
}

func TestRecallPack_ToPromptMessages_WithMidSummaries(t *testing.T) {
	rp := &RecallPack{
		MidSummaries: []ThreadSummary{
			{Summary: "Discussed Go testing"},
			{Summary: "Talked about CI/CD"},
		},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 context message, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected role 'system', got %q", msgs[0].Role)
	}
	if !contains(msgs[0].Content, "Discussed Go testing") {
		t.Error("context should contain mid summary 1")
	}
	if !contains(msgs[0].Content, "Talked about CI/CD") {
		t.Error("context should contain mid summary 2")
	}
}

func TestRecallPack_ToPromptMessages_WithLongFacts(t *testing.T) {
	rp := &RecallPack{
		LongFacts: []string{"User prefers Go", "User works at startup"},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 context message, got %d", len(msgs))
	}
	if !contains(msgs[0].Content, "User prefers Go") {
		t.Error("context should contain long fact 1")
	}
	if !contains(msgs[0].Content, "User works at startup") {
		t.Error("context should contain long fact 2")
	}
}

func TestRecallPack_ToPromptMessages_WithKBSnippets(t *testing.T) {
	rp := &RecallPack{
		KBSnippets: []string{"Go is a statically typed language"},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 context message, got %d", len(msgs))
	}
	if !contains(msgs[0].Content, "参考知識") {
		t.Error("context should contain KB header")
	}
	if !contains(msgs[0].Content, "Go is a statically typed language") {
		t.Error("context should contain KB snippet")
	}
}

func TestRecallPack_ToPromptMessages_ShortContextRoles(t *testing.T) {
	rp := &RecallPack{
		ShortContext: []Message{
			{Speaker: SpeakerUser, Msg: "hello"},
			{Speaker: SpeakerMio, Msg: "hi there"},
			{Speaker: SpeakerSystem, Msg: "tool result"},
		},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	expected := []struct {
		role    string
		content string
	}{
		{"user", "hello"},
		{"assistant", "hi there"},
		{"system", "tool result"},
	}
	for i, e := range expected {
		if msgs[i].Role != e.role {
			t.Errorf("msg[%d] role: want %q, got %q", i, e.role, msgs[i].Role)
		}
		if msgs[i].Content != e.content {
			t.Errorf("msg[%d] content: want %q, got %q", i, e.content, msgs[i].Content)
		}
	}
}

func TestRecallPack_ToPromptMessages_FullPack(t *testing.T) {
	rp := &RecallPack{
		Persona:      PersonaState{SystemPrompt: "You are Mio."},
		UserProfile:  UserProfile{Preferences: map[string]string{"theme": "dark"}, Facts: []string{}},
		MidSummaries: []ThreadSummary{{Summary: "Past topic"}},
		LongFacts:    []string{"Long fact"},
		KBSnippets:   []string{"KB info"},
		ShortContext: []Message{
			{Speaker: SpeakerUser, Msg: "recent msg"},
		},
	}
	msgs := rp.ToPromptMessages()
	// Expected: system(persona+profile), system(context), user(shortcontext)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// First: persona system prompt
	if msgs[0].Role != "system" {
		t.Errorf("msg[0] role: want 'system', got %q", msgs[0].Role)
	}
	if !contains(msgs[0].Content, "You are Mio.") {
		t.Error("msg[0] should contain persona")
	}
	if !contains(msgs[0].Content, "theme: dark") {
		t.Error("msg[0] should contain user profile")
	}
	// Second: context block
	if msgs[1].Role != "system" {
		t.Errorf("msg[1] role: want 'system', got %q", msgs[1].Role)
	}
	if !contains(msgs[1].Content, "Past topic") {
		t.Error("msg[1] should contain mid summary")
	}
	if !contains(msgs[1].Content, "Long fact") {
		t.Error("msg[1] should contain long fact")
	}
	if !contains(msgs[1].Content, "KB info") {
		t.Error("msg[1] should contain KB snippet")
	}
	// Third: short context
	if msgs[2].Role != "user" {
		t.Errorf("msg[2] role: want 'user', got %q", msgs[2].Role)
	}
	if msgs[2].Content != "recent msg" {
		t.Errorf("msg[2] content: want 'recent msg', got %q", msgs[2].Content)
	}
}

func TestRecallPack_ToPromptMessages_MidAndLongMergedInSameBlock(t *testing.T) {
	rp := &RecallPack{
		MidSummaries: []ThreadSummary{{Summary: "mid1"}},
		LongFacts:    []string{"long1"},
	}
	msgs := rp.ToPromptMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 context message, got %d", len(msgs))
	}
	// Both mid summaries and long facts under same header
	if !contains(msgs[0].Content, "過去の会話から思い出したこと") {
		t.Error("should contain recall header")
	}
	if !contains(msgs[0].Content, "mid1") {
		t.Error("should contain mid summary")
	}
	if !contains(msgs[0].Content, "long1") {
		t.Error("should contain long fact")
	}
}

func TestDefaultConstraints(t *testing.T) {
	c := DefaultConstraints()
	if c.MaxTotalTokens != 8192 {
		t.Errorf("MaxTotalTokens: want 8192, got %d", c.MaxTotalTokens)
	}
	if c.MaxPromptTokens != 4000 {
		t.Errorf("MaxPromptTokens: want 4000, got %d", c.MaxPromptTokens)
	}
	if c.MaxResponseTokens != 512 {
		t.Errorf("MaxResponseTokens: want 512, got %d", c.MaxResponseTokens)
	}
}

// contains is a test helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
