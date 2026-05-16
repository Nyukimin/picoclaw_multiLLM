package idlechat

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

func hasPromptLeak(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	markers := []string{
		"<|",
		"|>",
		"channel>thought",
		"channel=analysis",
		"analysis to=",
		"assistant to=",
		"発言帰属ガード",
		"相手の発言として受ける",
		"相手の案を整理",
		"前に自分も触れた",
		"次に起きそうな場面",
		"直前の相手発言",
		"直前の自分",
		"1〜2文",
		"1-2文",
		"具体物・選択",
		"条件・制約",
		"要件:",
		"要件：",
		"（話題:",
		"現在の状況",
		"目標:",
		"目標：",
		"制約事項",
		"会話の制約",
		"システムプロンプト",
	}
	for _, m := range markers {
		if strings.Contains(lower, strings.ToLower(m)) {
			return true
		}
	}
	if strings.Contains(lower, "発言として受け") {
		return true
	}
	return false
}

func extractVisibleLLMAnswer(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	finalMarkers := []string{
		"<|channel|>final",
		"<|channel>final",
		"channel>final",
		"channel=final",
	}
	for _, marker := range finalMarkers {
		if idx := strings.LastIndex(lower, marker); idx >= 0 {
			return trimHarmonyTail(strings.TrimSpace(s[idx+len(marker):]))
		}
	}
	if strings.Contains(lower, "<|channel") || strings.Contains(lower, "channel>thought") || strings.Contains(lower, "channel=analysis") {
		return ""
	}
	if extracted := extractFinalAnswerBlock(s); extracted != "" {
		return trimHarmonyTail(extracted)
	}
	if hasInternalReasoningLeak(s) {
		if extracted := extractQuotedJapaneseDialogueCandidate(s); extracted != "" {
			return trimHarmonyTail(extracted)
		}
	}
	if extracted := extractTrailingJapaneseDialogueBlock(s); extracted != "" {
		return trimHarmonyTail(extracted)
	}
	return trimHarmonyTail(s)
}

func extractFinalAnswerBlock(s string) string {
	type marker struct {
		raw        string
		allowColon bool
	}
	markers := []marker{
		{raw: "final answer", allowColon: true},
		{raw: "final response", allowColon: true},
		{raw: "answer", allowColon: true},
		{raw: "最終回答", allowColon: true},
		{raw: "最終返答", allowColon: true},
		{raw: "回答", allowColon: true},
	}
	lower := strings.ToLower(s)
	for _, m := range markers {
		idx := strings.LastIndex(lower, strings.ToLower(m.raw))
		if idx < 0 {
			continue
		}
		start := idx + len(m.raw)
		tail := strings.TrimSpace(s[start:])
		if m.allowColon {
			tail = strings.TrimLeftFunc(tail, func(r rune) bool {
				return r == ':' || r == '：' || unicode.IsSpace(r)
			})
		}
		if looksLikeDialogueBody(tail) {
			return tail
		}
	}
	return ""
}

func extractTrailingJapaneseDialogueBlock(s string) string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return ""
	}
	blocks := strings.Split(trimmed, "\n\n")
	for i := len(blocks) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(blocks[i])
		if candidate == "" {
			continue
		}
		if looksLikeDialogueBody(candidate) {
			return candidate
		}
	}
	return ""
}

func extractQuotedJapaneseDialogueCandidate(s string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`「([^「」]{12,240})」`),
		regexp.MustCompile(`“([^“”]{12,240})”`),
	}
	for _, re := range patterns {
		matches := re.FindAllStringSubmatch(s, -1)
		for i := len(matches) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(matches[i][1])
			if looksLikeDialogueBody(candidate) {
				return candidate
			}
		}
	}
	return ""
}

func looksLikeDialogueBody(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if hasPromptLeak(s) || hasInternalReasoningLeak(s) {
		return false
	}
	if !hasIdleSentenceEnd(s) {
		return false
	}
	hasKana := false
	for _, r := range s {
		if unicode.In(r, unicode.Hiragana, unicode.Katakana) {
			hasKana = true
			break
		}
	}
	return hasKana
}

func hasIdleSentenceEnd(s string) bool {
	return strings.ContainsAny(strings.TrimSpace(s), "。！？!?")
}

func trimHarmonyTail(s string) string {
	s = strings.TrimSpace(s)
	lower := strings.ToLower(s)
	for _, marker := range []string{"<|end|>", "<|return|>", "<|message|>", "<|endoftext|>"} {
		if idx := strings.Index(lower, marker); idx >= 0 {
			s = strings.TrimSpace(s[:idx])
			lower = strings.ToLower(s)
		}
	}
	return s
}

