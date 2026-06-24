package main

import "testing"

func TestSelectChatConversationProviderPrefersChatWorker(t *testing.T) {
	chat := fakeConversationProvider{name: "chat-provider"}
	chatWorker := fakeConversationProvider{name: "chatworker-provider"}

	got := selectChatConversationProvider(chatWorker, chat)
	if got == nil || got.Name() != "chatworker-provider" {
		t.Fatalf("provider = %#v, want chatworker-provider", got)
	}
}

func TestSelectChatConversationProviderFallsBackToChat(t *testing.T) {
	chat := fakeConversationProvider{name: "chat-provider"}

	got := selectChatConversationProvider(nil, chat)
	if got == nil || got.Name() != "chat-provider" {
		t.Fatalf("provider = %#v, want chat-provider", got)
	}
}
