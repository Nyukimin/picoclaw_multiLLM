package idlechat

import (
	"strings"
	"unicode"
)

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
