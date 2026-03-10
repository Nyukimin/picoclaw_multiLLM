package orchestrator

import "context"

// TTSSessionStart describes one TTS streaming session metadata.
type TTSSessionStart struct {
	SessionID             string
	ResponseID            string
	VoiceID               string
	SpeechMode            string
	Event                 string
	Urgency               string
	ConversationMode      string
	UserAttentionRequired bool
}

// TTSBridge streams response text to an external TTS server.
type TTSBridge interface {
	StartSession(ctx context.Context, req TTSSessionStart) error
	PushText(ctx context.Context, sessionID string, text string) error
	EndSession(ctx context.Context, sessionID string) error
}
