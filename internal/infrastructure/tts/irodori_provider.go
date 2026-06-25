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

	"github.com/Nyukimin/picoclaw_multiLLM/modules/core"
	moduletts "github.com/Nyukimin/picoclaw_multiLLM/modules/tts"
)

type IrodoriConfig struct {
	BaseURL               string
	EndpointPath          string
	VoiceID               string
	VoiceName             string
	Speed                 float64
	ReferenceAudio        string
	ReferenceAudioURL     string
	Timeout               time.Duration
	Checkpoint            string
	ModelDevice           string
	ModelPrecision        string
	CodecDevice           string
	CodecPrecision        string
	EnableWatermark       bool
	NumSteps              int
	NumCandidates         int
	SeedRaw               string
	CFGGuidanceMode       string
	CFGScaleText          float64
	CFGScaleSpeaker       float64
	CFGScaleRaw           string
	CFGMinT               float64
	CFGMaxT               float64
	ContextKVCache        bool
	TruncationFactorRaw   string
	RescaleKRaw           string
	RescaleSigmaRaw       string
	SpeakerKVScaleRaw     string
	SpeakerKVMinTRaw      string
	SpeakerKVMaxLayersRaw string
}

type IrodoriProvider struct {
	baseURL string
	voiceID string
	client  *http.Client
	cfg     IrodoriConfig
	refMu   sync.Mutex
	refPath string
}

func NewIrodoriProvider(cfg IrodoriConfig) *IrodoriProvider {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &IrodoriProvider{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		voiceID: cfg.VoiceID,
		client:  &http.Client{Timeout: timeout},
		cfg:     withIrodoriDefaults(cfg),
	}
}

func (p *IrodoriProvider) Name() string {
	return "irodori"
}

func (p *IrodoriProvider) Health(ctx context.Context) core.HealthReport {
	healthURL := p.healthURL()
	metadata := map[string]any{
		"provider":    p.Name(),
		"base_url":    p.baseURL,
		"endpoint":    p.cfg.EndpointPath,
		"health_url":  healthURL,
		"health_kind": "http",
	}
	if strings.TrimSpace(p.baseURL) == "" {
		return moduletts.BuildProviderHealth(moduletts.ProviderHealthSnapshot{
			Provider: p.Name(),
			Ready:    false,
			Detail:   "irodori base_url is empty",
			Metadata: metadata,
		})
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, healthURL, nil)
	if err != nil {
		return moduletts.BuildProviderHealth(moduletts.ProviderHealthSnapshot{
			Provider: p.Name(),
			Ready:    false,
			Detail:   "build irodori health request: " + err.Error(),
			Metadata: metadata,
		})
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return moduletts.BuildProviderHealth(moduletts.ProviderHealthSnapshot{
			Provider: p.Name(),
			Ready:    false,
			Detail:   "irodori health failed: " + err.Error(),
			Metadata: metadata,
		})
	}
	defer resp.Body.Close()
	metadata["status_code"] = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return moduletts.BuildProviderHealth(moduletts.ProviderHealthSnapshot{
			Provider: p.Name(),
			Ready:    false,
			Detail:   fmt.Sprintf("irodori health bad status=%d", resp.StatusCode),
			Metadata: metadata,
		})
	}
	return moduletts.BuildProviderHealth(moduletts.ProviderHealthSnapshot{
		Provider: p.Name(),
		Ready:    true,
		Detail:   "irodori external API reachable",
		Metadata: metadata,
	})
}

func (p *IrodoriProvider) healthURL() string {
	base := strings.TrimRight(strings.TrimSpace(p.baseURL), "/")
	if base == "" {
		return ""
	}
	if p.usesGradioCallGeneration() {
		if idx := strings.Index(strings.ToLower(base), "/gradio_api/"); idx >= 0 {
			base = base[:idx]
		}
		return base + "/gradio_api/info"
	}
	return base
}

