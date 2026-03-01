package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/pkg/providers"
)

type Classification struct {
	Route      string   `json:"route"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
	Evidence   []string `json:"evidence"`
}

type Classifier struct {
	provider providers.LLMProvider
	model    string
}

func NewClassifier(provider providers.LLMProvider, model string) *Classifier {
	return &Classifier{
		provider: provider,
		model:    model,
	}
}

func (c *Classifier) Classify(ctx context.Context, userText string) (Classification, bool) {
	if c == nil || c.provider == nil {
		return Classification{}, false
	}

	systemPrompt := "You are a strict router classifier. Return JSON only with keys: route, confidence, reason, evidence. " +
		"route must be one of CHAT, PLAN, ANALYZE, OPS, RESEARCH, CODE. confidence must be 0..1."
	userPrompt := "Classify this message:\n" + userText

	resp, err := c.provider.Chat(ctx, []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}, nil, c.model, map[string]interface{}{
		"temperature": 0.0,
		"max_tokens":  300,
	})
	if err != nil || resp == nil {
		return Classification{}, false
	}

	raw := extractFirstJSONText(resp.Content)
	if raw == "" {
		return Classification{}, false
	}

	var out Classification
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return Classification{}, false
	}

	out.Route = strings.ToUpper(strings.TrimSpace(out.Route))
	if !isAllowedRoute(out.Route) {
		return Classification{}, false
	}
	if out.Confidence < 0 || out.Confidence > 1 {
		return Classification{}, false
	}
	return out, true
}

func extractFirstJSONText(text string) string {
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : i+1])
			}
		}
	}
	return ""
}
