package idlechat

import (
	"fmt"
	"log"
	"math/rand"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// generateForecastTopicInline は従来のインライン生成パイプライン。
func (o *IdleChatOrchestrator) generateForecastTopicInline(domain ForecastDomain) (string, []string) {
	trendSeeds := fetchTrendSeeds(domain)
	nhkSeeds := fetchDomainSeeds(domain, 10)
	allHeadlines := rankForecastSeeds(domain, append(trendSeeds, nhkSeeds...))
	keyword := o.extractForecastKeyword(domain, allHeadlines)
	deepSeeds := fetchGoogleNewsSeeds(keyword, 5)
	seeds := rankForecastSeeds(domain, append(allHeadlines, deepSeeds...))
	log.Printf("[Forecast] %s: keyword=%q trends=%d nhk=%d google=%d", domain.Name, keyword, len(trendSeeds), len(nhkSeeds), len(deepSeeds))
	topic := o.generateForecastTopic(domain, seeds)
	return topic, seeds
}

// fetchDomainSeeds は指定ドメインのRSSからシードを取得する。
func fetchDomainSeeds(domain ForecastDomain, limit int) []string {
	var all []string
	for _, rssURL := range domain.RSSURLs {
		headlines, err := fetchNewsHeadlinesFrom(rssURL, limit)
		if err != nil {
			log.Printf("[Forecast] RSS fetch failed (%s): %v", rssURL, err)
			continue
		}
		all = append(all, headlines...)
	}
	// 重複排除
	seen := make(map[string]struct{}, len(all))
	unique := make([]string, 0, len(all))
	for _, h := range all {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			unique = append(unique, h)
		}
	}
	if len(unique) > limit {
		rand.Shuffle(len(unique), func(i, j int) { unique[i], unique[j] = unique[j], unique[i] })
		unique = unique[:limit]
	}
	return unique
}

// generateForecastTopicPrompt はドメインとニュースシードから未来展望トピックのプロンプトを生成する。
func generateForecastTopicPrompt(domain ForecastDomain, seeds []string, avoidThemes []string) string {
	seedSection := ""
	if len(seeds) > 0 {
		picked := seeds
		if len(picked) > 5 {
			picked = pickRandom(seeds, 5)
		}
		seedSection = fmt.Sprintf("\n\n最新ニュース（%s）:\n- %s", domain.Name, strings.Join(picked, "\n- "))
	}
	angle := forecastTopicAngles[rand.Intn(len(forecastTopicAngles))]
	avoidSection := ""
	if len(avoidThemes) > 0 {
		picked := avoidThemes
		if len(picked) > 4 {
			picked = picked[:4]
		}
		avoidSection = fmt.Sprintf("\n\n避けたい既出テーマ:\n- %s", strings.Join(picked, "\n- "))
	}

	return fmt.Sprintf(`あなたは「%s」分野の未来を展望する議論のお題を1つ提案してください。%s%s

要件:
- 現在の動向・ニュースから3〜10年後の社会への影響を考えさせるお題
- 具体的な論点が含まれ、賛否両論が生まれるもの
- 「もし〜だったら」形式は使わない
- 楽観/悲観の両面から議論できるもの
- 今回は特に「%s」という切り口を強める
- 同じような論点の焼き直しを避け、切り口をずらす
- 既出テーマに近い案は避ける

回答はお題だけを1行で出力してください。
- 質問文・感想文は禁止
- 「%sの未来:」のような接頭辞は不要
- 体言止め、または「〜を考える」「〜の行方」のような題名調にする
- 50文字以内を目安に簡潔にする`, domain.Name, seedSection, avoidSection, angle, domain.Name)
}