func hasInternalReasoningLeak(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	markers := []string{
		"okay, let's",
		"okay, so",
		"ok, let's",
		"alright,",
		"let me",
		"the user is asking",
		"the user wants me",
		"the task is",
		"i need to",
		"i should",
		"ユーザーは私",
		"私はmioとして",
		"私はshiroとして",
		"mioとして、",
		"shiroとして、",
		"必要がある",
		"遵守する必要",
		"以下の点",
		"会話の制約",
		"キャラクター（",
		"**現在の状況**",
		"**目標**",
		"1. **",
		"2. **",
		"好的",
		"我现在需要",
		"用户",
		"规则",
		"检查",
		"首先",
		"比如",
		"或者",
		"因为",
		"所以",
	}
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 3 {
		bullets := 0
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || regexp.MustCompile(`^\d+[.)．]\s*`).MatchString(line) {
				bullets++
			}
		}
		if bullets >= 2 {
			return true
		}
	}
	return false
}

func englishDominantIdleText(s string) bool {
	totalLetters := 0
	asciiLetters := 0
	japaneseLetters := 0
	for _, r := range s {
		if !unicode.IsLetter(r) {
			continue
		}
		totalLetters++
		if r <= unicode.MaxASCII {
			asciiLetters++
			continue
		}
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			japaneseLetters++
		}
	}
	if totalLetters < 12 {
		return false
	}
	return japaneseLetters == 0 && asciiLetters*100/totalLetters >= 80
}

