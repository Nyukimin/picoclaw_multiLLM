package tts

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

type SBV2TTSBridgeConfig struct {
	Provider           Provider
	Sink               AudioSink
	OutputDir          string
	OnChunkReady       func(sessionID, responseID string, chunkIndex int, characterID, text, audioPath, audioURL string)
	OnSessionCompleted func(sessionID, characterID string)
}

type sbv2BridgeSession struct {
	characterID string
	responseID  string
	voiceID     string
	nextChunk   int
}

type SBV2TTSBridge struct {
	cfg      SBV2TTSBridgeConfig
	mu       sync.Mutex
	sessions map[string]*sbv2BridgeSession
}

func NewSBV2TTSBridge(cfg SBV2TTSBridgeConfig) *SBV2TTSBridge {
	return &SBV2TTSBridge{
		cfg:      cfg,
		sessions: make(map[string]*sbv2BridgeSession),
	}
}

func (b *SBV2TTSBridge) StartSession(_ context.Context, req orchestrator.TTSSessionStart) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	b.mu.Lock()
	b.sessions[req.SessionID] = &sbv2BridgeSession{
		characterID: strings.TrimSpace(req.CharacterID),
		responseID:  strings.TrimSpace(req.ResponseID),
		voiceID:     strings.TrimSpace(req.VoiceID),
	}
	b.mu.Unlock()
	return nil
}

func (b *SBV2TTSBridge) PushText(ctx context.Context, sessionID string, text string, _ *ttsapp.EmotionState) error {
	if b.cfg.Provider == nil {
		return fmt.Errorf("sbv2 provider is not configured")
	}
	rawText := strings.TrimSpace(text)
	if rawText == "" {
		return nil
	}
	s := b.getOrCreateSession(sessionID)
	out, err := b.cfg.Provider.Synthesize(ctx, SynthesisInput{
		Text:       rawText,
		OutputDir:  b.cfg.OutputDir,
		FilePrefix: "viewer-tts",
		VoiceProfile: VoiceProfile{
			VoiceID: s.voiceID,
		},
	})
	if err != nil {
		return err
	}
	ch := audioChunk{
		ChunkIndex: s.nextChunk,
		Text:       rawText,
		AudioPath:  localAudioPathForViewer(b.cfg.OutputDir, out.AudioFilePath),
		AudioURL:   "",
		PauseAfter: chunkPauseForText(rawText),
	}
	s.nextChunk++
	if b.cfg.OnChunkReady != nil {
		b.cfg.OnChunkReady(sessionID, s.responseID, ch.ChunkIndex, s.characterID, ch.Text, ch.AudioPath, ch.AudioURL)
	}
	if b.cfg.Sink != nil {
		return b.cfg.Sink.SubmitChunk(ctx, sessionID, ch)
	}
	return nil
}

func localAudioPathForViewer(outputDir, audioPath string) string {
	outputDir = strings.TrimSpace(outputDir)
	audioPath = strings.TrimSpace(audioPath)
	if outputDir == "" || audioPath == "" {
		return audioPath
	}
	base, err := filepath.Abs(outputDir)
	if err != nil {
		return audioPath
	}
	candidate := audioPath
	if !filepath.IsAbs(candidate) {
		candidate, err = filepath.Abs(candidate)
		if err != nil {
			return audioPath
		}
	}
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(candidate))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return audioPath
	}
	return filepath.ToSlash(rel)
}

func (b *SBV2TTSBridge) EndSession(ctx context.Context, sessionID string) error {
	var characterID string
	b.mu.Lock()
	if s, ok := b.sessions[sessionID]; ok && s != nil {
		characterID = s.characterID
	}
	delete(b.sessions, sessionID)
	b.mu.Unlock()
	if b.cfg.Sink != nil {
		if err := b.cfg.Sink.CompleteSession(ctx, sessionID); err != nil {
			return err
		}
	}
	if b.cfg.OnSessionCompleted != nil {
		b.cfg.OnSessionCompleted(sessionID, characterID)
	}
	return nil
}

func (b *SBV2TTSBridge) getOrCreateSession(sessionID string) *sbv2BridgeSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.sessions[sessionID]; ok {
		return s
	}
	s := &sbv2BridgeSession{}
	b.sessions[sessionID] = s
	return s
}
