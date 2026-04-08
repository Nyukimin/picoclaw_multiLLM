package tts

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

type SBV2DirectBridgeConfig struct {
	Provider            *SBV2Provider
	Sink                AudioSink
	OutputDir           string
	VoiceID             string
	OnChunkReady        func(sessionID string, chunkIndex int, characterID, text, audioPath, audioURL string)
	OnSessionCompleted  func(sessionID string)
}

type sbv2DirectSession struct {
	characterID string
	voiceID     string
	nextChunk   int
}

// SBV2DirectBridge converts chunked text directly into local SBV2 wav files.
type SBV2DirectBridge struct {
	cfg      SBV2DirectBridgeConfig
	mu       sync.Mutex
	sessions map[string]*sbv2DirectSession
}

func NewSBV2DirectBridge(cfg SBV2DirectBridgeConfig) *SBV2DirectBridge {
	if strings.TrimSpace(cfg.VoiceID) == "" {
		cfg.VoiceID = "female_01"
	}
	return &SBV2DirectBridge{
		cfg:      cfg,
		sessions: make(map[string]*sbv2DirectSession),
	}
}

func (b *SBV2DirectBridge) StartSession(_ context.Context, req orchestrator.TTSSessionStart) error {
	if b == nil || b.cfg.Provider == nil {
		return fmt.Errorf("sbv2 direct bridge is not configured")
	}
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	b.mu.Lock()
	b.sessions[req.SessionID] = &sbv2DirectSession{
		characterID: strings.TrimSpace(req.CharacterID),
		voiceID:     chooseNonEmpty(req.VoiceID, b.cfg.VoiceID),
		nextChunk:   0,
	}
	b.mu.Unlock()
	return nil
}

func (b *SBV2DirectBridge) PushText(ctx context.Context, sessionID string, text string, _ *ttsapp.EmotionState) error {
	if b == nil || b.cfg.Provider == nil {
		return fmt.Errorf("sbv2 direct bridge is not configured")
	}
	text = ensureTTSPunctuation(text)
	if strings.TrimSpace(text) == "" {
		return nil
	}

	session := b.getSession(sessionID)
	chunkIndex := 0
	characterID := ""
	voiceID := b.cfg.VoiceID
	if session != nil {
		characterID = session.characterID
		voiceID = chooseNonEmpty(session.voiceID, voiceID)
		b.mu.Lock()
		chunkIndex = session.nextChunk
		session.nextChunk++
		b.mu.Unlock()
	}

	out, err := b.cfg.Provider.Synthesize(ctx, SynthesisInput{
		Text:       text,
		OutputDir:  b.cfg.OutputDir,
		FilePrefix: sanitizeAudioPrefix(sessionID) + "-chunk",
		VoiceProfile: VoiceProfile{
			VoiceID: voiceID,
		},
	})
	if err != nil {
		return err
	}

	ch := audioChunk{
		ChunkIndex: chunkIndex,
		Text:       text,
		AudioPath:  out.AudioFilePath,
		PauseAfter: chunkPauseForText(text),
	}
	if b.cfg.OnChunkReady != nil {
		b.cfg.OnChunkReady(sessionID, chunkIndex, characterID, text, ch.AudioPath, "")
	}
	if b.cfg.Sink != nil {
		if err := b.cfg.Sink.SubmitChunk(ctx, sessionID, ch); err != nil {
			return err
		}
	}
	return nil
}

func (b *SBV2DirectBridge) EndSession(ctx context.Context, sessionID string) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	delete(b.sessions, sessionID)
	b.mu.Unlock()
	if b.cfg.Sink != nil {
		if err := b.cfg.Sink.CompleteSession(ctx, sessionID); err != nil {
			return err
		}
	}
	if b.cfg.OnSessionCompleted != nil {
		b.cfg.OnSessionCompleted(sessionID)
	}
	return nil
}

func (b *SBV2DirectBridge) getSession(sessionID string) *sbv2DirectSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessions[sessionID]
}
