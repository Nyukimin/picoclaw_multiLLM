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
	tmpDir := t.TempDir()
	p := NewSBV2Provider(SBV2Config{BaseURL: "http://sbv2.local", VoiceID: "amitaro"})
	p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if got := r.URL.Path; got != "/voice" {
			t.Fatalf("unexpected path: %s", got)
		}
		q := r.URL.Query()
		if q.Get("text") != "hello。" || q.Get("model_id") != "0" || q.Get("speaker_id") != "0" || q.Get("style") != "Neutral" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("RIFFtest")),
			Header:     make(http.Header),
		}, nil
	})}

	out, err := p.Synthesize(context.Background(), SynthesisInput{Text: "hello", OutputDir: tmpDir})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.Provider != "sbv2" || out.VoiceID != "amitaro" {
		t.Fatalf("unexpected output: %+v", out)
	}
	got, err := os.ReadFile(out.AudioFilePath)
	if err != nil || string(got) != "RIFFtest" {
		t.Fatalf("unexpected wav output: err=%v got=%q", err, string(got))
	}
}

func TestSBV2Provider_SynthesizeFromAudioPath_WithRootMapping(t *testing.T) {
	p := NewSBV2Provider(SBV2Config{
		BaseURL:       "http://sbv2.local/voice",
		VoiceID:       "shi-gozaki",
		AudioPathRoot: "/unused",
	})
	p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("model_id") != "6" || q.Get("speaker_id") != "0" || q.Get("style") != "Neutral" {
			t.Fatalf("unexpected query for shi-gozaki: %s", r.URL.RawQuery)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString("RIFFshi")),
			Header:     make(http.Header),
		}, nil
	})}

	out, err := p.Synthesize(context.Background(), SynthesisInput{Text: "hello", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out.VoiceID != "shi-gozaki" {
		t.Fatalf("unexpected voice id: %s", out.VoiceID)
	}
}

func TestSBV2Provider_SynthesizeVoiceAliasesForViewerCharacters(t *testing.T) {
	tests := []struct {
		name        string
		voiceID     string
		wantVoiceID string
		wantModelID string
	}{
		{name: "mio alias", voiceID: "mio", wantVoiceID: "amitaro", wantModelID: "0"},
		{name: "female legacy alias", voiceID: "female_01", wantVoiceID: "amitaro", wantModelID: "0"},
		{name: "shiro alias", voiceID: "shiro", wantVoiceID: "shi-gozaki", wantModelID: "6"},
		{name: "male legacy alias", voiceID: "male_01", wantVoiceID: "shi-gozaki", wantModelID: "6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewSBV2Provider(SBV2Config{BaseURL: "http://sbv2.local", VoiceID: "amitaro"})
			p.client = &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				q := r.URL.Query()
				if q.Get("model_id") != tt.wantModelID || q.Get("speaker_id") != "0" || q.Get("style") != "Neutral" {
					t.Fatalf("unexpected query for %s: %s", tt.voiceID, r.URL.RawQuery)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString("RIFFalias")),
					Header:     make(http.Header),
				}, nil
			})}

			out, err := p.Synthesize(context.Background(), SynthesisInput{
				Text:      "hello",
				OutputDir: t.TempDir(),
				VoiceProfile: VoiceProfile{
					VoiceID: tt.voiceID,
				},
			})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if out.VoiceID != tt.wantVoiceID {
				t.Fatalf("unexpected resolved voice id: got %q want %q", out.VoiceID, tt.wantVoiceID)
			}
		})
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
