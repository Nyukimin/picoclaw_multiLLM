package orchestrator

import (
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/routing"
)

func normalizeChatRecipient(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mio", "chat":
		return "mio"
	case "shiro", "chatworker", "chat_worker", "worker-chat", "worker_chat":
		return "shiro"
	case "kuro", "heavy":
		return "kuro"
	case "midori", "wild":
		return "midori"
	default:
		return ""
	}
}

func chatRecipientRoute(recipient string) (routing.Route, bool) {
	switch normalizeChatRecipient(recipient) {
	case "mio":
		return routing.RouteCHAT, true
	case "shiro":
		return routing.RouteWORKERCHAT, true
	case "kuro":
		return routing.RouteANALYZE, true
	case "midori":
		return routing.RouteWILD, true
	default:
		return "", false
	}
}

func requestChatRecipient(req ProcessMessageRequest) string {
	return normalizeChatRecipient(req.Recipient)
}

func requestHasExplicitRouteCommand(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" || !strings.HasPrefix(trimmed, "/") {
		return false
	}
	head := strings.ToLower(strings.Fields(trimmed)[0])
	switch head {
	case "/ops", "/wild", "/heavy", "/chatworker", "/chat-worker", "/worker-chat", "/code", "/code1", "/code2", "/code3", "/code4", "/plan", "/analyze", "/research", "/chat":
		return true
	default:
		return false
	}
}

func chatRecipientDecision(req ProcessMessageRequest) (routing.Decision, string, bool) {
	recipient := requestChatRecipient(req)
	if recipient == "" || requestHasExplicitRouteCommand(req.UserMessage) {
		return routing.Decision{}, "", false
	}
	route, ok := chatRecipientRoute(recipient)
	if !ok {
		return routing.Decision{}, "", false
	}
	return routing.NewDecisionWithEvidence(route, 1.0, "viewer chat recipient",
		routing.DecisionEvidence{
			Source:     "viewer_to",
			Matched:    true,
			Route:      route,
			Confidence: 1.0,
			Reason:     "recipient=" + recipient,
		},
	), recipient, true
}
