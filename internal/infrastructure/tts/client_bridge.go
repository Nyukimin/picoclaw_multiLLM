package tts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	"golang.org/x/net/websocket"
)

type ClientConfig struct {
	HTTPBaseURL     string
	WSURL           string
	VoiceID         string
	SpeechMode      string
	ConnectTimeout  time.Duration
	ReceiveTimeout  time.Duration
	ChunkGapTimeout time.Duration
}

type ttsSession struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	nextSeq int
	buffer  *reorderBuffer
}

// ClientBridge is a best-effort TTS bridge implementation.
type ClientBridge struct {
	cfg    ClientConfig
	sink   AudioSink
	client *http.Client

	mu       sync.RWMutex
	sessions map[string]*ttsSession
}

func NewClientBridge(cfg ClientConfig, sink AudioSink) *ClientBridge {
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 3 * time.Second
	}
	if cfg.ReceiveTimeout <= 0 {
		cfg.ReceiveTimeout = 15 * time.Second
	}
	if cfg.ChunkGapTimeout <= 0 {
		cfg.ChunkGapTimeout = 3 * time.Second
	}
	if strings.TrimSpace(cfg.VoiceID) == "" {
		cfg.VoiceID = "female_01"
	}
	if strings.TrimSpace(cfg.SpeechMode) == "" {
		cfg.SpeechMode = "conversational"
	}
	return &ClientBridge{
		cfg:      cfg,
		sink:     sink,
		client:   &http.Client{Timeout: cfg.ConnectTimeout},
		sessions: make(map[string]*ttsSession),
	}
}

func (b *ClientBridge) StartSession(ctx context.Context, req orchestrator.TTSSessionStart) error {
	if strings.TrimSpace(req.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if err := b.ensureReady(ctx, req.VoiceID); err != nil {
		return err
	}

	conn, err := b.connectWS(ctx)
	if err != nil {
		return err
	}
	session := &ttsSession{
		conn:    conn,
		nextSeq: 1,
		buffer:  newReorderBuffer(b.cfg.ChunkGapTimeout),
	}

	start := map[string]any{
		"type":        "session_start",
		"session_id":  req.SessionID,
		"response_id": req.ResponseID,
		"voice_id":    chooseDefault(req.VoiceID, b.cfg.VoiceID),
		"speech_mode": chooseDefault(req.SpeechMode, b.cfg.SpeechMode),
		"context": map[string]any{
			"event":                   chooseDefault(req.Event, "conversation"),
			"urgency":                 chooseDefault(req.Urgency, "normal"),
			"conversation_mode":       chooseDefault(req.ConversationMode, "chat"),
			"user_attention_required": req.UserAttentionRequired,
		},
	}
	if err := websocket.JSON.Send(conn, start); err != nil {
		_ = conn.Close()
		return fmt.Errorf("send session_start: %w", err)
	}

	b.mu.Lock()
	b.sessions[req.SessionID] = session
	b.mu.Unlock()
	go b.receiveLoop(req.SessionID, session)
	log.Printf("tts_session_start_sent session=%s", req.SessionID)
	return nil
}

func (b *ClientBridge) PushText(ctx context.Context, sessionID string, text string) error {
	session, ok := b.getSession(sessionID)
	if !ok {
		return fmt.Errorf("tts session not found: %s", sessionID)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	session.mu.Lock()
	seq := session.nextSeq
	session.nextSeq++
	payload := map[string]any{
		"type":       "text_delta",
		"session_id": sessionID,
		"seq":        seq,
		"text":       text,
		"emitted_at": time.Now().UTC().Format(time.RFC3339),
	}
	err := websocket.JSON.Send(session.conn, payload)
	session.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send text_delta: %w", err)
	}
	log.Printf("tts_text_delta_sent session=%s seq=%d", sessionID, seq)
	return nil
}

func (b *ClientBridge) EndSession(ctx context.Context, sessionID string) error {
	session, ok := b.getSession(sessionID)
	if !ok {
		return nil
	}
	session.mu.Lock()
	err := websocket.JSON.Send(session.conn, map[string]any{
		"type":       "session_end",
		"session_id": sessionID,
		"is_final":   true,
	})
	session.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send session_end: %w", err)
	}
	_ = ctx
	log.Printf("tts_session_end_sent session=%s", sessionID)
	return nil
}

