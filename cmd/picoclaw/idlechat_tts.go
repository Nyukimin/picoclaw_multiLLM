package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

const (
	idleChatRoute            = "IDLECHAT"
	idleChatDefaultVoiceID   = "female_01"
	idleChatDefaultVoiceProf = "lumina_female"
	idleChatMaleVoiceID      = "male_01"
	idleChatMaleVoiceProf    = "lumina_male"
)

var idleChatTopicPrefixRe = regexp.MustCompile(`^今日のお題（[^）]+）:\s*`)

type idleChatTTSItem struct {
	bridge orchestrator.TTSBridge
	ev     idlechat.TimelineEvent
}

var (
	idleChatTTSOnce      sync.Once
	idleChatTTSQueue     chan idleChatTTSItem
	idleChatTTSPendingMu sync.Mutex
	idleChatTTSPending   = map[string]chan struct{}{}
	// Topic announcement must finish before the first agent line for the same idle session.
	idleChatTopicGate  = map[string]chan struct{}{}
	idleChatTopicByTTS = map[string]string{}
)

func emitIdleChatTTS(ctx context.Context, bridge orchestrator.TTSBridge, ev idlechat.TimelineEvent) (<-chan struct{}, bool) {
	if bridge == nil || strings.TrimSpace(ev.Content) == "" || ev.Type != "idlechat.message" {
		return nil, false
	}

	filtered := ttsapp.FilterSpeakableText("agent.response", idleChatRoute, formatIdleChatTTSText(ev))
	if filtered == "" {
		return nil, false
	}

	voiceID, voiceProfile := idleChatVoiceForSpeaker(ev.From)
	emotion := ttsapp.PlanEmotion(ttsapp.EmotionInput{
		Event: "conversation",
		Text:  filtered,
		Context: ttsapp.EmotionContext{
			ConversationMode: "chat",
			TimeOfDay:        idleChatTimeOfDay(),
			Urgency:          "normal",
		},
		VoiceProfile: voiceProfile,
	})

	sessionID := fmt.Sprintf("%s-tts-%d", strings.TrimSpace(ev.SessionID), time.Now().UnixNano())
	waitCh := registerIdleChatTTSPending(sessionID)
	if isIdleChatTopicAnnouncement(ev) {
		registerIdleChatTopicGate(ev.SessionID, sessionID)
	}
	if err := bridge.StartSession(ctx, orchestrator.TTSSessionStart{
		SessionID:        sessionID,
		VoiceID:          voiceID,
		SpeechMode:       "conversational",
		Event:            "conversation",
		ConversationMode: "chat",
		Context: ttsapp.EmotionContext{
			ConversationMode: "chat",
			TimeOfDay:        idleChatTimeOfDay(),
			Urgency:          "normal",
		},
		VoiceProfile: voiceProfile,
	}); err != nil {
		clearIdleChatTTSPending(sessionID)
		log.Printf("[IdleChat] TTS start failed: %v", err)
		return nil, false
	}
	if err := bridge.PushText(ctx, sessionID, filtered, &emotion); err != nil {
		log.Printf("[IdleChat] TTS push failed: %v", err)
	}
	if err := bridge.EndSession(ctx, sessionID); err != nil {
		clearIdleChatTTSPending(sessionID)
		log.Printf("[IdleChat] TTS end failed: %v", err)
		return nil, false
	}
	return waitCh, true
}

func formatIdleChatTTSText(ev idlechat.TimelineEvent) string {
	content := strings.TrimSpace(ev.Content)
	if strings.EqualFold(ev.From, "user") && strings.EqualFold(ev.To, "mio") && idleChatTopicPrefixRe.MatchString(content) {
		topic := strings.TrimSpace(idleChatTopicPrefixRe.ReplaceAllString(content, ""))
		if topic == "" {
			return "きょうのおだいです！"
		}
		return "きょうのおだいです。" + ensureIdleChatSentencePause(topic) + "です！"
	}
	return ensureIdleChatSentencePause(content)
}

func ensureIdleChatSentencePause(content string) string {
	if content == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(content, "。"),
		strings.HasSuffix(content, "！"),
		strings.HasSuffix(content, "？"),
		strings.HasSuffix(content, "."),
		strings.HasSuffix(content, "!"),
		strings.HasSuffix(content, "?"):
		return content
	default:
		return content + "。"
	}
}

