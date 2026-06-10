package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/task"
)

const voiceDirectReason = "voice_direct"

// ProcessVoiceDirectRequest は VDS 確定後の orchestrator 連携入力。
// Phase 1 では RenCrow_LLM WS が推論し、picoclaw は FinalText を受け取って SSE を出す。
type ProcessVoiceDirectRequest struct {
	UtteranceID   string
	SessionID     string
	Channel       string
	ChatID        string
	ViewerSession string
	Prompt        string
	SampleRate    int
	Channels      int
	AudioWAVPath  string
	FinalText     string
	StartedAt     time.Time
	FirstTokenAt  time.Time
}

func (req ProcessVoiceDirectRequest) normalizedChannel() string {
	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		return "viewer"
	}
	return channel
}

func (req ProcessVoiceDirectRequest) normalizedSessionID() string {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID != "" {
		return sessionID
	}
	if viewerSession := strings.TrimSpace(req.ViewerSession); viewerSession != "" {
		return viewerSession
	}
	return "viewer"
}

func (req ProcessVoiceDirectRequest) normalizedChatID() string {
	chatID := strings.TrimSpace(req.ChatID)
	if chatID != "" {
		return chatID
	}
	return "viewer-user"
}

func (req ProcessVoiceDirectRequest) userMessageLabel() string {
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		return fmt.Sprintf("[voice_direct] %s", prompt)
	}
	return "[voice_direct]"
}

func validateProcessVoiceDirectRequest(req ProcessVoiceDirectRequest) error {
	if strings.TrimSpace(req.FinalText) == "" {
		return errors.New("voice direct final text is required")
	}
	if strings.TrimSpace(req.UtteranceID) == "" {
		return errors.New("voice direct utterance_id is required")
	}
	channel := req.normalizedChannel()
	if channel != "viewer" {
		return fmt.Errorf("voice direct is only allowed on viewer channel, got %q", channel)
	}
	return nil
}

// ProcessVoiceDirect は LLM WS 推論完了後に Chat SSE イベントを発行する。
// STT / Mio.Chat / IdleChat には触れない。
func (o *MessageOrchestrator) ProcessVoiceDirect(ctx context.Context, req ProcessVoiceDirectRequest) (ProcessMessageResponse, error) {
	if o == nil {
		return ProcessMessageResponse{}, errors.New("message orchestrator is nil")
	}
	if err := validateProcessVoiceDirectRequest(req); err != nil {
		return ProcessMessageResponse{}, err
	}
	startedAt := req.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	ctx = contextWithLatencyTrace(ctx, startedAt)

	sessionID := req.normalizedSessionID()
	channel := req.normalizedChannel()
	chatID := req.normalizedChatID()
	jobID := task.NewJobID()
	decision := routing.NewDecision(routing.RouteCHAT, 1.0, voiceDirectReason)

	userMessage := req.userMessageLabel()
	o.events.EmitMessageReceived(ProcessMessageRequest{
		SessionID:   sessionID,
		Channel:     channel,
		ChatID:      chatID,
		UserMessage: userMessage,
	})
	emitLatencyMetric(o.events.Emit, "network", "server_received", startedAt, "", "", sessionID, channel, chatID, req.UtteranceID)

	o.events.Emit(
		"routing.decision",
		"mio",
		"",
		fmt.Sprintf("confidence 100%% evidence=voice_direct:matched:CHAT utterance_id=%s", req.UtteranceID),
		string(routing.RouteCHAT),
		jobID.String(),
		sessionID,
		channel,
		chatID,
	)
	emitLatencyMetric(o.events.Emit, "llm", "route_decision", startedAt, string(routing.RouteCHAT), jobID.String(), sessionID, channel, chatID, voiceDirectReason)
	emitLatencyMetric(o.events.Emit, "llm", "dispatch_start", startedAt, string(routing.RouteCHAT), jobID.String(), sessionID, channel, chatID, voiceDirectReason)

	if !req.FirstTokenAt.IsZero() {
		emitVoiceDirectPointLatency(
			o.events.Emit,
			"llm",
			"first_token",
			startedAt,
			req.FirstTokenAt,
			string(routing.RouteCHAT),
			jobID.String(),
			sessionID,
			channel,
			chatID,
			req.UtteranceID,
		)
	}

	finalText := strings.TrimSpace(req.FinalText)
	o.events.Emit("agent.response", "mio", "user", finalText, string(routing.RouteCHAT), jobID.String(), sessionID, channel, chatID)
	emitLatencyMetric(
		o.events.Emit,
		"llm",
		"response_complete",
		startedAt,
		string(routing.RouteCHAT),
		jobID.String(),
		sessionID,
		channel,
		chatID,
		fmt.Sprintf("utterance_id=%s response_len=%d", req.UtteranceID, len(finalText)),
	)

	if o.sessionTurnLogger != nil {
		o.sessionTurnLogger.WriteUser(sessionID, channel, userMessage)
		o.sessionTurnLogger.WriteAssistant(sessionID, channel, string(routing.RouteCHAT), jobID.String(), finalText)
	}

	_ = ctx
	return o.responses.Build(finalText, decision, jobID), nil
}

// NotifyVoiceDirectFirstToken は bridge が初回 llm.delta を転送したタイミングで呼ぶ。
func (o *MessageOrchestrator) NotifyVoiceDirectFirstToken(ctx context.Context, req ProcessVoiceDirectRequest, jobID task.JobID, firstTokenAt time.Time) {
	if o == nil || firstTokenAt.IsZero() {
		return
	}
	startedAt := req.StartedAt
	if startedAt.IsZero() {
		startedAt = firstTokenAt
	}
	sessionID := req.normalizedSessionID()
	channel := req.normalizedChannel()
	chatID := req.normalizedChatID()
	if jobID.IsZero() {
		jobID = task.NewJobID()
	}
	emitVoiceDirectPointLatency(
		o.events.Emit,
		"llm",
		"first_token",
		startedAt,
		firstTokenAt,
		string(routing.RouteCHAT),
		jobID.String(),
		sessionID,
		channel,
		chatID,
		req.UtteranceID,
	)
	_ = ctx
}

func emitVoiceDirectPointLatency(
	emit messageEventEmitter,
	kind, point string,
	startedAt, at time.Time,
	route, jobID, sessionID, channel, chatID, detail string,
) {
	if emit == nil || startedAt.IsZero() || at.IsZero() {
		return
	}
	payload := latencyMetricPayload{
		Kind:      kind,
		Point:     point,
		ElapsedMS: float64(at.Sub(startedAt).Microseconds()) / 1000.0,
		SinceMS:   float64(at.Sub(startedAt).Microseconds()) / 1000.0,
		AtUnixMS:  at.UnixMilli(),
		Detail:    detail,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		content = []byte(fmt.Sprintf(`{"kind":%q,"point":%q,"at_unix_ms":%d}`, kind, point, at.UnixMilli()))
	}
	emit("metrics.latency", "metrics", "viewer", string(content), route, jobID, sessionID, channel, chatID)
}
