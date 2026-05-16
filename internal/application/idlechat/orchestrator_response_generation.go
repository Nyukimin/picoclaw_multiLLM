package idlechat

import (
	"context"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func (o *IdleChatOrchestrator) generateResponse(speaker, target, sessionID string, turn int, segmentTurns int, topic string) (string, error) {
	response, _, err := o.generateResponseWithRaw(speaker, target, sessionID, turn, segmentTurns, topic)
	return response, err
}

func (o *IdleChatOrchestrator) generateResponseWithRaw(speaker, target, sessionID string, turn int, segmentTurns int, topic string) (string, string, error) {
	topic = o.resolveDialogueTopic(sessionID, speaker, topic)
	systemPrompt := o.getSystemPrompt(speaker)
	temp := o.temperatureForSpeaker(speaker)

	// 履歴は浅めにして、古いテンプレが自己強化しないようにする。
	recentEntries := o.memory.GetUnifiedView(12)
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
	}
	selfCtx, otherCtx := splitSpeakerContexts(recentEntries, sessionID, speaker, 2)
	latestOther := latestOtherUtterance(recentEntries, sessionID, speaker)
	latestSelf := latestSelfUtterance(recentEntries, sessionID, speaker)

	// OpenAI互換サーバによっては system message が先頭以外にあると拒否するため、
	// 追加の system 文脈は履歴や user 指示より前に集約する。
	o.mu.Lock()
	sc := o.sessionContext
	o.mu.Unlock()
	if sc != "" {
		messages[0].Content += "\n\n" + sc
	}
	if o.recentTopics != nil {
		if glossaryTopics, err := o.recentTopics(o.ctx, 5); err != nil {
			log.Printf("[IdleChat] glossary context failed: %v", err)
		} else if len(glossaryTopics) > 0 {
			messages[0].Content += "\n\n最近語彙メモ:\n- " + strings.Join(glossaryTopics, "\n- ") + "\n最近語彙は会話の種としてだけ使い、詳細断言はしないでください。"
		}
	}

	sessionEntries := make([]session.ConversationEntry, 0, 4)
	for i := len(recentEntries) - 1; i >= 0 && len(sessionEntries) < 4; i-- {
		if recentEntries[i].Message.SessionID == sessionID {
			sessionEntries = append(sessionEntries, recentEntries[i])
		}
	}
	for i := len(sessionEntries) - 1; i >= 0; i-- {
		entry := sessionEntries[i]
		role := "assistant"
		if entry.Message.From != speaker {
			role = "user"
		}
		messages = append(messages, llm.Message{
			Role:    role,
			Content: fmt.Sprintf("[%s]: %s", entry.Message.From, entry.Message.Content),
		})
	}

	messages = append(messages, llm.Message{
		Role:    "user",
		Content: buildIdleResponseGuardPrompt(speaker, selfCtx, otherCtx),
	})
	if isMovieTopicPrompt(topic) {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: "これは架空映画の妄想会話です。実在作品として扱わず、『聞いたことがある』『前に見た』『有名作だ』のような既知前提は禁止。抽象論より、主人公・事件・場面・対立・反転を早めに一つ出してください。",
		})
	}

	if turn == 0 {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildIdleTurnPrompt(topic, speaker, "", "", turn, segmentTurns, true),
		})
	} else {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: buildIdleTurnPrompt(topic, speaker, latestOther, latestSelf, turn, segmentTurns, false),
		})
	}

	req := llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatResponseMaxTokens),
		Temperature: temp,
	}

	provider := o.providerForSpeaker(speaker)
	resp, err := o.generateIdleLLM(provider, req)
	if err != nil {
		log.Printf("[IdleChat] LLM generate primary failed (%s turn=%d): %v", speaker, turn, err)
		return "", "", fmt.Errorf("idlechat dialogue generation failed: speaker=%s turn=%d: %w", speaker, turn, err)
	}
	logIdleRaw(fmt.Sprintf("dialogue.primary speaker=%s turn=%d", speaker, turn), resp.Content)
	firstRaw := strings.TrimSpace(resp.Content)
	first := sanitizeIdleResponse(resp.Content, topic)
	firstTruncated := finishReasonLooksTruncated(resp.FinishReason)
	if firstTruncated {
		log.Printf("[IdleChat] primary truncated (%s turn=%d): finish=%q max_tokens=%d", speaker, turn, resp.FinishReason, req.MaxTokens)
	}
	if !firstTruncated && !unusableIdleResponse(firstRaw, first) {
		secondMessages := append([]llm.Message{}, messages...)
		secondMessages = append(secondMessages, llm.Message{
			Role:    "assistant",
			Content: first,
		})
		secondMessages = append(secondMessages, llm.Message{
			Role:    "user",
			Content: "今の発話とは別候補を1つだけ出してください。前候補と同じ書き出し・同じ比喩・同じ結論を避け、読者の楽しさが上がるように、具体物・選択・秘密・感情の反転のどれかを一つだけ入れてください。英語だけの応答、英語の見出し、英語での説明は禁止です。候補番号、評価文、説明、ルール確認は書かず、発話として読める自然な日本語だけを1-2文で返してください。",
		})
		respSecond, errSecond := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    secondMessages,
			MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatRetryMaxTokens),
			Temperature: temp,
		})
		if errSecond != nil {
			log.Printf("[IdleChat] fun candidate B failed (%s turn=%d): %v", speaker, turn, errSecond)
		} else {
			logIdleRaw(fmt.Sprintf("dialogue.candidate_b speaker=%s turn=%d", speaker, turn), respSecond.Content)
			secondRaw := strings.TrimSpace(respSecond.Content)
			second := sanitizeIdleResponse(respSecond.Content, topic)
			if finishReasonLooksTruncated(respSecond.FinishReason) || unusableIdleResponse(secondRaw, second) {
				log.Printf("[IdleChat] fun candidate B unusable (%s turn=%d): raw=%q sanitized=%q", speaker, turn, truncate(secondRaw, 180), truncate(second, 180))
			} else {
				firstScore := idleFunScorePercent(first, latestOther, latestSelf, topic)
				secondScore := idleFunScorePercent(second, latestOther, latestSelf, topic)
				log.Printf("[IdleChat] fun candidate scores (%s turn=%d): A=%d%% B=%d%%", speaker, turn, firstScore, secondScore)
				if secondScore > firstScore {
					firstRaw = secondRaw
					first = second
					firstTruncated = false
				}
			}
		}
	}
	if firstTruncated || unusableIdleResponse(firstRaw, first) {
		retryInvalid := buildIdleCompactRetryMessages(speaker, topic, latestOther, firstTurnLabel(turn))
		respInvalid, errInvalid := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryInvalid,
			MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatRetryMaxTokens),
			Temperature: temp,
		})
		if errInvalid != nil {
			log.Printf("[IdleChat] retryInvalid failed (%s turn=%d): %v", speaker, turn, errInvalid)
		}
		if errInvalid == nil && strings.TrimSpace(respInvalid.Content) != "" {
			logIdleRaw(fmt.Sprintf("dialogue.retry_invalid speaker=%s turn=%d", speaker, turn), respInvalid.Content)
			first = sanitizeIdleResponse(respInvalid.Content, topic)
			firstRaw = strings.TrimSpace(respInvalid.Content)
			firstTruncated = finishReasonLooksTruncated(respInvalid.FinishReason)
		}
	}
	if needsIdleStyleRetry(speaker, first, latestOther, latestSelf, topic) {
		retryStyle := append([]llm.Message{}, messages...)
		retryStyle = append(retryStyle, llm.Message{
			Role:    "user",
			Content: "評価や言い直し宣言は書かず、別の手で自然な日本語だけで返してください。英語だけの応答、英語の見出し、英語での説明は禁止です。直前の言い回しをなぞらず、1文目で相手の論点に反応し、2文目で具体物・選択・秘密・感情の反転のどれかを一つだけ足してください。",
		})
		respStyle, errStyle := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryStyle,
			MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatRetryMaxTokens),
			Temperature: temp,
		})
		if errStyle != nil {
			log.Printf("[IdleChat] retryStyle failed (%s turn=%d): %v", speaker, turn, errStyle)
		}
		if errStyle == nil && strings.TrimSpace(respStyle.Content) != "" {
			logIdleRaw(fmt.Sprintf("dialogue.retry_style speaker=%s turn=%d", speaker, turn), respStyle.Content)
			first = sanitizeIdleResponse(respStyle.Content, topic)
			firstRaw = strings.TrimSpace(respStyle.Content)
			firstTruncated = finishReasonLooksTruncated(respStyle.FinishReason)
		}
	}
	if hasPromptLeak(firstRaw) || hasPromptLeak(first) || hasInternalReasoningLeak(firstRaw) || hasInternalReasoningLeak(first) {
		retryLeak := buildIdleCompactRetryMessages(speaker, topic, latestOther, "内部推論を出さずに本文だけで再生成")
		respLeak, errLeak := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retryLeak,
			MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatRetryMaxTokens),
			Temperature: temp,
		})
		if errLeak != nil {
			log.Printf("[IdleChat] retryLeak failed (%s turn=%d): %v", speaker, turn, errLeak)
		}
		if errLeak == nil && strings.TrimSpace(respLeak.Content) != "" {
			logIdleRaw(fmt.Sprintf("dialogue.retry_leak speaker=%s turn=%d", speaker, turn), respLeak.Content)
			first = sanitizeIdleResponse(respLeak.Content, topic)
			firstRaw = strings.TrimSpace(respLeak.Content)
			firstTruncated = finishReasonLooksTruncated(respLeak.FinishReason)
		}
	}
	if violatesAttribution(first, latestOther) {
		retry := append([]llm.Message{}, messages...)
		retry = append(retry, llm.Message{
			Role:    "user",
			Content: "発言帰属が曖昧です。相手の案を受けてから、自分の新しい具体物・選択・秘密・感情の反転を一つだけ足し、自然な日本語1-2文で言い直してください。英語だけの応答、英語の見出し、英語での説明は禁止です。",
		})
		resp2, err2 := o.generateIdleLLM(provider, llm.GenerateRequest{
			Messages:    retry,
			MaxTokens:   idleMaxTokensForSpeaker(speaker, idleChatRetryMaxTokens),
			Temperature: temp,
		})
		if err2 != nil {
			log.Printf("[IdleChat] retryAttribution failed (%s turn=%d): %v", speaker, turn, err2)
		}
		if err2 == nil && strings.TrimSpace(resp2.Content) != "" {
			logIdleRaw(fmt.Sprintf("dialogue.retry_attribution speaker=%s turn=%d", speaker, turn), resp2.Content)
			candidateRaw := strings.TrimSpace(resp2.Content)
			candidate := sanitizeIdleResponse(resp2.Content, topic)
			if finishReasonLooksTruncated(resp2.FinishReason) || unusableIdleResponse(candidateRaw, candidate) {
				log.Printf("[IdleChat] retryAttribution unusable (%s turn=%d): raw=%q sanitized=%q", speaker, turn, truncate(candidateRaw, 180), truncate(candidate, 180))
				return "", candidateRaw, fmt.Errorf("idlechat dialogue retry_attribution unusable: speaker=%s turn=%d", speaker, turn)
			}
			return candidate, candidateRaw, nil
		}
	}

	if firstTruncated || unusableIdleResponse(firstRaw, first) {
		log.Printf("[IdleChat] unusable response rejected (%s turn=%d): truncated=%t raw=%q sanitized=%q", speaker, turn, firstTruncated, truncate(firstRaw, 180), truncate(first, 180))
		return "", firstRaw, fmt.Errorf("idlechat dialogue response unusable: speaker=%s turn=%d truncated=%t", speaker, turn, firstTruncated)
	}

	return first, firstRaw, nil
}

