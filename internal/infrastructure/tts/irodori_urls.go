package tts

import (
	"encoding/json"
	"net/url"
	"strings"
)

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
