package openai

import modulellm "github.com/Nyukimin/picoclaw_multiLLM/modules/llm"

func (p *OpenAIProvider) addThinkingBridgeFields(req map[string]interface{}, streaming bool) {
	modulellm.ApplyThinkingBridgeFields(req, p.thinkingBridge, streaming)
}

func (p *OpenAIProvider) addProviderOptions(req map[string]interface{}, options map[string]any) {
	modulellm.ApplyThinkingBridgeProviderOptions(req, p.thinkingBridge, options)
}

func (p *OpenAIProvider) sanitizeThinkingBridgeContent(content, parseStatus, _ string) string {
	return modulellm.SanitizeThinkingBridgeContent(p.thinkingBridge, content, parseStatus)
}

func looksLikeUntaggedReasoning(s string) bool {
	return modulellm.LooksLikeUntaggedReasoning(s)
}

func extractFinalAnswerFromUntaggedReasoning(s string) string {
	return modulellm.ExtractFinalAnswerFromUntaggedReasoning(s)
}