func firstTurnLabel(turn int) string {
	if turn == 0 {
		return "会話の最初の発話"
	}
	return "直前の相手発言への返答"
}

func idleMaxTokensForSpeaker(speaker string, defaultMax int) int {
	if strings.EqualFold(strings.TrimSpace(speaker), "shiro") {
		if defaultMax <= idleChatRetryMaxTokens {
			return idleChatShiroRetryMaxTokens
		}
		return idleChatShiroResponseMaxTokens
	}
	return defaultMax
}

func buildIdleCompactRetryMessages(speaker, topic, latestOther, purpose string) []llm.Message {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "この会話の現在のお題"
	}
	other := strings.TrimSpace(latestOther)
	if other == "" {
		other = "-"
	}
	style := "自然な日本語で1-2文。英語だけの応答、英語の見出し、英語での説明は禁止。表示される会話本文だけを返す。具体物か小さな問いを一つ入れる。"
	if strings.EqualFold(strings.TrimSpace(speaker), "mio") {
		style += " Mioとしてタメ口で、明るく好奇心のある入口にする。"
	} else if strings.EqualFold(strings.TrimSpace(speaker), "shiro") {
		style += " Shiroとして落ち着いた常体寄りで、整理だけで終えず小さな未決点を残す。"
	}
	content := fmt.Sprintf("%sとして、話題「%s」について会話本文を作ってください。\n直前の相手発言: %s\n%s", speaker, topic, other, style)
	if strings.TrimSpace(purpose) != "" {
		content += "\n狙い: " + strings.TrimSpace(purpose)
	}
	return []llm.Message{
		{Role: "system", Content: "/no_think\n最終回答の本文だけを自然な日本語で返す。英語だけの応答、英語の見出し、英語での説明は禁止。"},
		{Role: "user", Content: content},
	}
}

