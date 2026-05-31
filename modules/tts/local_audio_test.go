package tts

import (
	"path/filepath"
	"testing"
)

func TestBuildLocalAudioURL(t *testing.T) {
	outputDir := t.TempDir()
	audioPath := filepath.Join(outputDir, "sample.wav")
	got := BuildLocalAudioURL(outputDir, audioPath)
	want := "/viewer/tts/audio?path=sample.wav"
	if got != want {
		t.Fatalf("BuildLocalAudioURL() = %q, want %q", got, want)
	}
}

func TestBuildLocalAudioURLRejectsOutsideOutputDir(t *testing.T) {
	outputDir := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "sample.wav")
	if got := BuildLocalAudioURL(outputDir, outsidePath); got != "" {
		t.Fatalf("BuildLocalAudioURL() = %q, want empty", got)
	}
}

func TestLocalAudioRelPathHandlesRelativeInput(t *testing.T) {
	outputDir := t.TempDir()
	got, ok := LocalAudioRelPath(outputDir, "nested/chunk.wav")
	if !ok || got != "nested/chunk.wav" {
		t.Fatalf("LocalAudioRelPath() = %q,%t", got, ok)
	}
}

func TestResolveLocalAudioPathRejectsTraversal(t *testing.T) {
	base, ok := NormalizeLocalAudioBase(t.TempDir())
	if !ok {
		t.Fatal("NormalizeLocalAudioBase() failed")
	}
	if got, ok := ResolveLocalAudioPath(base, "../secret.wav"); ok || got != "" {
		t.Fatalf("ResolveLocalAudioPath() = %q,%t, want reject", got, ok)
	}
}
