package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/orchestrator"
	ttsapp "github.com/Nyukimin/picoclaw_multiLLM/internal/application/tts"
)

type idleChatMockTTSBridge struct {
	startReqs    []orchestrator.TTSSessionStart
	pushTexts    []string
	displayTexts []string
	pushEmo      []*ttsapp.EmotionState
	endIDs       []string
	notifyOnEnd  bool
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

func (m *idleChatMockTTSBridge) PushTextWithDisplay(_ context.Context, sessionID string, text string, displayText string, emotion *ttsapp.EmotionState) error {
	_ = sessionID
	m.pushTexts = append(m.pushTexts, text)
	m.displayTexts = append(m.displayTexts, displayText)
	m.pushEmo = append(m.pushEmo, emotion)
	return nil
}

func (m *idleChatMockTTSBridge) EndSession(_ context.Context, sessionID string) error {
	m.endIDs = append(m.endIDs, sessionID)
	if m.notifyOnEnd {
		notifyIdleChatTTSCompleted(sessionID)
	}
	return nil
}

func TestEmitIdleChatTTSSendsMessage(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	_, _ = emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
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

	_, _ = emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
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

	_, _ = emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "user",
		To:        "mio",
		Content:   "今日のお題（external）: 震災の追悼の杜で、記憶と風景の関係をどう捉えたらどうだろう？",
		SessionID: "idle-topic-1",
	})

	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected 1 push text, got %d", len(bridge.pushTexts))
	}
	want := "きょうのおだい、震災の追悼の杜で、記憶と風景の関係をどう捉えたらどうだろう？"
	if bridge.pushTexts[0] != want {
		t.Fatalf("unexpected topic tts text: got %q want %q", bridge.pushTexts[0], want)
	}
	if got := bridge.displayTexts[0]; got != "今日のお題：震災の追悼の杜で、記憶と風景の関係をどう捉えたらどうだろう？" {
		t.Fatalf("unexpected topic display text: %q", got)
	}
	if got := bridge.startReqs[0].CharacterID; got != "user" {
		t.Fatalf("topic announcement should be attributed to Ren/user, got %q", got)
	}
}

func TestEmitIdleChatTTSAsyncTopicAnnouncementReturnsCompletion(t *testing.T) {
	bridge := &idleChatMockTTSBridge{notifyOnEnd: true}

	done := emitIdleChatTTSAsync(bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "user",
		To:        "mio",
		Content:   "今日のお題（external）: 記憶と風景の関係",
		SessionID: "idle-topic-async",
	})
	if done == nil {
		t.Fatal("expected topic announcement to return a completion channel")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("topic TTS completion was not signaled")
	}
	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected topic TTS to be pushed, got %d", len(bridge.pushTexts))
	}
}

func TestEmitIdleChatTTSAsyncSerializesIdleSpeech(t *testing.T) {
	bridge := &idleChatMockTTSBridge{notifyOnEnd: true}

	first := emitIdleChatTTSAsync(bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "shiro",
		Content:   "先の発話です。",
		SessionID: "idle-serial-1",
	})
	second := emitIdleChatTTSAsync(bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "shiro",
		To:        "mio",
		Content:   "後の発話です。",
		SessionID: "idle-serial-1",
	})

	for name, done := range map[string]<-chan struct{}{"first": first, "second": second} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s TTS completion was not signaled", name)
		}
	}
	if len(bridge.pushTexts) < 2 {
		t.Fatalf("expected two serialized pushes, got %d", len(bridge.pushTexts))
	}
	if bridge.pushTexts[len(bridge.pushTexts)-2] != "先の発話です。" || bridge.pushTexts[len(bridge.pushTexts)-1] != "後の発話です。" {
		t.Fatalf("speech was not serialized in enqueue order: %#v", bridge.pushTexts)
	}
}

func TestEmitIdleChatTTSAsyncPrefetchesWithoutPlaybackCompletion(t *testing.T) {
	bridge := &idleChatMockTTSBridge{notifyOnEnd: false}

	first := emitIdleChatTTSAsync(bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "shiro",
		Content:   "先に合成する発話です。",
		SessionID: "idle-prefetch-1",
	})
	second := emitIdleChatTTSAsync(bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "shiro",
		To:        "mio",
		Content:   "再生完了を待たずに合成する発話です。",
		SessionID: "idle-prefetch-1",
	})

	for name, done := range map[string]<-chan struct{}{"first": first, "second": second} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("%s synthesis completion was not signaled", name)
		}
	}
	if len(bridge.pushTexts) < 2 {
		t.Fatalf("expected queued speech to be synthesized without playback completion, got %d pushes", len(bridge.pushTexts))
	}
	if bridge.pushTexts[len(bridge.pushTexts)-2] != "先に合成する発話です。" ||
		bridge.pushTexts[len(bridge.pushTexts)-1] != "再生完了を待たずに合成する発話です。" {
		t.Fatalf("unexpected synthesis order: %#v", bridge.pushTexts)
	}
}

func TestEmitIdleChatTTS_RemovesLoopNotesFromSpeechOnly(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}
	content := "今回のまとめです。\n注記: テンプレ反復で打ち切り\n\n本文を読み上げます。"

	_, _ = emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
		Type:      "idlechat.message",
		From:      "mio",
		To:        "user",
		Content:   content,
		SessionID: "idle-note-1",
	})

	if len(bridge.pushTexts) != 1 {
		t.Fatalf("expected 1 push text, got %d", len(bridge.pushTexts))
	}
	if strings.Contains(bridge.pushTexts[0], "注記:") {
		t.Fatalf("note leaked into speech text: %q", bridge.pushTexts[0])
	}
	if !strings.Contains(bridge.pushTexts[0], "今回のまとめです。") || !strings.Contains(bridge.pushTexts[0], "本文を読み上げます。") {
		t.Fatalf("unexpected speech text: %q", bridge.pushTexts[0])
	}
	if got := formatIdleChatDisplayText(idlechat.TimelineEvent{Content: content}); !strings.Contains(got, "注記: テンプレ反復で打ち切り") {
		t.Fatalf("display text should keep note, got %q", got)
	}
}

func TestEmitIdleChatTTSSkipsNonMessageEvent(t *testing.T) {
	bridge := &idleChatMockTTSBridge{}

	_, _ = emitIdleChatTTS(context.Background(), bridge, idlechat.TimelineEvent{
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
	if voiceID != "mio" || voiceProfile != "lumina_female" {
		t.Fatalf("unexpected mio voice mapping: %q %q", voiceID, voiceProfile)
	}
}
