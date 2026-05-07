package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type IrodoriConfig struct {
	BaseURL               string
	EndpointPath          string
	VoiceID               string
	VoiceName             string
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

func (p *IrodoriProvider) Synthesize(ctx context.Context, in SynthesisInput) (SynthesisOutput, error) {
	if strings.TrimSpace(p.baseURL) == "" {
		return SynthesisOutput{}, fmt.Errorf("%w: irodori base_url is empty", ErrProviderUnavailable)
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return SynthesisOutput{}, fmt.Errorf("text is required")
	}
	voiceID := resolveIrodoriVoiceID(chooseNonEmpty(in.VoiceProfile.VoiceID, p.voiceID))
	voice := resolveIrodoriVoiceName(chooseNonEmpty(in.VoiceProfile.VoiceID, p.cfg.VoiceName, p.voiceID))
	style := resolveIrodoriStyle(in.Emotion)
	payload := map[string]any{
		"voice": voice,
		"style": style,
		"text":  ensureTTSPunctuation(text),
	}
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
	}, nil
}

func (p *IrodoriProvider) synthesisURL() string {
	base := strings.TrimRight(p.baseURL, "/")
	if base == "" {
		return ""
	}
	if u, err := url.Parse(base); err == nil && strings.Trim(strings.TrimSpace(u.Path), "/") != "" {
		return base
	}
	endpointPath := strings.TrimSpace(p.cfg.EndpointPath)
	if endpointPath == "" {
		endpointPath = "/api/tts"
	}
	return base + "/" + strings.TrimLeft(endpointPath, "/")
}

func (p *IrodoriProvider) runGenerationURL() string {
	base := strings.TrimRight(p.baseURL, "/")
	if strings.HasSuffix(strings.ToLower(base), "/gradio_api/run/_run_generation") {
		return base
	}
	return base + "/gradio_api/run/_run_generation"
}

func (p *IrodoriProvider) runGenerationData(text string, uploadedAudio any) []any {
	cfg := p.cfg
	return []any{
		cfg.Checkpoint,
		cfg.ModelDevice,
		cfg.ModelPrecision,
		cfg.CodecDevice,
		cfg.CodecPrecision,
		cfg.EnableWatermark,
		text,
		uploadedAudio,
		cfg.NumSteps,
		cfg.NumCandidates,
		cfg.SeedRaw,
		cfg.CFGGuidanceMode,
		cfg.CFGScaleText,
		cfg.CFGScaleSpeaker,
		cfg.CFGScaleRaw,
		cfg.CFGMinT,
		cfg.CFGMaxT,
		cfg.ContextKVCache,
		cfg.TruncationFactorRaw,
		cfg.RescaleKRaw,
		cfg.RescaleSigmaRaw,
		cfg.SpeakerKVScaleRaw,
		cfg.SpeakerKVMinTRaw,
		cfg.SpeakerKVMaxLayersRaw,
	}
}

func (p *IrodoriProvider) referenceAudioFileData(ctx context.Context) (any, error) {
	if strings.TrimSpace(p.cfg.ReferenceAudio) == "" && strings.TrimSpace(p.cfg.ReferenceAudioURL) == "" {
		return nil, nil
	}
	uploadedPath, err := p.uploadReferenceAudio(ctx)
	if err == nil && strings.TrimSpace(uploadedPath) != "" {
		return irodoriUploadedAudio(uploadedPath), nil
	}
	if strings.TrimSpace(p.cfg.ReferenceAudioURL) != "" {
		return nil, err
	}
	return irodoriUploadedAudio(p.cfg.ReferenceAudio), nil
}

func (p *IrodoriProvider) uploadReferenceAudio(ctx context.Context) (string, error) {
	p.refMu.Lock()
	defer p.refMu.Unlock()
	if strings.TrimSpace(p.refPath) != "" {
		return p.refPath, nil
	}
	var (
		r        io.ReadCloser
		fileName string
		err      error
	)
	if rawURL := strings.TrimSpace(p.cfg.ReferenceAudioURL); rawURL != "" {
		r, fileName, err = p.openReferenceAudioURL(ctx, rawURL)
	} else {
		r, fileName, err = openReferenceAudioFile(p.cfg.ReferenceAudio)
	}
	if err != nil {
		return "", err
	}
	defer r.Close()
	uploadedPath, err := p.uploadFile(ctx, r, fileName)
	if err != nil {
		return "", err
	}
	p.refPath = uploadedPath
	return uploadedPath, nil
}