func idleFunScorePercent(response, latestOther, latestSelf, topic string) int {
	s := strings.TrimSpace(response)
	if s == "" {
		return 0
	}
	score := 45
	runeLen := utf8.RuneCountInString(s)
	if runeLen >= 28 && runeLen <= 120 {
		score += 10
	} else if runeLen > 160 {
		score -= 15
	}
	if strings.ContainsAny(s, "？?") {
		score += 8
	}
	if containsAny(s, "秘密", "隠", "嘘", "鍵", "手紙", "封筒", "雨", "机", "駅", "階段", "選", "損", "怖", "失敗", "反転", "開ける", "落ちた") {
		score += 18
	}
	if containsAny(s, "誰", "なぜ", "どうして", "どちら", "選ぶ", "開ける", "守る", "困る") {
		score += 10
	}
	if containsAny(s, "面白いですね", "有効ですね", "構造", "整理", "検証", "可能性", "観点", "要素") {
		score -= 16
	}
	if latestOther != "" && textSimilarity(s, latestOther) >= 0.45 {
		score -= 12
	}
	if latestSelf != "" && textSimilarity(s, latestSelf) >= 0.45 {
		score -= 12
	}
	if topic != "" {
		for _, part := range strings.FieldsFunc(topic, func(r rune) bool {
			return r == ' ' || r == '　' || r == 'と' || r == '、' || r == '。' || r == ',' || r == '/'
		}) {
			part = strings.TrimSpace(part)
			if utf8.RuneCountInString(part) >= 2 && strings.Contains(s, part) {
				score += 4
				break
			}
		}
	}
	if englishDominantIdleText(s) || hasInternalReasoningLeak(s) || hasPromptLeak(s) {
		score -= 40
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return score
}

func unusableIdleResponse(raw, sanitized string) bool {
	return invalidIdleResponse(sanitized) ||
		((hasPromptLeak(raw) || hasInternalReasoningLeak(raw)) && !hasIdleSentenceEnd(sanitized)) ||
		hasPromptLeak(sanitized) ||
		hasInternalReasoningLeak(sanitized)
}

func finishReasonLooksTruncated(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "length", "max_tokens", "max_output_tokens":
		return true
	default:
		return false
	}
}