func sanitizeIdleResponse(s, topic string) string {
	out := strings.TrimSpace(extractVisibleLLMAnswer(s))
	if out == "" {
		return out
	}
	for _, marker := range []string{"<|channel", "channel>thought", "channel=analysis"} {
		if idx := strings.Index(strings.ToLower(out), strings.ToLower(marker)); idx >= 0 {
			out = strings.TrimSpace(out[:idx])
			break
		}
	}
	if strings.HasPrefix(out, "（話題:") {
		if idx := strings.Index(out, "）"); idx >= 0 && idx+len("）") < len(out) {
			out = strings.TrimSpace(out[idx+len("）"):])
		}
	}
	leaks := []string{
		"相手の発言として受ける",
		"相手の発言として受け、",
		"前に自分も触れた発言への応答として、",
		"前に自分も触れたように、",
		"要件:",
		"要件：",
	}
	for _, leak := range leaks {
		out = strings.ReplaceAll(out, leak, "")
	}
	speakerPrefixes := []string{
		// "Assistant: [speaker]:" 形式（LLMのプロンプトリーク）
		"assistant: [mio]:",
		"assistant: [mio]：",
		"assistant: [shiro]:",
		"assistant: [shiro]：",
		"assistant: mio:",
		"assistant: mio：",
		"assistant: shiro:",
		"assistant: shiro：",
		"assistant:",
		// 通常の speaker prefix
		"[mio]:",
		"[mio]：",
		"[shiro]:",
		"[shiro]：",
		"mio]:",
		"mio]：",
		"shiro]:",
		"shiro]：",
		"mio:",
		"mio：",
		"shiro:",
		"shiro：",
		"mioさん:",
		"mio さん:",
		"shiroさん:",
		"shiro さん:",
	}
	for {
		lowerOut := strings.ToLower(out)
		stripped := false
		for _, prefix := range speakerPrefixes {
			if strings.HasPrefix(lowerOut, prefix) {
				out = strings.TrimSpace(out[len(prefix):])
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	out = promptLeakLineRe.ReplaceAllString(out, "")
	out = strings.TrimLeftFunc(out, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	out = strings.ReplaceAll(out, "  ", " ")
	out = strings.TrimSpace(out)
	return out
}

func sanitizeIdleSummaryResponse(raw, topic string) string {
	out := strings.TrimSpace(extractVisibleLLMAnswer(raw))
	if out == "" {
		return ""
	}
	out = dropLeadingReasoningParagraphs(out)
	if hasPromptLeak(out) || hasInternalReasoningLeak(out) {
		// 同一抽出器で再抽出（Final answer / 末尾日本語ブロック）
		out = strings.TrimSpace(extractVisibleLLMAnswer(out))
		out = dropLeadingReasoningParagraphs(out)
	}
	out = strings.TrimSpace(out)
	if out == "" || hasPromptLeak(out) || hasInternalReasoningLeak(out) || summaryLooksLikeEnglishMetaReasoning(out) {
		return ""
	}
	return out
}

func dropLeadingReasoningParagraphs(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "\n\n")
	if len(parts) <= 1 {
		return strings.TrimSpace(s)
	}
	start := 0
	for start < len(parts) {
		p := strings.TrimSpace(parts[start])
		if p == "" || hasPromptLeak(p) || hasInternalReasoningLeak(p) || summaryLooksLikeEnglishReasoningLead(p) {
			start++
			continue
		}
		break
	}
	if start >= len(parts) {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.Join(parts[start:], "\n\n"))
}

func summaryLooksLikeEnglishReasoningLead(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	leadings := []string{
		"okay,",
		"ok,",
		"alright,",
		"first,",
		"the user wants me",
		"the user asks me",
		"i need to",
		"let me",
	}
	for _, p := range leadings {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func summaryLooksLikeEnglishMetaReasoning(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	markers := []string{
		"the user provided",
		"the user wants me",
		"the output format",
		"first, i need",
		"looking at the",
		"let me break it down",
		"wait, the",
	}
	metaHits := 0
	for _, m := range markers {
		if strings.Contains(lower, m) {
			metaHits++
		}
	}
	// メタ推論キーワード一致のみで判定する
	if metaHits >= 2 {
		return true
	}
	return false
}

func invalidIdleResponse(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return true
	}
	if containsUnexpectedIdleScript(trimmed) {
		return true
	}
	hasText := false
	for _, r := range trimmed {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			hasText = true
			break
		}
	}
	if !hasText {
		return true
	}
	if utf8.RuneCountInString(trimmed) < 12 && !hasIdleSentenceEnd(trimmed) {
		return true
	}
	first, _ := utf8.DecodeRuneInString(trimmed)
	if unicode.IsPunct(first) || unicode.IsSymbol(first) {
		return true
	}
	lower := strings.ToLower(trimmed)
	if lower == "。" || lower == "、" || lower == "!" || lower == "！" || lower == "?" || lower == "？" {
		return true
	}
	return false
}

func containsUnexpectedIdleScript(s string) bool {
	for _, r := range s {
		switch {
		case unicode.In(r, unicode.Devanagari, unicode.Hangul, unicode.Arabic, unicode.Hebrew, unicode.Thai, unicode.Cyrillic):
			return true
		}
	}
	return false
}

func hasAwkwardIdleStyle(speaker, s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	banned := []string{
		"前に自分も触れた",
		"相手の発言として受ける",
		"まさにその通りですね",
		"ご発言",
	}
	for _, phrase := range banned {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(speaker), "shiro") {
		shiroBanned := []string{
			"mioさん",
			"mio さん",
			"非常に興味深いですね",
			"非常に的確",
			"硬すぎました",
			"言い直すと",
			"少し硬すぎました",
		}
		for _, phrase := range shiroBanned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(speaker), "mio") {
		mioBanned := []string{
			"ご懸念はもっともかと存じます",
			"非常に興味深いですね",
			"その光",
		}
		for _, phrase := range mioBanned {
			if strings.Contains(lower, strings.ToLower(phrase)) {
				return true
			}
		}
	}
	return false
}

func needsIdleStyleRetry(speaker, response, latestOther, latestSelf, topic string) bool {
	return hasAwkwardIdleStyle(speaker, response) ||
		hasExcessivePhraseRepetition(response) ||
		mirrorsLatestOther(response, latestOther, topic) ||
		repeatsLatestSelf(response, latestSelf)
}

func mirrorsLatestOther(response, latestOther, topic string) bool {
	resp := strings.TrimSpace(response)
	other := strings.TrimSpace(latestOther)
	if resp == "" || other == "" {
		return false
	}
	common := longestCommonSubstring(resp, other)
	if utf8.RuneCountInString(common) < 6 {
		return false
	}
	if strings.TrimSpace(topic) != "" && strings.Contains(strings.TrimSpace(topic), common) {
		return false
	}
	return true
}

func repeatsLatestSelf(response, latestSelf string) bool {
	resp := strings.TrimSpace(response)
	self := strings.TrimSpace(latestSelf)
	if resp == "" || self == "" {
		return false
	}
	common := longestCommonSubstring(resp, self)
	return utf8.RuneCountInString(common) >= 10
}

func longestCommonSubstring(a, b string) string {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return ""
	}
	dp := make([]int, len(br)+1)
	bestLen := 0
	bestEnd := 0
	for i := 1; i <= len(ar); i++ {
		prev := 0
		for j := 1; j <= len(br); j++ {
			tmp := dp[j]
			if ar[i-1] == br[j-1] {
				dp[j] = prev + 1
				if dp[j] > bestLen {
					bestLen = dp[j]
					bestEnd = i
				}
			} else {
				dp[j] = 0
			}
			prev = tmp
		}
	}
	if bestLen == 0 {
		return ""
	}
	return string(ar[bestEnd-bestLen : bestEnd])
}

func hasExcessivePhraseRepetition(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return false
	}
	tokens := splitIdleTokens(normalized)
	if len(tokens) < 4 {
		return false
	}
	counts := map[string]int{}
	for _, token := range tokens {
		if len([]rune(token)) <= 1 {
			continue
		}
		counts[token]++
		if counts[token] >= 3 {
			return true
		}
	}
	for size := 2; size <= 4; size++ {
		if len(tokens) < size*2 {
			continue
		}
		ngrams := map[string]int{}
		for i := 0; i+size <= len(tokens); i++ {
			key := strings.Join(tokens[i:i+size], " ")
			ngrams[key]++
			if ngrams[key] >= 2 {
				return true
			}
		}
	}
	return false
}

func splitIdleTokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
}
