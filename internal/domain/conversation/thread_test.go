package conversation

import (
	"testing"
	"time"
)

func TestNewThread(t *testing.T) {
	sessionID := "test-session-001"
	domain := "programming"

	thread := NewThread(sessionID, domain)

	if thread.SessionID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, thread.SessionID)
	}

	if thread.Domain != domain {
		t.Errorf("Expected domain %s, got %s", domain, thread.Domain)
	}

	if thread.ID == 0 {
		t.Error("Expected non-zero thread ID")
	}

	if thread.Status != ThreadActive {
		t.Errorf("Expected status %s, got %s", ThreadActive, thread.Status)
	}

	if len(thread.Turns) != 0 {
		t.Errorf("Expected empty turns, got %d", len(thread.Turns))
	}

	if thread.EndTime != nil {
		t.Error("Expected nil EndTime for new thread")
	}
}

func TestThreadAddMessage(t *testing.T) {
	thread := NewThread("session-001", "test")

	msg1 := NewMessage(SpeakerUser, "Hello", nil)
	thread.AddMessage(msg1)

	if len(thread.Turns) != 1 {
		t.Errorf("Expected 1 turn, got %d", len(thread.Turns))
	}

	msg2 := NewMessage(SpeakerMio, "Hi there!", nil)
	thread.AddMessage(msg2)

	if len(thread.Turns) != 2 {
		t.Errorf("Expected 2 turns, got %d", len(thread.Turns))
	}

	if thread.Turns[0].Msg != "Hello" {
		t.Errorf("Expected first message 'Hello', got '%s'", thread.Turns[0].Msg)
	}

	if thread.Turns[1].Msg != "Hi there!" {
		t.Errorf("Expected second message 'Hi there!', got '%s'", thread.Turns[1].Msg)
	}
}

func TestThreadAddMessageMaxLimit(t *testing.T) {
	thread := NewThread("session-001", "test")

	// 15件のメッセージを追加（上限12件）
	for i := 0; i < 15; i++ {
		msg := NewMessage(SpeakerUser, "Message", nil)
		thread.AddMessage(msg)
	}

	// 最新12件のみ保持
	if len(thread.Turns) != 12 {
		t.Errorf("Expected 12 turns (max limit), got %d", len(thread.Turns))
	}
}

func TestThreadClose(t *testing.T) {
	thread := NewThread("session-001", "test")

	if thread.Status != ThreadActive {
		t.Errorf("Expected status %s, got %s", ThreadActive, thread.Status)
	}

	if thread.EndTime != nil {
		t.Error("Expected nil EndTime for active thread")
	}

	before := time.Now()
	thread.Close()
	after := time.Now()

	if thread.Status != ThreadClosed {
		t.Errorf("Expected status %s, got %s", ThreadClosed, thread.Status)
	}

	if thread.EndTime == nil {
		t.Fatal("Expected non-nil EndTime after close")
	}

	if thread.EndTime.Before(before) || thread.EndTime.After(after) {
		t.Error("EndTime should be between before and after")
	}
}

func TestThreadStatusConstants(t *testing.T) {
	statuses := []ThreadStatus{
		ThreadActive,
		ThreadClosed,
		ThreadArchived,
	}

	expected := []string{
		"active",
		"closed",
		"archived",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("Expected status %s, got %s", expected[i], status)
		}
	}
}

func TestGenerateThreadID(t *testing.T) {
	id1 := generateThreadID()
	time.Sleep(1 * time.Millisecond)
	id2 := generateThreadID()

	if id1 == id2 {
		t.Error("Expected unique thread IDs")
	}

	if id1 >= id2 {
		t.Error("Expected increasing thread IDs")
	}
}
