package tts

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AudioSink consumes ordered audio chunks.
type AudioSink interface {
	SubmitChunk(ctx context.Context, sessionID string, ch audioChunk) error
	CompleteSession(ctx context.Context, sessionID string) error
}

const defaultChunkPause = 200 * time.Millisecond // 同一話者内の句間ブレイク

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
	pause := parsePauseAfter(ch.PauseAfter)
	if pause <= 0 {
		pause = defaultChunkPause
	}
	select {
	case <-ctx.Done():
	case <-time.After(pause):
	}
	return nil
}

func (s *PlaybackAudioSink) CompleteSession(_ context.Context, sessionID string) error {
	_ = sessionID
	return nil
}

// parsePauseAfter はTTS Serverから返される pause_after 値をパースする。
// "0.5s", "500ms", "1.2" (秒として解釈) などを受け付ける。
func parsePauseAfter(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		return time.Duration(f * float64(time.Second))
	}
	return 0
}