func (p *IrodoriProvider) Synthesize(ctx context.Context, in SynthesisInput) (SynthesisOutput, error) {
	if strings.TrimSpace(p.baseURL) == "" {
		return SynthesisOutput{}, fmt.Errorf("%w: irodori base_url is empty", ErrProviderUnavailable)
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return SynthesisOutput{}, fmt.Errorf("text is required")
	}
	voiceID := resolveIrodoriVoiceID(moduletts.ChooseNonEmpty(in.VoiceProfile.VoiceID, p.voiceID))
	if p.usesGradioCallGeneration() {
		return p.synthesizeGradioCall(ctx, in, text, voiceID)
	}
	voice := resolveIrodoriVoiceName(moduletts.ChooseNonEmpty(in.VoiceProfile.VoiceID, p.cfg.VoiceName, p.voiceID))
	style := resolveIrodoriStyle(in.Emotion)
	payload := irodoriSynthesisPayload(voice, style, text, p.cfg.Speed)
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("marshal irodori request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.synthesisURL(), bytes.NewReader(reqBody))
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("build irodori request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("irodori request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return SynthesisOutput{}, fmt.Errorf("irodori bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	audioURL, err := parseIrodoriAudioURL(resp.Body)
	if err != nil {
		return SynthesisOutput{}, err
	}
	audioResp, err := p.downloadAudio(ctx, audioURL)
	if err != nil {
		return SynthesisOutput{}, err
	}
	defer audioResp.Body.Close()
	if audioResp.StatusCode < 200 || audioResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(audioResp.Body, 2048))
		return SynthesisOutput{}, fmt.Errorf("irodori audio bad status=%d body=%s", audioResp.StatusCode, strings.TrimSpace(string(body)))
	}

	audioPath, err := saveEditorWAV(audioResp.Body, in.OutputDir, in.FilePrefix)
	if err != nil {
		return SynthesisOutput{}, err
	}
	return SynthesisOutput{
		Provider:      "irodori",
		VoiceID:       voiceID,
		AudioFilePath: audioPath,
		AudioURL:      audioURL,
	}, nil
}

func (p *IrodoriProvider) usesGradioCallGeneration() bool {
	endpoint := strings.ToLower(strings.TrimSpace(p.cfg.EndpointPath))
	base := strings.ToLower(strings.TrimSpace(p.baseURL))
	return strings.Contains(endpoint, "/gradio_api/call/_run_generation") ||
		strings.Contains(endpoint, "/gradio_api/run/_run_generation") ||
		strings.Contains(base, "/gradio_api/call/_run_generation") ||
		strings.Contains(base, "/gradio_api/run/_run_generation")
}

func (p *IrodoriProvider) synthesizeGradioCall(ctx context.Context, in SynthesisInput, text, voiceID string) (SynthesisOutput, error) {
	uploadedAudio, err := p.referenceAudioFileData(ctx)
	if err != nil {
		return SynthesisOutput{}, err
	}
	payload := struct {
		Data []any `json:"data"`
	}{
		Data: p.runGenerationData(text, uploadedAudio),
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("marshal irodori gradio request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.gradioCallGenerationURL(), bytes.NewReader(reqBody))
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("build irodori gradio request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return SynthesisOutput{}, fmt.Errorf("irodori gradio request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return SynthesisOutput{}, fmt.Errorf("irodori gradio bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var queued struct {
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&queued); err != nil {
		return SynthesisOutput{}, fmt.Errorf("decode irodori gradio event id: %w", err)
	}
	if strings.TrimSpace(queued.EventID) == "" {
		return SynthesisOutput{}, fmt.Errorf("irodori gradio response has no event_id")
	}
	audioURL, err := p.pollGradioCallAudioURL(ctx, queued.EventID)
	if err != nil {
		return SynthesisOutput{}, err
	}
	audioResp, err := p.downloadAudio(ctx, audioURL)
	if err != nil {
		return SynthesisOutput{}, err
	}
	defer audioResp.Body.Close()
	if audioResp.StatusCode < 200 || audioResp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(audioResp.Body, 2048))
		return SynthesisOutput{}, fmt.Errorf("irodori audio bad status=%d body=%s", audioResp.StatusCode, strings.TrimSpace(string(body)))
	}
	audioPath, err := saveEditorWAV(audioResp.Body, in.OutputDir, in.FilePrefix)
	if err != nil {
		return SynthesisOutput{}, err
	}
	return SynthesisOutput{
		Provider:      "irodori",
		VoiceID:       voiceID,
		AudioFilePath: audioPath,
		AudioURL:      audioURL,
	}, nil
}

func (p *IrodoriProvider) gradioCallGenerationURL() string {
	url := strings.TrimRight(strings.TrimSpace(p.synthesisURL()), "/")
	if strings.Contains(strings.ToLower(url), "/gradio_api/run/_run_generation") {
		return strings.Replace(url, "/gradio_api/run/_run_generation", "/gradio_api/call/_run_generation", 1)
	}
	if strings.Contains(strings.ToLower(url), "/gradio_api/call/_run_generation") {
		return url
	}
	return strings.TrimRight(strings.TrimSpace(p.baseURL), "/") + "/gradio_api/call/_run_generation"
}

func (p *IrodoriProvider) pollGradioCallAudioURL(ctx context.Context, eventID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.gradioCallGenerationURL()+"/"+strings.TrimSpace(eventID), nil)
	if err != nil {
		return "", fmt.Errorf("build irodori gradio poll request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("irodori gradio poll failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("irodori gradio poll bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return parseIrodoriGradioSSEAudioURL(resp.Body)
}

func parseIrodoriGradioSSEAudioURL(r io.Reader) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r, 8*1024*1024))
	if err != nil {
		return "", fmt.Errorf("read irodori gradio stream: %w", err)
	}
	event := ""
	for _, rawLine := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if event == "error" {
			return "", fmt.Errorf("irodori gradio generation failed: %s", data)
		}
		if data == "" || data == "null" {
			continue
		}
		audioURL, err := parseIrodoriAudioURL(bytes.NewBufferString(`{"data":` + data + `}`))
		if err == nil && strings.TrimSpace(audioURL) != "" {
			return audioURL, nil
		}
		audioURL, err = parseIrodoriAudioURL(bytes.NewBufferString(data))
		if err == nil && strings.TrimSpace(audioURL) != "" {
			return audioURL, nil
		}
	}
	return "", fmt.Errorf("irodori gradio response has no generated audio candidate")
}
