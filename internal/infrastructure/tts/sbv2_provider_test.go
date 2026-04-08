package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestSBV2Provider_SynthesizeFromAudioPath_WithRootMapping(t *testing.T) {
	p := NewSBV2Provider(SBV2Config{
		BaseURL:       "http://sbv2.local/synthesis",
		VoiceID:       "mio",
		AudioPathRoot: "/mnt/e/GenerativeAI/Style-Bert-VITS2",
	})
	p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		out, _ := json.Marshal(map[string]any{
			"audio_path":  `cache\\oneshot-abc_000.wav`,
			"duration_ms": 100,
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
	want := "/mnt/e/GenerativeAI/Style-Bert-VITS2/cache/oneshot-abc_000.wav"
	if out.AudioFilePath != want {
		t.Fatalf("unexpected mapped audio path: got=%q want=%q", out.AudioFilePath, want)
	}
}

func TestSBV2Provider_SynthesizeEditorAPI_WritesWAV(t *testing.T) {
	tmpDir := t.TempDir()
	p := NewSBV2Provider(SBV2Config{
		BaseURL: "http://sbv2.local/api/synthesis",
		VoiceID: "jvnv-F1-jp",
	})
	p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/models_info":
			body := `[{"name":"jvnv-F1-jp","files":["model_assets\\jvnv-F1-jp\\voice.safetensors"],"speakers":["jvnv-F1-jp"]}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/g2p":
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatalf("decode g2p body failed: %v", err)
			}
			if in["text"] != "こんにちは。" {
				t.Fatalf("expected punctuated g2p text, got %+v", in)
			}
			body := `[{"mora":"コ","tone":0}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/api/synthesis":
			var in map[string]any
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				t.Fatalf("decode synthesis body failed: %v", err)
			}
			if in["speaker"] != "jvnv-F1-jp" {
				t.Fatalf("unexpected speaker: %+v", in)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString("RIFFtest")),
				Header:     http.Header{"Content-Type": []string{"audio/wav"}},
			}, nil
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})}

	out, err := p.Synthesize(context.Background(), SynthesisInput{
		Text:       "こんにちは",
		OutputDir:  tmpDir,
		FilePrefix: "sess1",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Provider != "sbv2" || out.VoiceID != "jvnv-F1-jp" {
		t.Fatalf("unexpected output metadata: %+v", out)
	}
	if filepath.Dir(out.AudioFilePath) != tmpDir {
		t.Fatalf("expected wav in tmp dir, got %q", out.AudioFilePath)
	}
	got, err := os.ReadFile(out.AudioFilePath)
	if err != nil {
		t.Fatalf("read wav failed: %v", err)
	}
	if string(got) != "RIFFtest" {
		t.Fatalf("unexpected wav contents: %q", string(got))
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
