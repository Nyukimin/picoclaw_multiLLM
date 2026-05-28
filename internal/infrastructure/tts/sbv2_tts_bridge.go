package tts

import (
	"context"
	"fmt"
	"log"
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
	OnChunkReady       func(sessionID, responseID string, chunkIndex int, characterID, text, displayText, audioPath, audioURL string)
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
	return b.PushTextWithDisplay(ctx, sessionID, text, text, nil)
}

func (b *SBV2TTSBridge) PushTextWithDisplay(ctx context.Context, sessionID string, text string, displayText string, _ *ttsapp.EmotionState) error {
	if b.cfg.Provider == nil {
		return fmt.Errorf("sbv2 provider is not configured")
	}
	rawText := strings.TrimSpace(text)
	if rawText == "" {
		return nil
	}
	s := b.getOrCreateSession(sessionID)
	speechChunks := orchestrator.SplitTTSChunks(rawText)
	for i, chunkText := range speechChunks {
		displayChunk := displayChunkForSpeechChunk(rawText, displayText, speechChunks, i)
		out, stats, err := b.synthesizeChunk(ctx, s, chunkText)
		if err != nil {
			return err
		}
		log.Printf("[TTS] chunk_ready session=%s response=%s chunk=%d voice=%s duration_ms=%d rms=%d peak=%d text=%q audio_path=%s",
			sessionID,
			s.responseID,
			s.nextChunk,
			s.voiceID,
			stats.DurationMS,
			stats.RMS,
			stats.Peak,
			chunkText,
			localAudioPathForViewer(b.cfg.OutputDir, out.AudioFilePath),
		)
		ch := audioChunk{
			ChunkIndex: s.nextChunk,
			Text:       chunkText,
			AudioPath:  localAudioPathForViewer(b.cfg.OutputDir, out.AudioFilePath),
			AudioURL:   "",
			PauseAfter: chunkPauseForText(chunkText),
		}
		s.nextChunk++
		if b.cfg.OnChunkReady != nil {
			b.cfg.OnChunkReady(sessionID, s.responseID, ch.ChunkIndex, s.characterID, ch.Text, displayChunk, ch.AudioPath, ch.AudioURL)
		}
		if b.cfg.Sink != nil {
			if err := b.cfg.Sink.SubmitChunk(ctx, sessionID, ch); err != nil {
				return err
			}
		}
	}
	return nil
}

func displayChunkForSpeechChunk(rawSpeechText, rawDisplayText string, speechChunks []string, index int) string {
	if index < 0 || index >= len(speechChunks) {
		return ""
	}
	chunkText := strings.TrimSpace(speechChunks[index])
	displayText := strings.TrimSpace(rawDisplayText)
	speechText := strings.TrimSpace(rawSpeechText)
	if displayText == "" || displayText == speechText {
		return chunkText
	}
	if len(speechChunks) == 1 {
		return displayText
	}
	// Do not independently split display text and speech text; that creates
	// false chunk-index correspondence when the two strings have different
	// boundaries. In multi-chunk pronunciation-normalized cases, keep the
	// diagnostic display chunk tied to the speech chunk.
	return chunkText
}

func (b *SBV2TTSBridge) synthesizeChunk(ctx context.Context, s *sbv2BridgeSession, chunkText string) (SynthesisOutput, wavStats, error) {
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		out, err := b.cfg.Provider.Synthesize(ctx, SynthesisInput{
			Text:       chunkText,
			OutputDir:  b.cfg.OutputDir,
			FilePrefix: "viewer-tts",
			VoiceProfile: VoiceProfile{
				VoiceID: s.voiceID,
			},
		})
		if err != nil {
			lastErr = err
			if attempt == 1 && strings.Contains(err.Error(), "silent") {
				log.Printf("[TTS] retrying near-silent chunk voice=%s text=%q err=%v", s.voiceID, chunkText, err)
				continue
			}
			return SynthesisOutput{}, wavStats{}, err
		}
		stats, ok, err := inspectPCM16WAV(out.AudioFilePath)
		if err != nil {
			return SynthesisOutput{}, wavStats{}, err
		}
		if ok && stats.NearSilent {
			lastErr = fmt.Errorf("%w: generated wav is near silent duration_ms=%d rms=%d peak=%d", ErrSynthesisFailed, stats.DurationMS, stats.RMS, stats.Peak)
			if attempt == 1 {
				log.Printf("[TTS] retrying near-silent chunk voice=%s text=%q audio_path=%s duration_ms=%d rms=%d peak=%d",
					s.voiceID, chunkText, out.AudioFilePath, stats.DurationMS, stats.RMS, stats.Peak)
				continue
			}
			return SynthesisOutput{}, stats, lastErr
		}
		return out, stats, nil
	}
	return SynthesisOutput{}, wavStats{}, lastErr
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
