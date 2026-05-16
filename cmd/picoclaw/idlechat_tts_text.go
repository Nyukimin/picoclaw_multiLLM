package main

import (
	"regexp"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/application/idlechat"
)

var idleChatTopicPrefixRe = regexp.MustCompile(`^今日のお題（[^）]+）:\s*`)

func formatIdleChatTTSText(ev idlechat.TimelineEvent) string {
	content := strings.TrimSpace(ev.Content)
	if strings.EqualFold(ev.From, "user") && strings.EqualFold(ev.To, "mio") && idleChatTopicPrefixRe.MatchString(content) {
		topic := strings.TrimSpace(idleChatTopicPrefixRe.ReplaceAllString(content, ""))
		if topic == "" {
			return "きょうのおだい。"
		}
		return "きょうのおだい、" + ensureIdleChatSentencePause(topic)
	}
	return ensureIdleChatSentencePause(stripIdleChatSpeechNotes(content))
}

func formatIdleChatDisplayText(ev idlechat.TimelineEvent) string {
	content := strings.TrimSpace(ev.Content)
	if strings.EqualFold(ev.From, "user") && strings.EqualFold(ev.To, "mio") && idleChatTopicPrefixRe.MatchString(content) {
		topic := strings.TrimSpace(idleChatTopicPrefixRe.ReplaceAllString(content, ""))
		if topic == "" {
			return "今日のお題："
		}
		return "今日のお題：" + topic
	}
	return ensureIdleChatSentencePause(content)
}

func ensureIdleChatSentencePause(content string) string {
	if content == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(content, "。"),
		strings.HasSuffix(content, "！"),
		strings.HasSuffix(content, "？"),
		strings.HasSuffix(content, "."),
		strings.HasSuffix(content, "!"),
		strings.HasSuffix(content, "?"):
		return content
	default:
		return content + "。"
	}
}

func stripIdleChatSpeechNotes(content string) string {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(kept) > 0 && kept[len(kept)-1] != "" {
				kept = append(kept, "")
			}
			continue
		}
		if strings.HasPrefix(line, "注記:") || strings.HasPrefix(line, "注記：") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func isIdleChatTopicAnnouncement(ev idlechat.TimelineEvent) bool {
	content := strings.TrimSpace(ev.Content)
	return strings.EqualFold(ev.From, "user") &&
		strings.EqualFold(ev.To, "mio") &&
		idleChatTopicPrefixRe.MatchString(content)
}
