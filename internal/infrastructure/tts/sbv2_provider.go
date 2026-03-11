package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type SBV2Config struct {
	BaseURL       string
	VoiceID       string
	Timeout       time.Duration
	AudioPathRoot string
}

type SBV2Provider struct {
	baseURL       string
	voiceID       string
	audioPathRoot string
	client        *http.Client
}

func NewSBV2Provider(cfg SBV2Config) *SBV2Provider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &SBV2Provider{
		baseURL:       strings.TrimRight(cfg.BaseURL, "/"),
		voiceID:       cfg.VoiceID,
		audioPathRoot: cfg.AudioPathRoot,
		client:        &http.Client{Timeout: timeout},
	}
}

func (p *SBV2Provider) Name() string {
	return "sbv2"
}

func (p *SBV2Provider) Synthesize(ctx context.Context, in SynthesisInput) (SynthesisOutput, error) {
	if strings.TrimSpace(p.baseURL) == "" {
		return SynthesisOutput{}, fmt.Errorf("%w: sbv2 base_url is empty", ErrProviderUnavailable)
	}
	if strings.TrimSpace(in.Text) == "" {
		return SynthesisOutput{}, fmt.Errorf("text is required")
	}

	payload := map[string]any{
		"text":     in.Text,
		"voice_id": p.voiceID,
		"emotion":  in.Emotion.Emotion,
		"speed":    in.Emotion.Speed,
		"pitch":    in.Emotion.Pitch,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(b))
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("sbv2 request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return SynthesisOutput{}, fmt.Errorf("sbv2 bad status: %d", resp.StatusCode)
	}

	var out struct {
		AudioPath  string `json:"audio_path"`
		DurationMS int    `json:"duration_ms"`
		VoiceID    string `json:"voice_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return SynthesisOutput{}, fmt.Errorf("decode response: %w", err)
	}
	if strings.TrimSpace(out.AudioPath) == "" {
		return SynthesisOutput{}, fmt.Errorf("sbv2 response missing audio_path")
	}
	voiceID := out.VoiceID
	if voiceID == "" {
		voiceID = p.voiceID
	}
	return SynthesisOutput{
		Provider:      "sbv2",
		VoiceID:       voiceID,
		AudioFilePath: resolveAudioPath(out.AudioPath, p.audioPathRoot),
		DurationMS:    out.DurationMS,
	}, nil
}
