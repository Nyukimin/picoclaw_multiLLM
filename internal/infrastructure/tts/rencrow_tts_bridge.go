package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

type RenCrowTTSBridgeConfig struct {
	HTTPBaseURL        string
	VoiceID            string
	RequestTimeout     time.Duration
	ProviderParams     map[string]any
	Sink               AudioSink
	OnChunkReady       func(sessionID string, chunkIndex int, characterID, text, audioPath, audioURL string)
	OnSessionCompleted func(sessionID string)
}

type renCrowTTSSession struct {
	characterID string
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
		cfg.RequestTimeout = 15 * time.Second
	}
	if cfg.ProviderParams == nil {
		cfg.ProviderParams = map[string]any{}
	}
	return &RenCrowTTSBridge{
		cfg:      cfg,
		client:   &http.Client{Timeout: cfg.RequestTimeout},
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
		voiceID:     chooseNonEmpty(req.VoiceID, b.cfg.VoiceID),
		nextChunk:   0,
	}
	b.mu.Unlock()
	return nil
}

func (b *RenCrowTTSBridge) PushText(ctx context.Context, sessionID string, text string, emotion *ttsapp.EmotionState) error {
	rawText := strings.TrimSpace(text)
	if rawText == "" {
		return nil
	}
	if utf8.RuneCountInString(rawText) > defaultMaxTextLength {
		return invalidRequestError("text exceeds max_text_length")
	}
	text = ensureTTSPunctuation(rawText)

	session := b.getOrCreateSession(sessionID)
	characterID := session.characterID
	voiceID := chooseNonEmpty(session.voiceID, b.cfg.VoiceID)

	payload := map[string]any{
		"text":     text,
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeSynthesisURL(b.cfg.HTTPBaseURL), bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("build /synthesis request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-RenCrow-TTS-Request-Id", buildRequestIDHeader(sessionID, session.nextChunk))

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("/synthesis request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return fmt.Errorf("read /synthesis response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code, message := parseSynthesisError(body)
		if code == "" {
			return fmt.Errorf("/synthesis bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return fmt.Errorf("/synthesis failed status=%d code=%s message=%s", resp.StatusCode, code, message)
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
		Text:       text,
		AudioPath:  out.AudioPath,
		AudioURL:   audioURL,
		PauseAfter: chunkPauseForText(text),
	}
	session.nextChunk++

	if b.cfg.OnChunkReady != nil {
		b.cfg.OnChunkReady(sessionID, ch.ChunkIndex, characterID, text, ch.AudioPath, ch.AudioURL)
	}
	if b.cfg.Sink != nil {
		if err := b.cfg.Sink.SubmitChunk(ctx, sessionID, ch); err != nil {
			return err
		}
	}
	return nil
}

func (b *RenCrowTTSBridge) EndSession(ctx context.Context, sessionID string) error {
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

func (b *RenCrowTTSBridge) getOrCreateSession(sessionID string) *renCrowTTSSession {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.sessions[sessionID]; ok {
		return s
	}
	s := &renCrowTTSSession{voiceID: b.cfg.VoiceID}
	b.sessions[sessionID] = s
	return s
}

func normalizeSynthesisURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(strings.ToLower(base), "/synthesis") {
		return base
	}
	return base + "/synthesis"
}

func mediaBaseURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(strings.ToLower(base), "/synthesis") {
		return strings.TrimSuffix(base, "/synthesis")
	}
	return base
}

func parseSynthesisError(body []byte) (string, string) {
	var out struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", ""
	}
	return strings.TrimSpace(out.Error.Code), strings.TrimSpace(out.Error.Message)
}

func copyStringAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func fallbackVoiceID(defaultVoiceID string, emotion *ttsapp.EmotionState) string {
	if emotion != nil {
		switch strings.ToLower(strings.TrimSpace(emotion.ReasonTrace.VoiceProfile)) {
		case "lumina_male":
			return "male_01"
		case "lumina_female":
			return "female_01"
		}
	}
	return defaultVoiceID
}

func extractSpeechSpeed(emotion *ttsapp.EmotionState) (float64, bool) {
	if emotion == nil || emotion.Prosody.Speed == 0 {
		return 0, false
	}
	return emotion.Prosody.Speed, true
}

func extractSpeechPitch(emotion *ttsapp.EmotionState) (float64, bool) {
	if emotion == nil {
		return 0, false
	}
	return emotion.Prosody.Pitch, true
}

func filterProviderParams(in map[string]any) (map[string]any, error) {
	if in == nil {
		return nil, nil
	}
	out := make(map[string]any)
	for k, v := range in {
		if _, ok := allowedProviderParamKeys[k]; !ok {
			return nil, fmt.Errorf("unknown provider_params key: %s", k)
		}
		if k == "length" {
			f, ok := toFloat64(v)
			if !ok || f <= 0 {
				return nil, fmt.Errorf("provider_params.length must be > 0")
			}
		}
		if validProviderParamValue(k, v) {
			out[k] = v
		}
	}
	return out, nil
}

func buildRequestIDHeader(sessionID string, chunkIndex int) string {
	prefix := sanitizeAudioPrefix(sessionID)
	if prefix == "" {
		prefix = "ttsreq"
	}
	return fmt.Sprintf("%s-%04d", prefix, chunkIndex)
}

func validProviderParamValue(key string, value any) bool {
	switch key {
	case "model_name", "model_file", "speaker_name", "style", "language":
		_, ok := value.(string)
		return ok
	case "line_split":
		_, ok := value.(bool)
		return ok
	case "speaker_id":
		return isNumeric(value) || isString(value)
	case "style_weight", "sdp_ratio", "noise", "noise_w", "split_interval", "length":
		return isNumeric(value)
	default:
		return false
	}
}

func isString(v any) bool {
	_, ok := v.(string)
	return ok
}

func isNumeric(v any) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	default:
		return false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func invalidRequestError(message string) error {
	return fmt.Errorf("code=invalid_request message=%s", strings.TrimSpace(message))
}