func extractIdleTopicText(content string) string {
	s := strings.TrimSpace(content)
	if s == "" {
		return ""
	}
	prefixes := []string{"今日のお題", "お題"}
	for _, prefix := range prefixes {
		if !strings.HasPrefix(s, prefix) {
			continue
		}
		if idx := strings.IndexAny(s, ":：、,"); idx >= 0 && idx+1 < len(s) {
			return strings.TrimSpace(s[idx+1:])
		}
	}
	return ""
}

func (o *IdleChatOrchestrator) generateIdleLLM(provider llm.LLMProvider, req llm.GenerateRequest) (llm.GenerateResponse, error) {
	if provider == nil {
		return llm.GenerateResponse{}, fmt.Errorf("idlechat LLM provider is nil")
	}
	timeout := idleChatLLMGenerateTimeout
	role := "none"
	if len(req.Messages) > 0 {
		role = strings.TrimSpace(req.Messages[len(req.Messages)-1].Role)
		if role == "" {
			role = "unknown"
		}
	}
	if timeout <= 0 {
		resp, err := provider.Generate(o.ctx, req)
		if err == nil {
			logIdleRaw(fmt.Sprintf("llm.generate role=%s", role), resp.Content)
			log.Printf("[IdleChat][llm] role=%s max_tokens=%d finish=%q tokens=%d", role, req.MaxTokens, resp.FinishReason, resp.TokensUsed)
		}
		return resp, err
	}
	ctx, cancel := context.WithTimeout(o.ctx, timeout)
	defer cancel()
	resp, err := provider.Generate(ctx, req)
	if err == nil {
		logIdleRaw(fmt.Sprintf("llm.generate role=%s", role), resp.Content)
		log.Printf("[IdleChat][llm] role=%s max_tokens=%d finish=%q tokens=%d", role, req.MaxTokens, resp.FinishReason, resp.TokensUsed)
	}
	return resp, err
}

