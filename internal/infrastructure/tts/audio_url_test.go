package tts

import "testing"

func TestResolveAudioURL_ExplicitRelative(t *testing.T) {
	got := resolveAudioURL("http://192.168.1.33:8765", "cache\\x.wav", "/cache/x.wav")
	want := "http://192.168.1.33:8765/cache/x.wav"
	if got != want {
		t.Fatalf("unexpected url: got=%q want=%q", got, want)
	}
}

func TestResolveAudioURL_FromAudioPath(t *testing.T) {
	got := resolveAudioURL("http://192.168.1.33:8765", `cache\oneshot-1_000.wav`, "")
	want := "http://192.168.1.33:8765/cache/oneshot-1_000.wav"
	if got != want {
		t.Fatalf("unexpected url: got=%q want=%q", got, want)
	}
}

func TestResolveAudioURL_AbsolutePreferred(t *testing.T) {
	got := resolveAudioURL("http://192.168.1.33:8765", "cache\\x.wav", "https://cdn.example/audio.wav")
	want := "https://cdn.example/audio.wav"
	if got != want {
		t.Fatalf("unexpected url: got=%q want=%q", got, want)
	}
}
