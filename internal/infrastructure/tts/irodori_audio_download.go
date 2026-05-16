package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

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
