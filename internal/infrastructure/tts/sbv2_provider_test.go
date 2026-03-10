package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

func TestSBV2Provider_SynthesizeFromAudioPath(t *testing.T) {
	p := NewSBV2Provider(SBV2Config{BaseURL: "http://sbv2.local/synthesis", VoiceID: "mio"})
	p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var in map[string]any
		if err := json.Unmarshal(body, &in); err != nil {
			t.Fatalf("invalid request json: %v", err)
		}
		if in["text"] != "hello" {
			t.Fatalf("unexpected request payload: %+v", in)
		}
		out, _ := json.Marshal(map[string]any{
			"audio_path":  "/tmp/sbv2.wav",
			"duration_ms": 1234,
			"voice_id":    "mio",
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(out)),
			Header:     make(http.Header),
		}, nil
	})}

	out, err := p.Synthesize(context.Background(), SynthesisInput{Text: "hello"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Provider != "sbv2" || out.AudioFilePath != "/tmp/sbv2.wav" || out.DurationMS != 1234 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
