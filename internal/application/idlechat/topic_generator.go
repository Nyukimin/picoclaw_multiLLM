package idlechat

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TopicStrategy はトピック生成の戦略
type TopicStrategy string

const (
	StrategySingleGenre      TopicStrategy = "single"   // 1ジャンル単体 (25%)
	StrategyDoubleGenre      TopicStrategy = "double"   // 2ジャンル掛け合わせ (40%)
	StrategyExternalStimulus TopicStrategy = "external" // 外部刺激 (25%)
)

// genrePool はカオストピック生成用の多様なジャンル（260個）
var genrePool = []string{
	// === 学問・研究分野 (30) ===
	"昆虫学", "RNA生物学", "地理学", "音楽史", "教育学",
	"民俗学", "考古学", "言語学", "天文学", "地質学",
	"海洋学", "気象学", "植物学", "動物行動学", "生態学",
	"心理学", "社会学", "経済学", "人類学", "哲学",
	"倫理学", "論理学", "美学", "記号論", "文化人類学",
	"歴史学", "政治学", "法学", "医学", "薬学",

	// === 自然・環境・地理 (25) ===
	"火山活動", "氷河", "砂漠", "熱帯雨林", "サンゴ礁",
	"干潟", "湿地", "洞窟", "温泉", "地下水脈",
	"潮汐", "台風", "オーロラ", "地震", "津波",
	"侵食", "堆積", "風化", "結晶化", "化石化",
	"島嶼", "半島", "峡谷", "高原", "盆地",

	// === 生物・生命 (25) ===
	"共生", "寄生", "擬態", "冬眠", "渡り",
	"変態", "再生", "発光", "毒", "免疫",
	"発酵", "腐敗", "光合成", "呼吸", "代謝",
	"遺伝", "突然変異", "進化", "絶滅", "適応",
	"群れ", "縄張り", "求愛", "育児", "老化",

	// === 文化・芸術・伝統 (25) ===
	"茶道", "華道", "書道", "剣道", "柔道",
	"能", "歌舞伎", "狂言", "落語", "漫才",
	"俳句", "短歌", "和歌", "川柳", "狂歌",
	"盆栽", "生け花", "折り紙", "水墨画", "浮世絵",
	"陶芸", "漆芸", "染色", "織物", "金工",

	// === 音楽・芸能 (15) ===
	"交響詩", "室内楽", "ジャズ", "ブルース", "民謡",
	"雅楽", "声明", "詩吟", "吟詠", "朗読",
	"即興", "変奏", "対位法", "和声", "リズム",

	// === 社会制度・システム (20) ===
	"刑務所制度", "教育制度", "医療保険", "年金", "税制",
	"選挙", "裁判", "警察", "消防", "郵便",
	"通貨", "銀行", "株式", "保険", "賃金",
	"契約", "所有権", "著作権", "特許", "商標",

	// === 技術・工学 (20) ===
	"RNA治療", "遺伝子治療", "再生医療", "免疫療法", "放射線治療",
	"発酵技術", "醸造", "蒸留", "精製", "合成",
	"印刷", "製本", "活版", "写植", "組版",
	"測量", "製図", "設計", "施工", "保守",

	// === 日常・生活 (20) ===
	"睡眠", "食事", "掃除", "洗濯", "買い物",
	"料理", "調味", "盛り付け", "配膳", "片付け",
	"散歩", "ジョギング", "体操", "ストレッチ", "瞑想",
	"読書", "手紙", "日記", "メモ", "整理",

	// === 抽象概念・感情 (20) ===
	"記憶", "忘却", "認識", "錯覚", "デジャヴ",
	"夢", "悪夢", "白昼夢", "幻覚", "妄想",
	"信頼", "疑念", "希望", "絶望", "後悔",
	"郷愁", "孤独", "安心", "焦燥", "嫉妬",

	// === 時間・周期・暦 (15) ===
	"満月", "新月", "日食", "月食", "潮時",
	"春分", "秋分", "夏至", "冬至", "節分",
	"正月", "盆", "彼岸", "土用", "閏年",

	// === 物質・現象 (15) ===
	"蒸発", "凝縮", "昇華", "溶解", "析出",
	"燃焼", "酸化", "還元", "中和", "触媒",
	"共鳴", "干渉", "回折", "屈折", "反射",

	// === 空間・場所・建築 (20) ===
	"橋", "トンネル", "塔", "灯台", "ダム",
	"倉庫", "屋根裏", "地下室", "温室", "物置",
	"広場", "路地", "階段", "廊下", "中庭",
	"駐車場", "駅", "港", "空港", "停留所",

	// === 道具・機構 (15) ===
	"歯車", "バネ", "滑車", "てこ", "車輪",
	"ねじ", "釘", "ボルト", "ナット", "ワッシャー",
	"レンズ", "鏡", "プリズム", "フィルター", "センサー",

	// === 記号・表現・伝達 (15) ===
	"文字", "記号", "暗号", "署名", "印章",
	"身振り", "表情", "声色", "抑揚", "間",
	"比喩", "隠喩", "象徴", "寓話", "風刺",

	// === 遊び・娯楽 (10) ===
	"将棋", "囲碁", "麻雀", "トランプ", "すごろく",
	"かるた", "けん玉", "独楽", "凧揚げ", "紙相撲",

	// === その他・カオス (10) ===
	"噂", "迷信", "ジンクス", "都市伝説", "怪談",
	"占い", "予言", "呪文", "おまじない", "験担ぎ",
}