func fallbackIdleResponse(speaker, topic, latestOther string, turn int) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "この話題"
	}
	topicShort := truncate(topic, 42)
	other := truncate(strings.TrimSpace(latestOther), 34)
	isShiro := strings.EqualFold(strings.TrimSpace(speaker), "shiro")
	variants := turn % 8
	if isShiro {
		switch variants {
		case 0:
			return fmt.Sprintf("そのお題なら、まず%sを一つの場面に絞ると話が進みます。棚や通路みたいな具体物を置くと、見え方が安定します。", topicShort)
		case 1:
			if other != "" {
				return fmt.Sprintf("今の「%s」は入口として使えますね。次は誰が何を見落としたのか、一点だけ決めると輪郭が出ます。", other)
			}
			return fmt.Sprintf("%sは抽象のままだと散るので、最初の発見を一つ決めたいです。小さな違和感から始めるのがよさそうです。", topicShort)
		case 2:
			return fmt.Sprintf("%sでは、人物の動きより先に場所のルールを決めると整理できます。何が普通で、何が一度だけズレたのかを見たいですね。", topicShort)
		case 3:
			return fmt.Sprintf("ここは結論を急がず、%sを触れる物に落としましょう。音、匂い、置き場所のどれか一つが次の手がかりになります。", topicShort)
		case 4:
			return fmt.Sprintf("%sを追うなら、誰か一人の習慣を決めるのがよさそうです。同じ動きが一度だけ崩れると、会話の焦点になります。", topicShort)
		case 5:
			return fmt.Sprintf("視点を少し狭めると、%sは観察記録として扱えます。最初に残る痕跡を一つ選ぶと、次の問いが自然に出ます。", topicShort)
		case 6:
			return fmt.Sprintf("その方向なら、場所の明るさや足音の変化を使えます。%sを説明ではなく、誰かが気づく瞬間に寄せたいですね。", topicShort)
		default:
			return fmt.Sprintf("いま必要なのは、%sの中で変化する一点を決めることです。人、物、時間のどれが先にズレるかで展開が変わります。", topicShort)
		}
	}
	switch variants {
	case 0:
		return fmt.Sprintf("えー、%sって、最初に小さな違和感を一つ置くと一気に見えそうだね。たとえば誰かがいつもと違う場所で立ち止まる場面から始めたいな。", topicShort)
	case 1:
		if other != "" {
			return fmt.Sprintf("その「%s」って手がかり、けっこう効きそう。じゃあ次は、それを最初に見つける人の表情から決めてみない？", other)
		}
		return fmt.Sprintf("いいじゃん、%sなら人の癖が見える瞬間から入りたいな。何気ない動きが、あとで意味を持つ感じにしたい。", topicShort)
	case 2:
		return fmt.Sprintf("%s、ただ説明するより一場面で見せたいね。古い照明が一瞬だけ揺れる、みたいな合図があると話が動きそう。", topicShort)
	case 3:
		return fmt.Sprintf("気になるのは、%sの中で誰が最初に違和感へ気づくかだね。そこを決めたら、会話も自然に前へ進みそう。", topicShort)
	case 4:
		return fmt.Sprintf("いいね、%sなら最初の手がかりをすごく小さくしたいな。落ちている紙片とか、匂いが一瞬変わるとか、そのくらいが効きそう。", topicShort)
	case 5:
		return fmt.Sprintf("%sって、誰かのいつもの癖がズレるだけで話になりそう。そこから『今日は何か違う』って空気を作りたい。", topicShort)
	case 6:
		return fmt.Sprintf("それなら、%sを一人の目線で追うのがよさそうだね。見慣れた場所の一箇所だけが変わっていて、そこから会話が動く感じ。", topicShort)
	default:
		return fmt.Sprintf("じゃあ%sは、最後に大きな説明を置くより、最初に触れる物を決めたいな。その物が誰の記憶につながるかで広げられそう。", topicShort)
	}
}

