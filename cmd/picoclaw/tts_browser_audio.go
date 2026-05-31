package main

import (
	"net/http"

	moduletts "github.com/Nyukimin/picoclaw_multiLLM/modules/tts"
)

func buildLocalTTSAudioURL(outputDir, audioPath string) string {
	return moduletts.BuildLocalAudioURL(outputDir, audioPath)
}

func localTTSAudioRelPath(outputDir, audioPath string) (string, bool) {
	return moduletts.LocalAudioRelPath(outputDir, audioPath)
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
	return moduletts.NormalizeLocalAudioBase(outputDir)
}

func resolveLocalTTSAudioPath(baseDir, rawRelPath string) (string, bool) {
	return moduletts.ResolveLocalAudioPath(baseDir, rawRelPath)
}