// DailySeedCache は1日1回取得する外部シードのキャッシュ
type DailySeedCache struct {
	Date           string    `json:"date"`
	WikipediaSeeds []string  `json:"wikipedia_seeds"`
	NewsSeeds      []string  `json:"news_seeds"`
	FetchedAt      time.Time `json:"fetched_at"`
}

var (
	dailyCache *DailySeedCache
	cacheMu    sync.RWMutex
)

// fetchDailySeeds は1日1回、起動時に外部シードを取得してキャッシュ
func fetchDailySeeds() error {
	today := time.Now().In(jst).Format("2006-01-02")

	cacheMu.RLock()
	if dailyCache != nil && dailyCache.Date == today {
		cacheMu.RUnlock()
		return nil // 既に取得済み
	}
	cacheMu.RUnlock()

	cacheMu.Lock()
	defer cacheMu.Unlock()

	// ダブルチェック
	if dailyCache != nil && dailyCache.Date == today {
		return nil
	}

	log.Printf("[IdleChat] Fetching daily seeds for %s...", today)

	// Wikipedia Random（10件）
	wikiSeeds, err := fetchWikipediaRandom(10)
	if err != nil {
		log.Printf("[IdleChat] Wikipedia fetch failed: %v", err)
		wikiSeeds = []string{} // フォールバック
	}

	// News Headlines（NHK RSS、10件）
	newsSeeds, err := fetchNewsHeadlines(10)
	if err != nil {
		log.Printf("[IdleChat] News fetch failed: %v", err)
		newsSeeds = []string{} // フォールバック
	}

	dailyCache = &DailySeedCache{
		Date:           today,
		WikipediaSeeds: wikiSeeds,
		NewsSeeds:      newsSeeds,
		FetchedAt:      time.Now(),
	}

	log.Printf("[IdleChat] Daily seeds fetched: Wikipedia=%d, News=%d", len(wikiSeeds), len(newsSeeds))
	return nil
}

