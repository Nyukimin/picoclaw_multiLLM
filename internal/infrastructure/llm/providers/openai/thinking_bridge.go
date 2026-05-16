package openai

import (
	"strings"
)

func (p *OpenAIProvider) addThinkingBridgeFields(req map[string]interface{}, streaming bool) {
	if !p.thinkingBridge {
		return
	}
	req["parse_reasoning"] = true
	req["include_reasoning"] = false
	req["separate_reasoning"] = true
	if streaming {
		req["stream"] = true
	}
}

func (p *OpenAIProvider) sanitizeThinkingBridgeContent(content, parseStatus, _ string) string {
	if !p.thinkingBridge {
		return content
	}
	if strings.TrimSpace(parseStatus) != "no_reasoning" {
		return content
	}
	if !looksLikeUntaggedReasoning(content) {
		return content
	}
	if final := extractFinalAnswerFromUntaggedReasoning(content); final != "" {
		return final
	}
	return ""
}

func looksLikeUntaggedReasoning(s string) bool {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	startsLikeReasoning := strings.HasPrefix(lower, "okay,") ||
		strings.HasPrefix(lower, "ok,") ||
		strings.HasPrefix(lower, "let me ") ||
		strings.HasPrefix(lower, "we need ") ||
		strings.HasPrefix(lower, "i need ") ||
		strings.HasPrefix(lower, "i should ") ||
		strings.HasPrefix(lower, "the user ")
	if !startsLikeReasoning {
		return false
	}
	markers := []string{
		"the user",
		"they wrote",
		"the query",
		"let me",
		"i need to",
		"i should",
		"translates to",
		"asking for",
		"want me to",
		"need to respond",
		"final answer",
	}
	hits := 0
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			hits++
		}
	}
	return hits >= 2
}

func extractFinalAnswerFromUntaggedReasoning(s string) string {
	candidates := []string{
		"Final answer:",
		"Final Answer:",
		"final answer:",
		"最終回答:",
		"最終回答：",
		"回答:",
		"回答：",
	}
	for _, marker := range candidates {
		if idx := strings.LastIndex(s, marker); idx >= 0 {
			return strings.TrimSpace(s[idx+len(marker):])
		}
	}
	return ""
}
