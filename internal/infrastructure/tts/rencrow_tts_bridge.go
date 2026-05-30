package tts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

var allowedProviderParamKeys = map[string]struct{}{
	"model_name":     {},
	"model_file":     {},
	"speaker_id":     {},
	"speaker_name":   {},
	"style":          {},
	"style_weight":   {},
	"language":       {},
	"sdp_ratio":      {},
	"noise":          {},
	"noise_w":        {},
	"split_interval": {},
	"line_split":     {},
	"length":         {},
}

const defaultMaxTextLength = 1000
const defaultSynthesisTimeout = 30 * time.Second

type RenCrowTTSBridgeConfig struct {
	HTTPBaseURL        string
	VoiceID            string
	TLSSkipVerify      bool
	RequestTimeout     time.Duration
	ProviderParams     map[string]any
	Sink               AudioSink
	OnChunkReady       func(sessionID, responseID string, chunkIndex int, characterID, text, displayText, audioPath, audioURL string)
	OnSessionCompleted func(sessionID, characterID string)
}

type renCrowTTSSession struct {
	characterID string
	responseID  string
	voiceID     string
	nextChunk   int
}

type RenCrowTTSBridge struct {
	cfg      RenCrowTTSBridgeConfig
	client   *http.Client
	mu       sync.Mutex
	sessions map[string]*renCrowTTSSession
}

func NewRenCrowTTSBridge(cfg RenCrowTTSBridgeConfig) *RenCrowTTSBridge {
	if strings.TrimSpace(cfg.VoiceID) == "" {
		cfg.VoiceID = "female_01"
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultSynthesisTimeout
	}
	if cfg.ProviderParams == nil {
		cfg.ProviderParams = map[string]any{}
	}
	transport := &http.Transport{}
	if cfg.TLSSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &RenCrowTTSBridge{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.RequestTimeout, Transport: transport},
		sessions: make(map[string]*renCrowTTSSession),
	}
}

func (b *RenCrowTTSBridge) StartSession(_ context.Context, req orchestrator.TTSSessionStart) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	b.mu.Lock()
	b.sessions[req.SessionID] = &renCrowTTSSession{
		characterID: strings.TrimSpace(req.CharacterID),
		responseID:  strings.TrimSpace(req.ResponseID),
		voiceID:     chooseNonEmpty(req.VoiceID, b.cfg.VoiceID),
		nextChunk:   0,
	}
	b.mu.Unlock()
	return nil
}

func (b *RenCrowTTSBridge) PushText(ctx context.Context, sessionID string, text string, emotion *ttsapp.EmotionState) error {
	return b.PushTextWithDisplay(ctx, sessionID, text, text, emotion)
}

func (b *RenCrowTTSBridge) PushTextWithDisplay(ctx context.Context, sessionID string, text string, displayText string, emotion *ttsapp.EmotionState) error {
	rawText := strings.TrimSpace(text)
	if rawText == "" {
		return nil
	}
	if utf8.RuneCountInString(rawText) > defaultMaxTextLength {
		return invalidRequestError("text exceeds max_text_length")
	}
	plan := planTTSChunks(rawText, displayText)
	if len(plan) == 0 {
		return nil
	}

	session := b.getOrCreateSession(sessionID)
	characterID := session.characterID
	responseID := session.responseID
	voiceID := chooseNonEmpty(session.voiceID, b.cfg.VoiceID)

	for _, item := range plan {
		speechText := ttsapp.EnsureEmotionPrefixForCharacter(item.SpeechText, emotion, characterID)
		payload := map[string]any{
			"text":     speechText,
			"voice_id": fallbackVoiceID(voiceID, emotion),
		}
		if speed, ok := extractSpeechSpeed(emotion); ok {
			if speed <= 0 {
				return invalidRequestError("speed must be > 0")
			}
			payload["speed"] = speed
		}
		if pitch, ok := extractSpeechPitch(emotion); ok {
			payload["pitch"] = pitch
		}
		if len(b.cfg.ProviderParams) > 0 {
			filtered, err := filterProviderParams(b.cfg.ProviderParams)
			if err != nil {
				return invalidRequestError(err.Error())
			}
			if len(filtered) > 0 {
				payload["provider_params"] = filtered
			}
		}

		reqBody, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal /synthesis request: %w", err)
		}
		body, err := b.postSynthesisWithRetry(ctx, reqBody, sessionID, session.nextChunk)
		if err != nil {
			return err
		}

		var out struct {
			RequestID string `json:"request_id"`
			AudioPath string `json:"audio_path"`
			AudioURL  string `json:"audio_url"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return fmt.Errorf("decode /synthesis response: %w", err)
		}
		if strings.TrimSpace(out.AudioPath) == "" && strings.TrimSpace(out.AudioURL) == "" {
			return fmt.Errorf("/synthesis response missing audio_path/audio_url")
		}

		audioURL := resolveAudioURL(mediaBaseURL(b.cfg.HTTPBaseURL), out.AudioPath, out.AudioURL)
		ch := audioChunk{
			ChunkIndex: session.nextChunk,
			Text:       speechText,
			AudioPath:  out.AudioPath,
			AudioURL:   audioURL,
			PauseAfter: chunkPauseForText(speechText),
		}
		session.nextChunk++

		if b.cfg.OnChunkReady != nil {
			b.cfg.OnChunkReady(sessionID, responseID, ch.ChunkIndex, characterID, speechText, strings.TrimSpace(item.DisplayText), ch.AudioPath, ch.AudioURL)
		}
		if b.cfg.Sink != nil {
			if err := b.cfg.Sink.SubmitChunk(ctx, sessionID, ch); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *RenCrowTTSBridge) EndSession(ctx context.Context, sessionID string) error {
	if b == nil {
		return nil
	}
	var characterID string
	b.mu.Lock()
	if session, ok := b.sessions[sessionID]; ok && session != nil {
		characterID = strings.TrimSpace(session.characterID)
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
