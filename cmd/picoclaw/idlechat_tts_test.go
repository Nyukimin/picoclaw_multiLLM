package main

import (
	"context"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

type idleChatMockTTSBridge struct {
	startReqs []orchestrator.TTSSessionStart
	pushTexts []string
	pushEmo   []*ttsapp.EmotionState
	endIDs    []string
}

func (m *idleChatMockTTSBridge) StartSession(_ context.Context, req orchestrator.TTSSessionStart) error {
	m.startReqs = append(m.startReqs, req)
	return nil
}

func (m *idleChatMockTTSBridge) PushText(_ context.Context, sessionID string, text string, emotion *ttsapp.EmotionState) error {
	_ = sessionID
	m.pushTexts = append(m.pushTexts, text)
	m.pushEmo = append(m.pushEmo, emotion)
	return nil
}

func (m *idleChatMockTTSBridge) EndSession(_ context.Context, sessionID string) error {
	m.endIDs = append(m.endIDs, sessionID)
	return nil
}

func TestEmitIdleChatTTSSendsMessage(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "shiro",
		To:        "mio",
		Content:   "はい、承知いたしました。おはようございます！",
		SessionID: "idle-1",
	})

	if len(bridge.startReqs) != 1 {
		t.Fatalf("expected 1 start request, got %d", len(bridge.startReqs))
	}
	if bridge.startReqs[0].VoiceID != "male_01" {
		t.Fatalf("expected male_01 voice, got %q", bridge.startReqs[0].VoiceID)
	}
	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected 1 push text, got %d", len(bridge.pushTexts))
	}
	if got := bridge.pushTexts[0]; got != "おはようございます！" {
		t.Fatalf("unexpected filtered text: %q", got)
	}
	if len(bridge.pushEmo) != 1 || bridge.pushEmo[0] == nil {
		t.Fatal("expected emotion payload")
	}
	if len(bridge.endIDs) != 1 {
		t.Fatalf("expected 1 end request, got %d", len(bridge.endIDs))
	}
}

func TestEmitIdleChatTTS_AppendsSentencePauseForAgentMessage(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "shiro",
		Content:   "次は別の観点で見てみよう",
		SessionID: "idle-3",
	})

	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected 1 push text, got %d", len(bridge.pushTexts))
	}
	if got := bridge.pushTexts[0]; got != "次は別の観点で見てみよう。" {
		t.Fatalf("unexpected filtered text: %q", got)
	}
}

func TestEmitIdleChatTTS_FormatsTopicAnnouncement(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "user",
		To:        "mio",
		Content:   "今日のお題（external）: 震災の追悼の杜で、記憶と風景の関係をどう捉えたらどうだろう？",
		SessionID: "idle-topic-1",
	})

	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected 1 push text, got %d", len(bridge.pushTexts))
	}
	want := "今日のお題です。。震災の追悼の杜で、記憶と風景の関係をどう捉えたらどうだろう？。。です！。"
	if bridge.pushTexts[0] != want {
		t.Fatalf("unexpected topic tts text: got %q want %q", bridge.pushTexts[0], want)
	}
}

func TestEmitIdleChatTTSSkipsNonMessageEvent(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.summary",
		From:      "shiro",
		Content:   "summary",
		SessionID: "idle-2",
	})

	if len(bridge.startReqs) != 0 || len(bridge.pushTexts) != 0 || len(bridge.endIDs) != 0 {
		t.Fatal("expected no tts calls for non-message event")
	}
}

func TestIdleChatVoiceForSpeaker(t *testing.T) {
	voiceID, voiceProfile := idleChatVoiceForSpeaker("shiro")
	if voiceID != "male_01" || voiceProfile != "lumina_male" {
		t.Fatalf("unexpected shiro voice mapping: %q %q", voiceID, voiceProfile)
	}
	voiceID, voiceProfile = idleChatVoiceForSpeaker("mio")
	if voiceID != "female_01" || voiceProfile != "lumina_female" {
		t.Fatalf("unexpected mio voice mapping: %q %q", voiceID, voiceProfile)
	}
}