// fetchWikipediaRandom はWikipedia Random APIから記事タイトルを取得
func fetchWikipediaRandom(limit int) ([]string, error) {
	url := fmt.Sprintf("https://ja.wikipedia.org/w/api.php?action=query&list=random&rnlimit=%d&rnnamespace=0&format=json", limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PicoClaw/1.0 (https://github.com/Nyukimin/picoclaw_multiLLM)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("wikipedia api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Query struct {
			Random []struct {
				Title string `json:"title"`
			} `json:"random"`
		} `json:"query"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	titles := make([]string, 0, len(result.Query.Random))
	for _, item := range result.Query.Random {
		titles = append(titles, item.Title)
	}

	return titles, nil
}

// fetchNewsHeadlines はNHK News RSSからヘッドラインを取得
func fetchNewsHeadlines(limit int) ([]string, error) {
	// NHK News RSS（トップニュース）
	url := "https://www.nhk.or.jp/rss/news/cat0.xml"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PicoClaw/1.0 (https://github.com/Nyukimin/picoclaw_multiLLM)")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nhk rss returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 簡易RSSパース（<title>タグ抽出）
	content := string(body)
	headlines := []string{}

	// <item>ブロック内の<title>を抽出
	inItem := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "<item>") {
			inItem = true
		} else if strings.HasPrefix(line, "</item>") {
			inItem = false
		} else if inItem && strings.HasPrefix(line, "<title>") {
			title := strings.TrimPrefix(line, "<title>")
			title = strings.TrimSuffix(title, "</title>")
			title = strings.TrimSpace(title)
			if title != "" && len(headlines) < limit {
				headlines = append(headlines, title)
			}
		}
	}

	return headlines, nil
}

// getDailyCache は現在のキャッシュを取得（スレッドセーフ）
func getDailyCache() *DailySeedCache {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return dailyCache
}

// pickRandom はスライスからn個をランダムに選択
func pickRandom(slice []string, n int) []string {
	if n >= len(slice) {
		// シャッフルして全て返す
		result := make([]string, len(slice))
		copy(result, slice)
		rand.Shuffle(len(result), func(i, j int) {
			result[i], result[j] = result[j], result[i]
		})
		return result
	}

	indices := rand.Perm(len(slice))[:n]
	result := make([]string, n)
	for i, idx := range indices {
		result[i] = slice[idx]
	}
	return result
}

// chooseStrategy は生成戦略をランダムに選択
// single: 40%, double: 30%, external: 30%
func chooseStrategy() TopicStrategy {
	r := rand.Intn(100)
	switch {
	case r < 40:
		return StrategySingleGenre
	case r < 70:
		return StrategyDoubleGenre
	default:
		return StrategyExternalStimulus
	}
}

func topicPromptFooter() string {
	return `回答はお題だけを1行で出力してください。
- 質問文・感想文・呼びかけは禁止
- 「〜って面白いんじゃない？」のような会話調は禁止
- 体言止め、または「〜の関係」「〜を考える」のような題名調にする
- 40文字以内を目安に簡潔にする`
}

// generateSingleGenrePrompt は1ジャンル単体のプロンプトを生成
func generateSingleGenrePrompt() (string, []string) {
	genres := pickRandom(genrePool, 1)

	bannedKeywords := extractBannedKeywords()

	prompt := fmt.Sprintf(`以下のジャンルを深掘りした、興味深い話題を1つ提案してください。

ジャンル: %s

要件:
- 深い洞察と新しい視点
- 会話が発展する具体性
- エンターテイメント性

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない
- 教科書的な真面目な説明は避ける
- 直近トピックと類似した内容は避ける

%s`, genres[0], strings.Join(bannedKeywords, "、"), topicPromptFooter())

	return prompt, genres
}

// generateDoubleGenrePrompt は2ジャンル掛け合わせのプロンプトを生成
func generateDoubleGenrePrompt() (string, []string) {
	genres := pickRandom(genrePool, 2)

	bannedKeywords := extractBannedKeywords()

	prompt := fmt.Sprintf(`以下の2つのジャンルを組み合わせた、面白い話題を1つ提案してください。

ジャンル: %s × %s

要件:
- 意外な組み合わせだが、深く考えると繋がりが見える
- 会話が深まる具体性
- 適度なエンターテイメント性

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない
- 教科書的な真面目な組み合わせは避ける
- 直近トピックと類似した内容は避ける

%s`, genres[0], genres[1], strings.Join(bannedKeywords, "、"), topicPromptFooter())

	return prompt, genres
}

// generateExternalPrompt は外部刺激を使ったプロンプトを生成
func generateExternalPrompt() (string, string) {
	cache := getDailyCache()
	if cache == nil {
		// フォールバック: 2ジャンル生成
		p, _ := generateDoubleGenrePrompt()
		return p, "fallback"
	}

	// Wikipedia or News からランダム選択
	var seed string
	var source string

	if len(cache.WikipediaSeeds) > 0 && len(cache.NewsSeeds) > 0 {
		if rand.Intn(2) == 0 {
			seed = cache.WikipediaSeeds[rand.Intn(len(cache.WikipediaSeeds))]
			source = "Wikipedia"
		} else {
			seed = cache.NewsSeeds[rand.Intn(len(cache.NewsSeeds))]
			source = "News"
		}
	} else if len(cache.WikipediaSeeds) > 0 {
		seed = cache.WikipediaSeeds[rand.Intn(len(cache.WikipediaSeeds))]
		source = "Wikipedia"
	} else if len(cache.NewsSeeds) > 0 {
		seed = cache.NewsSeeds[rand.Intn(len(cache.NewsSeeds))]
		source = "News"
	} else {
		// フォールバック: 2ジャンル
		p, _ := generateDoubleGenrePrompt()
		return p, "fallback"
	}

	genre := pickRandom(genrePool, 1)[0]
	bannedKeywords := extractBannedKeywords()

	prompt := fmt.Sprintf(`以下の外部刺激とジャンルを組み合わせた、意外性のある話題を1つ提案してください。

外部刺激 (%s): %s
組み合わせジャンル: %s

要件:
- 予想外の切り口を優先
- 深く考察できる具体的な話題
- エンターテイメント性重視

禁止事項:
- %s に関するトピックは避ける
- 「もし〜だったら」形式は使わない

%s`, source, seed, genre, strings.Join(bannedKeywords, "、"), topicPromptFooter())

	return prompt, source + ":" + seed
}

// extractBannedKeywords は頻出キーワードを抽出
func extractBannedKeywords() []string {
	return []string{
		"AI", "タイムマシン", "過去", "未来", "宇宙人",
		"もし", "だったら", "なら", "想像", "考えて",
	}
}
