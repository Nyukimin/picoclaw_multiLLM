package tts

import (
	"testing"
	"time"
)

func TestPendingPlaybackStoreCompleteByResponseClosesWaitAndTopicGate(t *testing.T) {
	store := NewPendingPlaybackStore()
	wait := store.Register("tts-1", "resp-1")
	store.RegisterTopicGate("idle-1", "tts-1")

	action := store.CompleteByResponse("resp-1")
	if !action.Matched || action.ClearPublicBy != "resp-1" || !action.ClosePendingWait || !action.CloseTopicGate {
		t.Fatalf("unexpected action: %+v", action)
	}
	select {
	case <-wait:
	case <-time.After(time.Second):
		t.Fatal("pending wait should close")
	}
	if got := store.Snapshot(); got.PendingSessionCount != 0 || got.TopicGateCount != 0 {
		t.Fatalf("pending state should be empty: %+v", got)
	}
}

func TestPendingPlaybackStoreClearByWait(t *testing.T) {
	store := NewPendingPlaybackStore()
	wait := store.Register("tts-1", "resp-1")
	action := store.ClearByWait(wait)
	if !action.Matched || action.ClearPublicSession != "tts-1" {
		t.Fatalf("unexpected action: %+v", action)
	}
	select {
	case <-wait:
	case <-time.After(time.Second):
		t.Fatal("pending wait should close")
	}
}

func TestPendingPlaybackStoreCompleteByResponseClosesWaitWhenSessionIDWasReused(t *testing.T) {
	store := NewPendingPlaybackStore()
	first := store.Register("tts-reused", "resp-1")
	second := store.Register("tts-reused", "resp-2")

	firstAction := store.CompleteByResponse("resp-1")
	if !firstAction.Matched || !firstAction.ClosePendingWait || firstAction.ClearPublicBy != "resp-1" {
		t.Fatalf("unexpected first action: %+v", firstAction)
	}
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first wait should close even after the tts session id was reused")
	}

	secondAction := store.CompleteByResponse("resp-2")
	if !secondAction.Matched || secondAction.TTSSessionID != "tts-reused" {
		t.Fatalf("unexpected second action: %+v", secondAction)
	}
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second wait should close")
	}
}

func TestPendingPlaybackStoreClearAll(t *testing.T) {
	store := NewPendingPlaybackStore()
	first := store.Register("tts-1", "resp-1")
	second := store.Register("tts-2", "resp-2")
	store.RegisterTopicGate("idle-1", "tts-1")
	sessions := store.ClearAll()
	if len(sessions) != 2 {
		t.Fatalf("sessions = %#v, want 2", sessions)
	}
	for name, ch := range map[string]<-chan struct{}{"first": first, "second": second} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("%s wait should close", name)
		}
	}
	if got := store.Snapshot(); got.PendingSessionCount != 0 || got.PendingResponseCount != 0 || got.TopicGateCount != 0 {
		t.Fatalf("pending state should be empty: %+v", got)
	}
}
