package idlechat

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
	domaintransport "github.com/Nyukimin/picoclaw_multiLLM/internal/domain/transport"
)

func (o *IdleChatOrchestrator) generateTopicFromChat(sessionID string, strategy TopicStrategy) (string, TopicStrategy) {
	movieMode := rand.Intn(100) < 20
	recentTopics := o.getRecentTopics(12)

	var prompt string
	var logInfo string
	var fallbackTopic string

	switch strategy {
	case StrategySingleGenre:
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v anchor=%s", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", anchor, movieMode)

	case StrategyDoubleGenre:
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateDoubleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("double:%v anchor=%s", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(strategy, genres, "", "", anchor, movieMode)

	case StrategyExternalStimulus:
		var source string
		prompt, source = generateExternalPrompt(movieMode)
		logInfo = fmt.Sprintf("external:%s", source)
		fallbackTopic = fallbackTopicForStrategy(strategy, nil, source, "", topicAnchor{}, movieMode)

	default:
		// Fallback to single genre
		var genres []string
		var anchor topicAnchor
		prompt, genres, anchor = generateSingleGenrePrompt(movieMode)
		logInfo = fmt.Sprintf("single:%v anchor=%s (fallback)", genres, anchor.Value)
		fallbackTopic = fallbackTopicForStrategy(StrategySingleGenre, genres, "", "", anchor, movieMode)
	}

	if o.recentTopics != nil {
		if glossaryTopics, err := o.recentTopics(o.ctx, 6); err != nil {
			log.Printf("[IdleChat] glossary topics failed: %v", err)
		} else if len(glossaryTopics) > 0 {
			prompt += "\n\n最近語彙メモ:\n- " + strings.Join(glossaryTopics, "\n- ") + "\n上の語彙は、最近の時事語彙や固有名詞の種です。詳細断言ではなく、お題の発想補助として軽く使ってください。"
		}
	}

	log.Printf("[IdleChat] Strategy: %s (%s, movie=%t)", strategy, logInfo, movieMode)

	// トピック生成（最大3回リトライ）
	for attempt := 0; attempt < 3; attempt++ {
		messages := []llm.Message{
			{Role: "system", Content: idleTopicGeneratorSystemPrompt()},
			{Role: "user", Content: prompt},
		}
		req := llm.GenerateRequest{
			Messages:    messages,
			MaxTokens:   420,
			Temperature: 0.9 + float64(attempt)*0.05, // 高めの温度で多様性確保
		}
		resp, err := o.providerForSpeaker("mio").Generate(o.ctx, req)
		if err != nil {
			log.Printf("[IdleChat] topic generation failed: %v", err)
			break
		}
		logIdleRaw(fmt.Sprintf("topic.generate attempt=%d strategy=%s", attempt+1, strategy), resp.Content)
		topic := normalizeIdleTopic(resp.Content, movieMode)
		if topic == "" {
			continue
		}
		if topicTooSimilar(topic, recentTopics) {
			log.Printf("[IdleChat] topic too similar to recent history, retrying: %s", truncate(topic, 80))
			continue
		}
		log.Printf("[IdleChat] Topic: %s (%s)", topic, strategy)
		return topic, strategy
	}

	// フォールバック
	fallback := normalizeIdleTopic(fallbackTopic, movieMode)
	if fallback == "" {
		fallback = "予想外の切り口から考える論点"
	}
	log.Printf("[IdleChat] Topic (fallback): %s", fallback)
	return fallback, strategy
}

