package main

import (
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
)

func TestShouldStopIdleChatByEvent(t *testing.T) {
	tests := []struct {
		name string
		ev   orchestrator.OrchestratorEvent
		want bool
	}{
		{
			name: "user message stops idlechat",
			ev:   orchestrator.NewEvent("message.received", "user", "mio", "hi", "", "", "", "", ""),
			want: true,
		},
		{
			name: "idlechat timeline does not stop itself",
			ev:   orchestrator.NewEvent("idlechat.message", "mio", "shiro", "hi", "IDLECHAT", "", "", "", ""),
			want: false,
		},
		{
			name: "tts audio chunk does not stop idlechat",
			ev:   orchestrator.NewEvent("tts.audio_chunk", "tts", "user", "{}", "TTS", "", "", "", ""),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldStopIdleChatByEvent(tt.ev); got != tt.want {
				t.Fatalf("shouldStopIdleChatByEvent() = %v, want %v", got, tt.want)
			}
		})
	}
}
