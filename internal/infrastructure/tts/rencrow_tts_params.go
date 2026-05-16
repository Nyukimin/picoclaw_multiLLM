package tts

import (
	"fmt"
	"strings"

	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

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