func fallbackTopicForStrategy(strategy TopicStrategy, genres []string, source string, seed string, anchor topicAnchor, movieMode bool) string {
	anchorValue := strings.TrimSpace(anchor.Value)
	switch strategy {
	case StrategySingleGenre:
		if len(genres) >= 1 && strings.TrimSpace(genres[0]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "の裏側")
			}
			if anchorValue != "" {
				return fmt.Sprintf("%sを%sの視点から考える", genres[0], anchorValue)
			}
			return fmt.Sprintf("%sで見落としがちな判断基準", genres[0])
		}
	case StrategyDoubleGenre:
		if len(genres) >= 2 && strings.TrimSpace(genres[0]) != "" && strings.TrimSpace(genres[1]) != "" {
			if movieMode {
				return formatMovieTopicPrompt(genres[0] + "と" + genres[1])
			}
			if anchorValue != "" {
				return fmt.Sprintf("%sと%sを%sでつなぐ", genres[0], genres[1], anchorValue)
			}
			return fmt.Sprintf("%sと%sに共通する設計思想", genres[0], genres[1])
		}
	case StrategyExternalStimulus:
		sourceName := source
		seedText := seed
		if strings.Contains(source, ":") {
			parts := strings.SplitN(source, ":", 2)
			sourceName = parts[0]
			seedText = parts[1]
		}
		if strings.TrimSpace(seedText) != "" {
			if movieMode {
				return formatMovieTopicPrompt(seedText)
			}
			return fmt.Sprintf("「%s」から掘る盲点と前提", seedText)
		}
		if strings.TrimSpace(sourceName) != "" {
			if movieMode {
				return formatMovieTopicPrompt(sourceName + "の裏側")
			}
			return fmt.Sprintf("%s由来の刺激から掘る盲点と前提", sourceName)
		}
	}
	if movieMode {
		return formatMovieTopicPrompt("予想外の切り口")
	}
	return "予想外の切り口から考える論点"
}

