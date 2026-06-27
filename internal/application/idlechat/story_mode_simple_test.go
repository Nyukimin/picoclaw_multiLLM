package idlechat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	modulechat "github.com/Nyukimin/picoclaw_multiLLM/modules/chat"
)

func TestValidateSimpleStoryDraftRejectsWeakStory(t *testing.T) {
	result := validateSimpleStoryDraft("桃太郎", "AIロボット", "もし桃太郎がAIロボットだったら面白いです。", "仮説だけの短文です。")
	if result.Valid {
		t.Fatal("expected invalid story")
	}
	if result.Reason == "" {
		t.Fatal("expected validation reason")
	}
}

func TestValidateSimpleStoryDraftAcceptsChangedProtagonistStory(t *testing.T) {
	body := strings.Repeat("AIロボットは村の回覧板を解析し、犬と猿と雉に役割を配った。鬼ヶ島では交渉ログを突きつけ、盗まれた米俵を取り戻した。", 4) + "こうして村の倉庫は守られ、AIロボットは毎朝、桃の糖度と鬼の反省文を同じ棚に保存するようになったのでした。"
	result := validateSimpleStoryDraft("桃太郎", "AIロボット", "桃と回覧板のロボ太郎", body)
	if !result.Valid {
		t.Fatalf("expected valid story, got %s", result.Reason)
	}
}

func TestSimpleStoryTopicKeepsBaseAndTransform(t *testing.T) {
	result := buildSimpleStoryTopicResult("桃太郎", "AIロボット")
	if result.Category != TopicCategoryStorySimple {
		t.Fatalf("category = %q, want story-simple", result.Category)
	}
	if result.Strategy != "story-simple" {
		t.Fatalf("strategy = %q, want story-simple", result.Strategy)
	}
	if !strings.Contains(result.Topic, "桃太郎") || !strings.Contains(result.Topic, "AIロボット") || !strings.Contains(result.Topic, "語り直") {
		t.Fatalf("story topic lost base or transform axis: %q", result.Topic)
	}
	if err := modulechat.ValidateTopicCandidate(TopicCategoryStorySimple, result.Seed, result.Candidates[0]); err != nil {
		t.Fatalf("story topic candidate should satisfy contract: %v", err)
	}
}

func TestRunSimpleStorySessionRevisesWeakDraftBeforeSaving(t *testing.T) {
	var taleTitles []string
	for _, tale := range simpleStoryTales {
		taleTitles = append(taleTitles, tale.title)
	}
	actors := strings.Join(protagonistOptions, "と")
	sources := strings.Join(taleTitles, "と")
	revisedBody := strings.Repeat(actors+"は"+sources+"の事件を村の困りごととしてログにまとめた。仲間には役割が割り振られ、盗まれた米俵の配送履歴が証拠になった。", 4) + "こうして事件は解決し、主人公たちは鬼たちに反省文の自動保存を教えて村へ帰ったのでした。"
	provider := &queuedQualityProvider{responses: []string{
		"【もしもの桃太郎】\nもし桃太郎がAIロボットだったら面白いです。",
		"QUALITY: fail\nSCORE: 45\nISSUES:\n- 企画文のままで物語になっていない\nREVISION_PROMPT: 事件と解決とオチまで本文にする。",
		"【桃ログ太郎】\n" + revisedBody,
		"QUALITY: pass\nISSUES:\n- なし\nPROMPT_FIX: ",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil)
	defer o.Stop()
	closed := make(chan struct{})
	close(closed)
	o.SetEventEmitter(func(TimelineEvent) <-chan struct{} {
		return closed
	})

	o.RunSimpleStorySession()

	history := o.GetHistory(1)
	if len(history) != 1 {
		t.Fatalf("history count=%d, want 1", len(history))
	}
	if history[0].LoopRestarted || history[0].LoopReason != "" {
		t.Fatalf("revised story should be saved as successful: %#v", history[0])
	}
	if !strings.Contains(history[0].StoryText, "反省文の自動保存") {
		t.Fatalf("revised story body should be stored: %q", history[0].StoryText)
	}
}

func TestRunSimpleStorySessionUsesCompletedStockWithoutGeneratingDraft(t *testing.T) {
	body := strings.Repeat("AIロボットは村の困りごとを解析し、犬と猿と雉に役割を配った。鬼ヶ島では証拠ログを開き、盗まれた米俵を取り戻した。", 3) + "こうして事件は解決し、AIロボットは村の見守り台として朝まで光っていたのでした。"
	provider := &queuedQualityProvider{responses: []string{
		"QUALITY: pass\nISSUES:\n- なし\nPROMPT_FIX: ",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil)
	defer o.Stop()
	o.simpleStoryTopicStock = newSimpleStoryTopicStock()
	o.simpleStoryTopicStock.push(simpleStoryPreparedTopic{
		Tale:        simpleStoryTales[0],
		Protagonist: "AIロボット",
		Result:      buildSimpleStoryTopicResult("桃太郎", "AIロボット"),
		StoryTitle:  "桃ログ太郎",
		StoryText:   body,
	})

	events := make(chan TimelineEvent, 32)
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		events <- ev
		return nil
	})

	o.RunSimpleStorySession()

	select {
	case ev := <-events:
		if ev.Type != "idlechat.viewer" || ev.Content == "" {
			t.Fatalf("first event = %#v, want viewer intro", ev)
		}
	default:
		t.Fatal("expected viewer event from completed stock")
	}
	history := o.GetHistory(1)
	if len(history) != 1 || history[0].StoryText != body {
		t.Fatalf("story session should use completed stock body: %#v", history)
	}
}