func (o *IdleChatOrchestrator) resolveDialogueTopic(sessionID, speaker, topic string) string {
	if normalized := strings.TrimSpace(topic); normalized != "" {
		return normalized
	}
	if sessionID != "" {
		for _, entry := range o.memory.GetUnifiedView(24) {
			if entry.Message.SessionID != sessionID {
				continue
			}
			if extracted := extractIdleTopicText(entry.Message.Content); extracted != "" {
				log.Printf("[IdleChat] Empty dialogue topic recovered from session memory: session=%s topic=%q", sessionID, truncate(extracted, 80))
				return extracted
			}
		}
	}
	o.mu.Lock()
	currentTopic := strings.TrimSpace(o.currentTopic)
	o.mu.Unlock()
	if currentTopic != "" && !strings.Contains(currentTopic, "準備中") {
		log.Printf("[IdleChat] Empty dialogue topic recovered from current topic: session=%s topic=%q", sessionID, truncate(currentTopic, 80))
		return currentTopic
	}
	log.Printf("[IdleChat] Empty dialogue topic; using emergency fallback: session=%s speaker=%s", sessionID, speaker)
	return "この会話の現在のお題"
}

func (o *IdleChatOrchestrator) temperatureForSpeaker(speaker string) float64 {
	switch strings.ToLower(strings.TrimSpace(speaker)) {
	case "mio", "shiro":
		return 0.65
	default:
		return o.temperature
	}
}
