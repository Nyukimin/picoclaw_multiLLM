package main

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func buildLocalTTSAudioURL(outputDir, audioPath string) string {
	rel, ok := localTTSAudioRelPath(outputDir, audioPath)
	if !ok {
		return ""
	}
	return "/viewer/tts/audio?path=" + url.QueryEscape(rel)
}

func localTTSAudioRelPath(outputDir, audioPath string) (string, bool) {
	baseDir, ok := normalizeLocalTTSAudioBase(outputDir)
	if !ok {
		return "", false
	}
	audioPath = strings.TrimSpace(audioPath)
	if audioPath == "" {
		return "", false
	}

	candidate := audioPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(baseDir, filepath.FromSlash(candidate))
	}
	candidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", false
	}

	rel, err := filepath.Rel(baseDir, candidate)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func handleLocalTTSAudio(outputDir string) http.HandlerFunc {
	baseDir, ok := normalizeLocalTTSAudioBase(outputDir)
	return func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			http.NotFound(w, r)
			return
		}
		target, ok := resolveLocalTTSAudioPath(baseDir, r.URL.Query().Get("path"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, target)
	}
}

func normalizeLocalTTSAudioBase(outputDir string) (string, bool) {
	outputDir = strings.TrimSpace(outputDir)
	if outputDir == "" {
		return "", false
	}
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return "", false
	}
	return filepath.Clean(absDir), true
}

func resolveLocalTTSAudioPath(baseDir, rawRelPath string) (string, bool) {
	rawRelPath = strings.TrimSpace(rawRelPath)
	if rawRelPath == "" {
		return "", false
	}
	rawRelPath = filepath.Clean(filepath.FromSlash(rawRelPath))
	if rawRelPath == "." || rawRelPath == ".." || filepath.IsAbs(rawRelPath) {
		return "", false
	}
	if strings.HasPrefix(rawRelPath, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return filepath.Join(baseDir, rawRelPath), true
}