func normalizeIdleTopic(raw string, movieMode bool) string {
	s := strings.TrimSpace(extractVisibleLLMAnswer(raw))
	if s == "" {
		return ""
	}
	if hasPromptLeak(s) || hasInternalReasoningLeak(s) {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	replacers := []string{
		"話題:", "",
		"トピック:", "",
		"お題:", "",
		"話題：", "",
		"トピック：", "",
		"お題：", "",
		"\"", "",
	}
	s = strings.NewReplacer(replacers...).Replace(s)
	s = strings.TrimSpace(s)
	s = extractTopicTitleFromConversationalText(s)

	for _, marker := range []string{"、つまり、", "。つまり、", " つまり、", "っていうのは", "ってのは", "というのは"} {
		if idx := strings.Index(s, marker); idx > 0 {
			s = strings.TrimSpace(s[:idx])
			break
		}
	}
	for _, ending := range []string{
		"って、めちゃくちゃ面白いんじゃない？",
		"って、面白いんじゃない？",
		"って面白いんじゃない？",
		"ってどうだろう？",
		"じゃない？",
		"でしょうか？",
		"どうだろう？",
	} {
		s = strings.TrimSpace(strings.TrimSuffix(s, ending))
	}
	s = strings.TrimSpace(strings.TrimRight(s, "。！？!? "))
	s = multiSpaceForTopic(s)
	if s == "" || hasPromptLeak(s) || hasInternalReasoningLeak(s) || strings.HasPrefix(strings.TrimSpace(s), "<") || looksTruncatedIdleTopic(s) {
		return ""
	}
	if movieMode {
		return formatMovieTopicPrompt(s)
	}
	return strings.TrimSpace(s)
}

func looksTruncatedIdleTopic(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	if strings.HasSuffix(s, "、") || strings.HasSuffix(s, ",") {
		return true
	}
	for _, suffix := range []string{"そして", "また", "から", "ため", "との", "への", "取り", "取"} {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	if idx := strings.LastIndexAny(s, "、,"); idx >= 0 {
		tail := []rune(strings.TrimSpace(s[idx+len("、"):]))
		if len(tail) > 0 && len(tail) <= 2 {
			return true
		}
	}
	return false
}

func idleTopicGeneratorSystemPrompt() string {
	return `あなたはRenCrowのidleChat用お題生成器です。
キャラクターとして会話せず、感想・相づち・呼びかけ・絵文字を出さないでください。
出力はユーザーが指定した条件に合う「お題」本文だけを1行で返してください。`
}

func extractTopicTitleFromConversationalText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Trim(s, "「」『』\"' ")
	s = trimLeadingTopicReaction(s)
	for _, marker := range []string{"って組み合わせ", "という組み合わせ"} {
		if idx := strings.Index(s, marker); idx > 0 {
			return strings.TrimSpace(strings.Trim(s[:idx], "「」『』\"' "))
		}
	}
	for _, marker := range []string{"めっちゃ", "すごく", "なんか物語", "物語になりそう", "エモい"} {
		if idx := strings.Index(s, marker); idx > 0 {
			s = strings.TrimSpace(strings.TrimRight(s[:idx], "、。！？!? "))
			break
		}
	}
	return strings.TrimSpace(strings.Trim(s, "「」『』\"' "))
}

func trimLeadingTopicReaction(s string) string {
	for {
		trimmed := strings.TrimSpace(s)
		cut := -1
		for _, mark := range []string{"！", "!", "？", "?"} {
			if idx := strings.Index(trimmed, mark); idx >= 0 && utf8.RuneCountInString(trimmed[:idx]) < 40 {
				if cut == -1 || idx < cut {
					cut = idx
				}
			}
		}
		if cut < 0 {
			return trimmed
		}
		prefix := trimmed[:cut]
		if !containsAny(prefix, "えー", "うーん", "わあ", "おお", "なるほど", "たしかに") {
			return trimmed
		}
		s = strings.TrimSpace(trimmed[cut+len(string([]rune(trimmed[cut:])[0])):])
	}
}

func formatMovieTopicPrompt(raw string) string {
	title := strings.TrimSpace(raw)
	if title == "" {
		return ""
	}
	for {
		switch {
		case strings.HasPrefix(title, "「") && strings.HasSuffix(title, "」"):
			title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "「"), "」"))
			continue
		case strings.HasPrefix(title, "『") && strings.HasSuffix(title, "』"):
			title = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(title, "『"), "』"))
			continue
		}
		break
	}
	if idx := strings.Index(title, "ってどんな映画"); idx >= 0 {
		title = title[:idx]
	}
	title = strings.TrimSpace(strings.Trim(title, "「」『』\"'"))
	title = multiSpaceForTopic(title)
	if title == "" {
		return ""
	}
	if utf8.RuneCountInString(title) > 24 {
		title = truncate(title, 24)
		title = strings.TrimSpace(strings.TrimSuffix(title, "..."))
	}
	return fmt.Sprintf("「%s」ってどんな映画？", title)
}

func isMovieTopicPrompt(topic string) bool {
	s := strings.TrimSpace(topic)
	return strings.HasPrefix(s, "「") && strings.Contains(s, "」ってどんな映画")
}

func multiSpaceForTopic(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func collectLatestSessionSnippets(entries []session.ConversationEntry, match func(domaintransport.Message) bool, max int) []string {
	latestSessionID := ""
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if match(m) && strings.TrimSpace(m.SessionID) != "" {
			latestSessionID = m.SessionID
			break
		}
	}
	if latestSessionID == "" {
		return nil
	}

	snippets := make([]string, 0, max)
	for i := len(entries) - 1; i >= 0 && len(snippets) < max; i-- {
		m := entries[i].Message
		if m.SessionID == latestSessionID && match(m) {
			snippets = append(snippets, truncate(m.Content, 80))
		}
	}
	return snippets
}

func isIdleSession(sessionID string) bool {
	return strings.HasPrefix(strings.ToLower(sessionID), "idle-")
}

func isIdleMessage(m domaintransport.Message) bool {
	return m.Type == domaintransport.MessageTypeIdleChat || isIdleSession(m.SessionID)
}

func isWorkerMessage(m domaintransport.Message) bool {
	return strings.EqualFold(m.From, "shiro") || strings.EqualFold(m.To, "shiro")
}

func isUserMessage(m domaintransport.Message) bool {
	return strings.EqualFold(m.From, "user")
}