func emitIdleChatTTSAsync(bridge orchestrator.TTSBridge, ev idlechat.TimelineEvent) {
	if bridge == nil {
		return
	}
	if !isIdleChatTopicAnnouncement(ev) {
		go func() {
			// Non-topic lines wait until the topic TTS closes its gate for this idle session.
			waitIdleChatTopicGate(ev.SessionID)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			_, _ = emitIdleChatTTS(ctx, bridge, ev)
		}()
		return
	}
	idleChatTTSOnce.Do(func() {
		idleChatTTSQueue = make(chan idleChatTTSItem, 128)
		go func() {
			for item := range idleChatTTSQueue {
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				waitCh, ok := emitIdleChatTTS(ctx, item.bridge, item.ev)
				if ok && waitCh != nil {
					select {
					case <-waitCh:
					case <-ctx.Done():
						clearIdleChatTTSPendingByChan(waitCh)
					}
				}
				cancel()
			}
		}()
	})
	select {
	case idleChatTTSQueue <- idleChatTTSItem{bridge: bridge, ev: ev}:
	default:
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, _ = emitIdleChatTTS(ctx, bridge, ev)
	}
}

func isIdleChatTopicAnnouncement(ev idlechat.TimelineEvent) bool {
	content := strings.TrimSpace(ev.Content)
	return strings.EqualFold(ev.From, "user") &&
		strings.EqualFold(ev.To, "mio") &&
		idleChatTopicPrefixRe.MatchString(content)
}

func registerIdleChatTTSPending(sessionID string) <-chan struct{} {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	ch := make(chan struct{})
	idleChatTTSPending[sessionID] = ch
	return ch
}

func registerIdleChatTopicGate(idleSessionID, ttsSessionID string) {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	if _, ok := idleChatTopicGate[idleSessionID]; !ok {
		idleChatTopicGate[idleSessionID] = make(chan struct{})
	}
	idleChatTopicByTTS[ttsSessionID] = idleSessionID
}

func notifyIdleChatTTSCompleted(sessionID string) {
	idleChatTTSPendingMu.Lock()
	ch, ok := idleChatTTSPending[sessionID]
	if ok {
		delete(idleChatTTSPending, sessionID)
	}
	idleSessionID, topicOK := idleChatTopicByTTS[sessionID]
	if topicOK {
		delete(idleChatTopicByTTS, sessionID)
	}
	var topicCh chan struct{}
	if topicOK {
		topicCh = idleChatTopicGate[idleSessionID]
		delete(idleChatTopicGate, idleSessionID)
	}
	idleChatTTSPendingMu.Unlock()
	if ok {
		close(ch)
	}
	if topicCh != nil {
		// Unblock queued agent speech once the topic announcement session is fully completed.
		close(topicCh)
	}
}

func clearIdleChatTTSPending(sessionID string) {
	idleChatTTSPendingMu.Lock()
	delete(idleChatTTSPending, sessionID)
	if idleSessionID, ok := idleChatTopicByTTS[sessionID]; ok {
		delete(idleChatTopicByTTS, sessionID)
		if topicCh := idleChatTopicGate[idleSessionID]; topicCh != nil {
			delete(idleChatTopicGate, idleSessionID)
			close(topicCh)
		}
	}
	idleChatTTSPendingMu.Unlock()
}

func clearIdleChatTTSPendingByChan(target <-chan struct{}) {
	idleChatTTSPendingMu.Lock()
	defer idleChatTTSPendingMu.Unlock()
	for sessionID, ch := range idleChatTTSPending {
		if (<-chan struct{})(ch) == target {
			delete(idleChatTTSPending, sessionID)
			return
		}
	}
}

func waitIdleChatTopicGate(idleSessionID string) {
	idleChatTTSPendingMu.Lock()
	ch := idleChatTopicGate[idleSessionID]
	idleChatTTSPendingMu.Unlock()
	if ch == nil {
		return
	}
	<-ch
}

func idleChatVoiceForSpeaker(speaker string) (voiceID, voiceProfile string) {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "shiro":
		return idleChatMaleVoiceID, idleChatMaleVoiceProf
	default:
		return idleChatDefaultVoiceID, idleChatDefaultVoiceProf
	}
}

func idleChatTimeOfDay() string {
	hour := time.Now().Hour()
	if hour < 6 || hour >= 21 {
		return "night"
	}
	return "day"
}
