package tts

import (
	"context"
	"fmt"
	"strings"
)

// AudioSink consumes ordered audio chunks.
type AudioSink interface {
	SubmitChunk(ctx context.Context, sessionID string, ch audioChunk) error
	CompleteSession(ctx context.Context, sessionID string) error
}

// PlaybackAudioSink reuses CommandPlayer to play generated audio paths.
type PlaybackAudioSink struct {
	player        Player
	audioPathRoot string
}

func NewPlaybackAudioSink(player Player, audioPathRoot string) *PlaybackAudioSink {
	return &PlaybackAudioSink{player: player, audioPathRoot: audioPathRoot}
}

func (s *PlaybackAudioSink) SubmitChunk(ctx context.Context, sessionID string, ch audioChunk) error {
	if s == nil || s.player == nil {
		return fmt.Errorf("audio sink is not configured")
	}
	if strings.TrimSpace(ch.AudioPath) == "" {
		return fmt.Errorf("audio_path is empty")
	}
	resolvedPath := resolveAudioPath(ch.AudioPath, s.audioPathRoot)
	r, err := s.player.Play(ctx, resolvedPath)
	if err != nil {
		return err
	}
	if r.ExitCode != 0 {
		return fmt.Errorf("audio playback exit code=%d", r.ExitCode)
	}
	return nil
}

func (s *PlaybackAudioSink) CompleteSession(_ context.Context, sessionID string) error {
	_ = sessionID
	return nil
}
