package idlechat

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// simpleStoryTales は簡易版物語生成で使う昔話リスト。
var simpleStoryTales = []struct {
	title    string
	synopsis string
}{
	{"桃太郎", "川から桃が流れてきて生まれた子が、犬・猿・雉を仲間に鬼ヶ島へ鬼退治に行く"},
	{"一寸法師", "親指ほどの小さな武士が針を刀に都へ上り、鬼を倒して打ち出の小槌で大きくなる"},
	{"浦島太郎", "亀を助けた漁師が竜宮城へ招かれ、帰ると何百年も経っていて老人になる"},
	{"かぐや姫", "竹から生まれた娘が貴族たちの求婚を難題で退け、月へ帰っていく"},
	{"鶴の恩返し", "助けた鶴が娘に化けて機を織るが、見ることを禁じられた部屋を覗かれて去る"},
	{"舌切り雀", "親切な翁が舌を切られた雀を助け、意地悪な婆が欲張って痛い目に遭う"},
	{"花咲かじいさん", "犬の教えで金を掘り当てた翁が、灰で枯れ木に花を咲かせて殿様に褒められる"},
	{"さるかに合戦", "猿に騙されたカニの子が栗・蜂・臼と協力して仇を討つ"},
	{"笠地蔵", "雪の中の地蔵に笠をかぶせた翁夫婦の元へ、夜中に宝物が届く"},
	{"金太郎", "山で熊と相撲を取って育った怪力の子が、坂田金時として武将に仕える"},
}

const simpleStorySystemPrompt = `あなたは昔話リメイク作家です。ユーザーの指示に従って、笑えるくらい大袈裟で面白い短編を書いてください。`

// simpleStoryUserPrompt は1回のLLM呼び出しで物語全文を生成するプロンプト。
func simpleStoryUserPrompt(tale struct {
	title    string
	synopsis string
}, protagonist string) string {
	return fmt.Sprintf(`昔話「%s」を、主人公を「%s」に置き換えてリメイクしてください。

元の話のあらすじ: %s

条件:
- 主人公が「%s」になったことで、世界設定・社会の常識・登場人物の反応もすべて大胆に変わる
- 元の話の骨格（事件 → 解決 → オチ）は残す
- テンポよく、会話と描写を交えて
- 大げさなくらい面白く仕上げる（笑えるくらいでよい）
- 2000文字前後
- タイトルは1行目に「【タイトル】」形式で書く
- 本文のみ出力（解説・メタ発言不要）`, tale.title, protagonist, tale.synopsis, protagonist)
}

// protagonistOptions は主人公改変の候補リスト。
var protagonistOptions = []string{
	"AIロボット",
	"サラリーマン",
	"宅配業者",
	"YouTuber",
	"コンビニ店員",
	"定年退職したおじいさん",
	"高校生",
	"宇宙人",
	"忍者",
	"猫",
	"ドラゴン",
	"魔法使い見習い",
	"探偵",
	"料理人",
}

// StartSimpleStoryMode は簡易版物語モードを手動起動する。
func (o *IdleChatOrchestrator) StartSimpleStoryMode() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.participants) < 1 {
		return fmt.Errorf("idlechat requires at least 1 participant")
	}
	if o.chatActive {
		return fmt.Errorf("chat session already active")
	}
	o.manualMode = false
	o.chatActive = true
	o.sessionMode = "story-simple"
	o.currentTopic = ""
	o.lastActivity = time.Now()
	log.Println("[SimpleStory] Simple story mode started")
	return nil
}

// RunSimpleStorySession はCoder2（forecastProvider）を使った簡易版物語生成セッション。
// ワンプロンプトで昔話の主人公改変物語を生成し、Viewer に段落単位で配信する。
func (o *IdleChatOrchestrator) RunSimpleStorySession() {
	sessionID := fmt.Sprintf("story-simple-%d", time.Now().Unix())

	o.mu.Lock()
	o.chatActive = true
	o.sessionMode = "story-simple"
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.chatActive = false
		o.sessionMode = ""
		o.currentTopic = ""
		o.lastActivity = time.Now()
		o.mu.Unlock()
	}()

	// 昔話と主人公をランダム選択
	tale := simpleStoryTales[rand.Intn(len(simpleStoryTales))]
	protagonist := protagonistOptions[rand.Intn(len(protagonistOptions))]

	log.Printf("[SimpleStory] Generating: %s × %s", tale.title, protagonist)

	messages := []llm.Message{
		{Role: "system", Content: simpleStorySystemPrompt},
		{Role: "user", Content: simpleStoryUserPrompt(tale, protagonist)},
	}

	provider := o.forecastLLM()
	resp, err := provider.Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   2500,
		Temperature: 0.9,
	})
	if err != nil {
		log.Printf("[SimpleStory] generation failed: %v", err)
		return
	}

	raw := strings.TrimSpace(resp.Content)
	if raw == "" {
		log.Printf("[SimpleStory] empty response")
		return
	}

	// タイトル行と本文を分離
	titleLine := ""
	bodyLines := make([]string, 0)
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i == 0 && (strings.HasPrefix(line, "【") || strings.HasPrefix(line, "#")) {
			titleLine = strings.TrimPrefix(strings.TrimPrefix(line, "#"), " ")
			titleLine = strings.Trim(titleLine, "【】")
		} else {
			bodyLines = append(bodyLines, line)
		}
	}
	body := strings.Join(bodyLines, "\n")

	// 導入アナウンス
	intro := fmt.Sprintf("今夜の物語です。『%s』を、主人公を%sに置き換えたら——", tale.title, protagonist)
	o.emitStoryParagraph(sessionID, intro)

	if titleLine != "" {
		o.emitStoryParagraph(sessionID, fmt.Sprintf("改題は『%s』。", titleLine))
	}

	// 本文を段落単位でViewerに配信
	for _, para := range groupStoryIntoViewerParagraphs(body, 150) {
		o.emitStoryParagraph(sessionID, para)
	}

	// 締め
	closing := fmt.Sprintf("『%s』を下敷きにした、主人公%sのお話でした。", tale.title, protagonist)
	o.emitStoryParagraph(sessionID, closing)

	log.Printf("[SimpleStory] Session complete: %s × %s", tale.title, protagonist)
}