func (p *IrodoriProvider) openReferenceAudioURL(ctx context.Context, rawURL string) (io.ReadCloser, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build irodori reference audio request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download irodori reference audio failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, "", fmt.Errorf("irodori reference audio bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	name := "reference.wav"
	if u, err := url.Parse(rawURL); err == nil {
		if base := filepath.Base(u.Path); base != "." && base != "/" && strings.TrimSpace(base) != "" {
			name = base
		}
	}
	return resp.Body, name, nil
}

func openReferenceAudioFile(referenceAudio string) (io.ReadCloser, string, error) {
	path := strings.TrimSpace(referenceAudio)
	if path == "" {
		return nil, "", fmt.Errorf("irodori reference_audio is empty")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open irodori reference audio: %w", err)
	}
	name := filepath.Base(path)
	if name == "." || strings.TrimSpace(name) == "" {
		name = "reference.wav"
	}
	return f, name, nil
}

func (p *IrodoriProvider) uploadFile(ctx context.Context, r io.Reader, fileName string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		return "", fmt.Errorf("create irodori upload form: %w", err)
	}
	if _, err := io.Copy(part, r); err != nil {
		return "", fmt.Errorf("write irodori upload form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close irodori upload form: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.baseURL, "/")+"/gradio_api/upload", &body)
	if err != nil {
		return "", fmt.Errorf("build irodori upload request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("irodori upload failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("irodori upload bad status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var paths []string
	if err := json.NewDecoder(resp.Body).Decode(&paths); err != nil {
		return "", fmt.Errorf("decode irodori upload response: %w", err)
	}
	if len(paths) == 0 || strings.TrimSpace(paths[0]) == "" {
		return "", fmt.Errorf("irodori upload response has no file path")
	}
	return paths[0], nil
}

func irodoriUploadedAudio(referenceAudio string) any {
	referenceAudio = strings.TrimSpace(referenceAudio)
	if referenceAudio == "" {
		return nil
	}
	return map[string]any{
		"path": referenceAudio,
		"meta": map[string]any{
			"_type": "gradio.FileData",
		},
	}
}

func (p *IrodoriProvider) downloadAudio(ctx context.Context, rawURL string) (*http.Response, error) {
	audioURL := strings.TrimSpace(rawURL)
	if audioURL == "" {
		return nil, fmt.Errorf("irodori response did not include an audio url")
	}
	if strings.HasPrefix(audioURL, "/") {
		audioURL = strings.TrimRight(p.baseURL, "/") + audioURL
	} else if !strings.Contains(audioURL, "://") {
		audioURL = strings.TrimRight(p.baseURL, "/") + "/" + strings.TrimLeft(audioURL, "/")
	} else {
		audioURL = rewriteLoopbackIrodoriFileURL(p.baseURL, audioURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build irodori audio request: %w", err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download irodori audio failed: %w", err)
	}
	return resp, nil
}

func parseIrodoriAudioURL(r io.Reader) (string, error) {
	var raw json.RawMessage
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return "", fmt.Errorf("decode irodori response: %w", err)
	}
	if url := parseIrodoriSimpleAudioURL(raw); strings.TrimSpace(url) != "" {
		return url, nil
	}
	var body struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return "", fmt.Errorf("decode irodori response: %w", err)
	}
	if len(body.Data) == 0 || string(body.Data[0]) == "null" {
		return "", fmt.Errorf("irodori response has no generated audio candidate")
	}
	var candidate struct {
		URL      string `json:"url"`
		Path     string `json:"path"`
		OrigName string `json:"orig_name"`
		MIMEType string `json:"mime_type"`
		Value    *struct {
			URL      string `json:"url"`
			Path     string `json:"path"`
			OrigName string `json:"orig_name"`
			MIMEType string `json:"mime_type"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body.Data[0], &candidate); err != nil {
		return "", fmt.Errorf("decode irodori audio candidate: %w", err)
	}
	if strings.TrimSpace(candidate.URL) == "" && candidate.Value != nil {
		candidate.URL = candidate.Value.URL
	}
	if strings.TrimSpace(candidate.URL) == "" {
		return "", fmt.Errorf("irodori audio candidate has no url")
	}
	return candidate.URL, nil
}

func withIrodoriDefaults(cfg IrodoriConfig) IrodoriConfig {
	if cfg.EndpointPath == "" {
		cfg.EndpointPath = "/api/tts"
	}
	if cfg.Checkpoint == "" {
		cfg.Checkpoint = "Aratako/Irodori-TTS-500M-v2"
	}
	if cfg.ModelDevice == "" {
		cfg.ModelDevice = "mps"
	}
	if cfg.ModelPrecision == "" {
		cfg.ModelPrecision = "fp32"
	}
	if cfg.CodecDevice == "" {
		cfg.CodecDevice = "mps"
	}
	if cfg.CodecPrecision == "" {
		cfg.CodecPrecision = "fp32"
	}
	if cfg.NumSteps <= 0 {
		cfg.NumSteps = 16
	}
	if cfg.NumCandidates <= 0 {
		cfg.NumCandidates = 1
	}
	if cfg.CFGGuidanceMode == "" {
		cfg.CFGGuidanceMode = "independent"
	}
	if cfg.CFGScaleText == 0 {
		cfg.CFGScaleText = 3.0
	}
	if cfg.CFGScaleSpeaker == 0 {
		cfg.CFGScaleSpeaker = 5.0
	}
	if cfg.CFGMinT == 0 {
		cfg.CFGMinT = 0.5
	}
	if cfg.CFGMaxT == 0 {
		cfg.CFGMaxT = 1.0
	}
	if !cfg.ContextKVCache {
		cfg.ContextKVCache = true
	}
	return cfg
}

func rewriteLoopbackIrodoriFileURL(baseURL, rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil {
		return rawURL
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return rawURL
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base == nil || strings.TrimSpace(base.Host) == "" {
		return rawURL
	}
	parsed.Scheme = base.Scheme
	parsed.Host = base.Host
	return parsed.String()
}

func resolveIrodoriVoiceID(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shiro", "male", "male_01", "shi-gozaki", "shigozaki":
		return "shiro"
	default:
		return "mio"
	}
}

func resolveIrodoriVoiceName(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "shiro", "male", "male_01", "shi-gozaki", "shigozaki":
		return "Shiro"
	case "mio", "female", "female_01", "female_01_mio":
		return "Mio"
	default:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return "Mio"
		}
		return trimmed
	}
}

func resolveIrodoriStyle(emotion EmotionState) string {
	switch strings.ToLower(strings.TrimSpace(emotion.Emotion)) {
	case "alert", "warning", "urgent":
		return "urgent"
	case "serious", "report":
		return "firm"
	case "cheerful", "happy":
		return "bright"
	case "warm", "soft":
		return "soft"
	case "flat":
		return "flat"
	case "calm":
		return "calm"
	}
	if emotion.Intensity == 0 && emotion.Expressiveness == 0 && emotion.Pitch == 0 && emotion.Speed == 0 && strings.TrimSpace(emotion.Pause) == "" {
		return "neutral"
	}
	if emotion.Intensity >= 0.75 {
		return "urgent"
	}
	if emotion.Expressiveness >= 0.65 || emotion.Pitch >= 0.58 {
		return "bright"
	}
	if (emotion.Speed > 0 && emotion.Speed <= 0.42) || emotion.Pause == "long" {
		return "calm"
	}
	return "neutral"
}

func parseIrodoriSimpleAudioURL(raw json.RawMessage) string {
	var body struct {
		URL      string `json:"url"`
		AudioURL string `json:"audio_url"`
		WAVURL   string `json:"wav_url"`
		Path     string `json:"path"`
		Audio    *struct {
			URL      string `json:"url"`
			AudioURL string `json:"audio_url"`
			Path     string `json:"path"`
		} `json:"audio"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return ""
	}
	for _, candidate := range []string{body.AudioURL, body.WAVURL, body.URL} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	if body.Audio != nil {
		for _, candidate := range []string{body.Audio.AudioURL, body.Audio.URL, body.Audio.Path} {
			if strings.TrimSpace(candidate) != "" {
				return candidate
			}
		}
	}
	return strings.TrimSpace(body.Path)
}