func (b *ClientBridge) connectWS(ctx context.Context) (*websocket.Conn, error) {
	wsURL := strings.TrimSpace(b.cfg.WSURL)
	if wsURL == "" {
		return nil, fmt.Errorf("tts ws_url is empty")
	}
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ws_url: %w", err)
	}
	origin := "http://localhost/"
	if base := strings.TrimSpace(b.cfg.HTTPBaseURL); base != "" {
		origin = base
	}
	cfg, err := websocket.NewConfig(u.String(), origin)
	if err != nil {
		return nil, fmt.Errorf("build websocket config: %w", err)
	}
	timeout := b.cfg.ConnectTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if d := time.Until(deadline); d > 0 && d < timeout {
			timeout = d
		}
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type dialResult struct {
		conn *websocket.Conn
		err  error
	}
	ch := make(chan dialResult, 1)
	go func() {
		conn, err := websocket.DialConfig(cfg)
		ch <- dialResult{conn: conn, err: err}
	}()
	select {
	case <-dialCtx.Done():
		return nil, fmt.Errorf("connect ws timeout: %w", dialCtx.Err())
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("connect ws: %w", r.err)
		}
		return r.conn, nil
	}
}

func (b *ClientBridge) ensureReady(ctx context.Context, voiceID string) error {
	base := strings.TrimSpace(b.cfg.HTTPBaseURL)
	if base == "" {
		return fmt.Errorf("tts http_base_url is empty")
	}
	u := strings.TrimRight(base, "/") + "/health/ready"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	log.Printf("tts_health_check_start")
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("tts health check failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tts not ready: status=%d", resp.StatusCode)
	}
	var body struct {
		Status string   `json:"status"`
		Voices []string `json:"voices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("decode health ready: %w", err)
	}
	if !strings.EqualFold(body.Status, "ready") {
		return fmt.Errorf("tts status is not ready: %s", body.Status)
	}
	targetVoice := chooseDefault(voiceID, b.cfg.VoiceID)
	if targetVoice != "" && len(body.Voices) > 0 {
		found := false
		for _, v := range body.Voices {
			if strings.EqualFold(v, targetVoice) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("voice not ready: %s", targetVoice)
		}
	}
	log.Printf("tts_health_check_ready")
	return nil
}

func (b *ClientBridge) receiveLoop(sessionID string, s *ttsSession) {
	defer func() {
		_ = s.conn.Close()
		b.mu.Lock()
		delete(b.sessions, sessionID)
		b.mu.Unlock()
	}()

	for {
		_ = s.conn.SetReadDeadline(time.Now().Add(b.cfg.ReceiveTimeout))
		var msg map[string]any
		if err := websocket.JSON.Receive(s.conn, &msg); err != nil {
			log.Printf("tts_session_abort session=%s err=%v", sessionID, err)
			_ = b.sink.CompleteSession(context.Background(), sessionID)
			return
		}

		msgType, _ := msg["type"].(string)
		switch msgType {
		case "audio_chunk_ready":
			ch, ok := parseAudioChunk(msg)
			if !ok {
				continue
			}
			s.buffer.add(ch, time.Now().UTC())
			for _, item := range s.buffer.drain(time.Now().UTC(), false) {
				log.Printf("tts_audio_chunk_received session=%s chunk=%d", sessionID, item.ChunkIndex)
				if err := b.sink.SubmitChunk(context.Background(), sessionID, item); err != nil {
					log.Printf("tts_audio_chunk_play_error session=%s chunk=%d err=%v", sessionID, item.ChunkIndex, err)
				}
			}
		case "session_completed":
			for _, item := range s.buffer.drain(time.Now().UTC(), true) {
				if err := b.sink.SubmitChunk(context.Background(), sessionID, item); err != nil {
					log.Printf("tts_audio_chunk_play_error session=%s chunk=%d err=%v", sessionID, item.ChunkIndex, err)
				}
			}
			_ = b.sink.CompleteSession(context.Background(), sessionID)
			log.Printf("tts_session_completed_received session=%s", sessionID)
			return
		case "error":
			code, _ := msg["code"].(string)
			message, _ := msg["message"].(string)
			log.Printf("tts_error_received session=%s code=%s message=%s", sessionID, code, message)
			_ = b.sink.CompleteSession(context.Background(), sessionID)
			return
		}
	}
}

func parseAudioChunk(msg map[string]any) (audioChunk, bool) {
	getInt := func(k string) int {
		v, ok := msg[k]
		if !ok {
			return 0
		}
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		default:
			return 0
		}
	}
	chunk := audioChunk{
		ChunkIndex: getInt("chunk_index"),
	}
	text, _ := msg["text"].(string)
	path, _ := msg["audio_path"].(string)
	pause, _ := msg["pause_after"].(string)
	chunk.Text = text
	chunk.AudioPath = path
	chunk.PauseAfter = pause
	return chunk, strings.TrimSpace(path) != ""
}

func chooseDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func (b *ClientBridge) getSession(sessionID string) (*ttsSession, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s, ok := b.sessions[sessionID]
	return s, ok
}
