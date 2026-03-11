package tts

import (
	"net/url"
	"path"
	"strings"
)

// resolveAudioURL builds a browser-fetchable URL from explicit audio_url or audio_path.
func resolveAudioURL(httpBaseURL, audioPath, explicitAudioURL string) string {
	base := strings.TrimSpace(httpBaseURL)
	raw := strings.TrimSpace(explicitAudioURL)
	if raw == "" {
		raw = strings.TrimSpace(audioPath)
	}
	if raw == "" {
		return ""
	}

	// If already absolute URL, use as-is.
	if u, err := url.Parse(raw); err == nil && u.IsAbs() {
		return raw
	}

	if base == "" {
		return raw
	}

	baseURL, err := url.Parse(base)
	if err != nil {
		return raw
	}

	// Best-effort normalize server-returned path (Windows separators, relative path).
	rel := strings.ReplaceAll(raw, "\\", "/")
	rel = strings.TrimPrefix(rel, "./")
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return raw
	}
	baseURL.Path = path.Join(strings.TrimRight(baseURL.Path, "/"), rel)
	baseURL.RawPath = ""
	return baseURL.String()
}