// buildForecastLLMTopic は LLM に渡す詳細版トピックを構築する。
// 背景情報・ニュースシード・議論の方向性を含む。Viewer/TTS には使わない。
func buildForecastLLMTopic(domain ForecastDomain, displayTopic string, seeds []string) string {
	displayTopic = normalizeForecastDisplayTopic(domain, displayTopic)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("【%s 未来展望】%s\n\n", domain.Name, displayTopic))
	if len(seeds) > 0 {
		sb.WriteString("背景情報（最新ニュース・トレンド）:\n")
		shown := seeds
		if len(shown) > 8 {
			shown = shown[:8]
		}
		for _, s := range shown {
			sb.WriteString("- ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(`議論の方向性:
- 上記の最新動向を踏まえ、3〜10年後の社会への具体的な影響を議論する
- 楽観/悲観の両面から、根拠のある主張を展開する
- 抽象論ではなく、具体的な事例・数字・影響を挙げる
- 過去の類似事例との比較や、国際的な視点も取り入れる`)
	return sb.String()
}

func normalizeForecastDisplayTopic(domain ForecastDomain, topic string) string {
	if normalized := normalizeIdleTopic(topic, false); normalized != "" {
		return normalized
	}
	name := strings.TrimSpace(domain.Name)
	if name == "" {
		name = "未来展望"
	}
	return fmt.Sprintf("%sの3年後を考える", name)
}

// generateForecastTopic はドメイン特化のトピックをLLM生成する。
func (o *IdleChatOrchestrator) generateForecastTopic(domain ForecastDomain, seeds []string) string {
	recentTopics := o.getRecentTopics(12)
	pastTitleThemes := o.getHistoricalTitleThemes(500)

	for attempt := 0; attempt < 3; attempt++ {
		prompt := generateForecastTopicPrompt(domain, seeds, pastTitleThemes)
		messages := []llm.Message{
			{Role: "system", Content: o.getSystemPrompt("mio")},
			{Role: "user", Content: prompt},
		}
		req := llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   420,
			Temperature: 0.9 + float64(attempt)*0.05,
		}
		resp, err := o.forecastLLM().Generate(o.idleRunContext(), req)
		if err != nil {
			log.Printf("[Forecast] Topic generation failed: %v", err)
			break
		}
		logIdleRaw(fmt.Sprintf("forecast.topic.generate attempt=%d domain=%s", attempt+1, domain.Name), resp.Content)
		topic := normalizeIdleTopic(resp.Content, false)
		if topic == "" {
			continue
		}
		if topicTooSimilar(topic, recentTopics) {
			log.Printf("[Forecast] Topic too similar, retrying: %s", truncate(topic, 80))
			continue
		}
		if topicTooSimilar(topic, pastTitleThemes) {
			log.Printf("[Forecast] Topic overlaps with past title memory, retrying: %s", truncate(topic, 80))
			continue
		}
		return topic
	}

	// フォールバック
	if len(seeds) > 0 {
		return fmt.Sprintf("%sの最新動向から見る今後の展望", domain.Name)
	}
	return fmt.Sprintf("%sの3年後を考える", domain.Name)
}

// extractForecastKeyword はNHKヘッドラインからドメインに関連する注目キーワードを1つ抽出する。
func (o *IdleChatOrchestrator) extractForecastKeyword(domain ForecastDomain, headlines []string) string {
	if len(headlines) == 0 {
		return domain.Name
	}
	prompt := fmt.Sprintf(`以下は「%s」分野の最新情報です。

%s

この中から、今後の社会に最もインパクトがありそうな検索キーワードを1つだけ抽出してください。
- キーワードのみ出力（説明不要）
- 2〜6語程度の具体的な用語
- 一般的すぎる語（「技術」「問題」等）は避ける`, domain.Name, strings.Join(headlines, "\n"))

	messages := []llm.Message{
		{Role: "system", Content: "あなたはニュース分析の専門家です。"},
		{Role: "user", Content: prompt},
	}
	resp, err := o.forecastLLM().Generate(o.idleRunContext(), llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   30,
		Temperature: 0.5,
	})
	if err != nil {
		log.Printf("[Forecast] Keyword extraction failed: %v", err)
		return domain.Name
	}
	logIdleRaw("forecast.keyword.generate", resp.Content)
	kw := strings.TrimSpace(resp.Content)
	if kw == "" {
		return domain.Name
	}
	// 改行があれば最初の行だけ
	if i := strings.IndexAny(kw, "\r\n"); i >= 0 {
		kw = strings.TrimSpace(kw[:i])
	}
	return kw
}