func TestRunSimpleStorySessionEmitsUniqueStoryTTSMessageIDs(t *testing.T) {
	body := strings.Repeat(strings.Join(protagonistOptions, "と")+"が村の困りごとを調べ、仲間の反応を変えながら事件を解決した。", 3)
	provider := &queuedQualityProvider{responses: []string{
		"【主人公たちの改変昔話】\n" + body,
		"QUALITY: pass\nISSUES:\n- なし\nPROMPT_FIX: ",
	}}
	o := NewIdleChatOrchestrator(provider, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil)
	defer o.Stop()

	seen := map[string]bool{}
	var ttsIDs []string
	closed := make(chan struct{})
	close(closed)
	o.SetEventEmitter(func(ev TimelineEvent) <-chan struct{} {
		if ev.Type != "idlechat.tts" {
			return nil
		}
		if ev.MessageID == "" {
			t.Fatal("story TTS message id is empty")
		}
		if seen[ev.MessageID] {
			t.Fatalf("duplicate story TTS message id: %s", ev.MessageID)
		}
		seen[ev.MessageID] = true
		ttsIDs = append(ttsIDs, ev.MessageID)
		return closed
	})

	o.RunSimpleStorySession()

	if len(ttsIDs) < 4 {
		t.Fatalf("story TTS id count=%d, want at least 4: %#v", len(ttsIDs), ttsIDs)
	}
	for i, id := range ttsIDs {
		want := fmt.Sprintf(":story-simple:%04d", i+1)
		if !strings.Contains(id, want) {
			t.Fatalf("story TTS id[%d]=%q, want sequential suffix containing %q", i, id, want)
		}
	}
}

func TestStartModesExposeNonEmptyCurrentTopicImmediately(t *testing.T) {
	tests := []struct {
		name  string
		start func(*IdleChatOrchestrator) error
	}{
		{
			name:  "forecast",
			start: func(o *IdleChatOrchestrator) error { return o.StartForecastMode() },
		},
		{
			name:  "story-simple",
			start: func(o *IdleChatOrchestrator) error { return o.StartSimpleStoryMode() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewIdleChatOrchestrator(nil, session.NewCentralMemory(), []string{"mio", "shiro"}, 5, 10, 0.8, nil)
			if err := tt.start(o); err != nil {
				t.Fatalf("start failed: %v", err)
			}
			if got := o.CurrentTopic(); got == "" {
				t.Fatal("current topic is empty immediately after start")
			}
		})
	}
}
