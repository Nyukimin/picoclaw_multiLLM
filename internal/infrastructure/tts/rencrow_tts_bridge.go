package tts

import (
	"bytes"
	"context"
	"crypto/tls"
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
const defaultSynthesisTimeout = 30 * time.Second

type RenCrowTTSBridgeConfig struct {
	HTTPBaseURL        string
	VoiceID            string
	TLSSkipVerify      bool
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
	return normalizeErrorCode(out.Error.Code), strings.TrimSpace(out.Error.Message)
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
		normalized, err := normalizeProviderParamValue(k, v)
		if err != nil {
			return nil, err
		}
		if k == "length" {
			f, ok := toFloat64(normalized)
			if !ok || f <= 0 {
				return nil, fmt.Errorf("provider_params.length must be > 0")
			}
		}
		out[k] = normalized
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

func normalizeProviderParamValue(key string, value any) (any, error) {
	switch key {
	case "model_name", "model_file", "speaker_name", "style", "language":
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("provider_params.%s must be string", key)
		}
		s = strings.TrimSpace(s)
		if key == "language" && !isAllowedLanguage(s) {
			return nil, fmt.Errorf("provider_params.language must be one of JP/EN/ZH")
		}
		return s, nil
	case "line_split":
		if b, ok := value.(bool); ok {
			return b, nil
		}
		if s, ok := value.(string); ok {
			if b, parsed := parseBoolLike(s); parsed {
				return b, nil
			}
		}
		return nil, fmt.Errorf("provider_params.line_split must be bool")
	case "speaker_id":
		if isNumeric(value) || isString(value) {
			return value, nil
		}
		return nil, fmt.Errorf("provider_params.speaker_id must be string or number")
	case "style_weight", "sdp_ratio", "noise", "noise_w", "split_interval", "length":
		if isNumeric(value) {
			return value, nil
		}
		return nil, fmt.Errorf("provider_params.%s must be number", key)
	default:
		return nil, fmt.Errorf("unknown provider_params key: %s", key)
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

func normalizeErrorCode(code string) string {
	code = strings.TrimSpace(strings.ToUpper(code))
	code = strings.ReplaceAll(code, "-", "_")
	return code
}

func shouldRetrySynthesis(code string, attempt int) bool {
	switch normalizeErrorCode(code) {
	case "ENGINE_UNAVAILABLE":
		return attempt < 2
	case "SYNTHESIS_FAILED":
		return attempt < 1
	default:
		return false
	}
}

func backoffForAttempt(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	base := 200 * time.Millisecond
	return time.Duration(1<<attempt) * base
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (b *RenCrowTTSBridge) postSynthesisWithRetry(ctx context.Context, reqBody []byte, sessionID string, chunkIndex int) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeSynthesisURL(b.cfg.HTTPBaseURL), bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("build /synthesis request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-RenCrow-TTS-Request-Id", buildRequestIDHeader(sessionID, chunkIndex))

		resp, err := b.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("/synthesis request failed: %w", err)
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read /synthesis response: %w", readErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return body, nil
		}

		code, message := parseSynthesisError(body)
		if code == "" {
			return nil, fmt.Errorf("/synthesis bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if shouldRetrySynthesis(code, attempt) {
			if err := sleepWithContext(ctx, backoffForAttempt(attempt)); err != nil {
				return nil, fmt.Errorf("/synthesis retry cancelled: %w", err)
			}
			continue
		}
		return nil, fmt.Errorf("/synthesis failed status=%d code=%s message=%s", resp.StatusCode, code, message)
	}
}

func parseBoolLike(v string) (bool, bool) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func isAllowedLanguage(language string) bool {
	switch strings.ToUpper(strings.TrimSpace(language)) {
	case "JP", "JA", "EN", "ZH":
		return true
	default:
		return false
	}
}
