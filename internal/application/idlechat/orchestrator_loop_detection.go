package idlechat

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/session"
)

func (o *IdleChatOrchestrator) getRecentTopics(limit int) []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if limit <= 0 || limit > len(o.history) {
		limit = len(o.history)
	}
	out := make([]string, 0, limit)
	for i := len(o.history) - 1; i >= 0 && len(out) < limit; i-- {
		t := strings.TrimSpace(o.history[i].Topic)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func hasAlternatingLoop(transcript []string) bool {
	if len(transcript) < 8 {
		return false
	}
	a := normalizeLoopText(transcript[len(transcript)-1])
	b := normalizeLoopText(transcript[len(transcript)-2])
	if a == "" || b == "" {
		return false
	}
	matches := 0
	for i := len(transcript) - 3; i >= 0 && i >= len(transcript)-7; i -= 2 {
		if textSimilarity(a, normalizeLoopText(transcript[i])) >= 0.9 {
			matches++
		}
	}
	for i := len(transcript) - 4; i >= 0 && i >= len(transcript)-8; i -= 2 {
		if textSimilarity(b, normalizeLoopText(transcript[i])) >= 0.9 {
			matches++
		}
	}
	return matches >= 3
}

func hasShortAlternatingLoop(transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	a := normalizeLoopText(transcript[len(transcript)-1])
	b := normalizeLoopText(transcript[len(transcript)-2])
	c := normalizeLoopText(transcript[len(transcript)-3])
	d := normalizeLoopText(transcript[len(transcript)-4])
	if a == "" || b == "" || c == "" || d == "" {
		return false
	}
	return textSimilarity(a, c) >= 0.9 && textSimilarity(b, d) >= 0.9
}

func hasHighSimilarityLoop(transcript []string) bool {
	if len(transcript) < 10 {
		return false
	}
	start := len(transcript) - 10
	base := make([]string, 0, 10)
	for i := start; i < len(transcript); i++ {
		t := normalizeLoopText(transcript[i])
		if t != "" {
			base = append(base, t)
		}
	}
	if len(base) < 6 {
		return false
	}
	similarPairs := 0
	totalPairs := 0
	for i := 0; i < len(base); i++ {
		for j := i + 1; j < len(base); j++ {
			totalPairs++
			if textSimilarity(base[i], base[j]) >= 0.92 {
				similarPairs++
			}
		}
	}
	return totalPairs > 0 && similarPairs*3 >= totalPairs
}

func hasShortHighSimilarityLoop(transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	start := len(transcript) - 4
	base := make([]string, 0, 4)
	for i := start; i < len(transcript); i++ {
		t := normalizeLoopText(transcript[i])
		if t != "" {
			base = append(base, t)
		}
	}
	if len(base) < 4 {
		return false
	}
	similarPairs := 0
	for i := 0; i < len(base); i++ {
		for j := i + 1; j < len(base); j++ {
			if textSimilarity(base[i], base[j]) >= 0.94 {
				similarPairs++
			}
		}
	}
	return similarPairs >= 3
}

func hasSpeakerTemplateLoop(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	type speakerTurn struct {
		speaker string
		text    string
	}
	turns := make([]speakerTurn, 0, 10)
	start := len(transcript) - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < len(transcript); i++ {
		speaker, text := splitTranscriptSpeaker(transcript[i])
		if speaker == "" || text == "" {
			continue
		}
		turns = append(turns, speakerTurn{speaker: speaker, text: text})
	}
	if len(turns) < 6 {
		return false
	}

	perSpeaker := map[string][]string{}
	for _, turn := range turns {
		key := transcriptLeadPattern(turn.text)
		if key == "" {
			continue
		}
		perSpeaker[turn.speaker] = append(perSpeaker[turn.speaker], key)
	}
	for _, keys := range perSpeaker {
		if repeatedLeadPattern(keys) {
			return true
		}
	}

	for speaker := range perSpeaker {
		msgs := make([]string, 0, 4)
		for i := len(turns) - 1; i >= 0 && len(msgs) < 4; i-- {
			if turns[i].speaker == speaker {
				msgs = append(msgs, normalizeLoopText(turns[i].text))
			}
		}
		if len(msgs) < 3 {
			continue
		}
		similarPairs := 0
		for i := 0; i < len(msgs); i++ {
			for j := i + 1; j < len(msgs); j++ {
				if textSimilarity(msgs[i], msgs[j]) >= 0.82 {
					similarPairs++
				}
			}
		}
		if similarPairs >= 2 {
			return true
		}
	}
	return false
}

func hasShortSpeakerTemplateLoop(transcript []string) bool {
	if len(transcript) < 6 {
		return false
	}
	type speakerTurn struct {
		speaker string
		text    string
	}
	// 直近6ターンを検査。同一話者3ターン連続一致で発火。
	// 2ターン一致（4ターン窓）は深い議論での誤発火が多いため閾値を上げる。
	turns := make([]speakerTurn, 0, 6)
	for i := len(transcript) - 6; i < len(transcript); i++ {
		speaker, text := splitTranscriptSpeaker(transcript[i])
		if speaker == "" || text == "" {
			continue
		}
		turns = append(turns, speakerTurn{speaker: speaker, text: text})
	}
	if len(turns) < 6 {
		return false
	}
	perSpeaker := map[string][]string{}
	for _, turn := range turns {
		key := transcriptLeadPattern(turn.text)
		if key == "" {
			continue
		}
		perSpeaker[turn.speaker] = append(perSpeaker[turn.speaker], key)
	}
	for _, keys := range perSpeaker {
		// 同一話者3ターン分が揃い、かつ最後の3ターンすべて同一パターン
		if len(keys) >= 3 && keys[len(keys)-1] == keys[len(keys)-2] && keys[len(keys)-2] == keys[len(keys)-3] {
			return true
		}
	}
	return false
}

func splitTranscriptSpeaker(line string) (speaker, text string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", strings.TrimSpace(line)
	}
	speaker = strings.ToLower(strings.TrimSpace(line[:idx]))
	text = strings.TrimSpace(line[idx+1:])
	return speaker, text
}

func transcriptLeadPattern(text string) string {
	s := strings.TrimSpace(strings.ToLower(text))
	s = strings.TrimLeftFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	s = strings.TrimPrefix(s, "[mio]")
	s = strings.TrimPrefix(s, "[shiro]")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	count := 0
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			break
		}
		b.WriteRune(r)
		count++
		if count >= 8 {
			break
		}
	}
	// 5文字未満は「確かに」「なるほど」等の短い同意接頭辞。
	// 構造的テンプレートとはみなさず、誤検知を防ぐ。
	if b.Len() < 5 {
		return ""
	}
	return b.String()
}

func repeatedLeadPattern(keys []string) bool {
	if len(keys) < 3 {
		return false
	}
	counts := map[string]int{}
	for _, key := range keys {
		if key == "" {
			continue
		}
		counts[key]++
		if counts[key] >= 3 {
			return true
		}
	}
	return false
}

func topicTooSimilar(topic string, recent []string) bool {
	n := normalizeLoopText(topic)
	if n == "" {
		return true
	}
	for _, prev := range recent {
		if textSimilarity(n, normalizeLoopText(prev)) >= 0.9 {
			return true
		}
	}
	return false
}

func isResponseTooSimilar(response string, transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	cur := normalizeLoopText(response)
	if cur == "" {
		return false
	}
	start := len(transcript) - 6
	if start < 0 {
		start = 0
	}
	hits := 0
	for i := start; i < len(transcript); i++ {
		prev := normalizeLoopText(transcript[i])
		if prev == "" {
			continue
		}
		if textSimilarity(cur, prev) >= 0.93 {
			hits++
		}
	}
	return hits >= 2
}

func normalizeLoopText(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if idx := strings.Index(s, ":"); idx >= 0 {
		s = strings.TrimSpace(s[idx+1:])
	}
	s = strings.TrimPrefix(s, "[mio]")
	s = strings.TrimPrefix(s, "[shiro]")
	s = strings.TrimPrefix(s, "[worker]")
	s = strings.TrimPrefix(s, "[chat]")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func textSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	ag := runeNGrams(a, 2)
	bg := runeNGrams(b, 2)
	if len(ag) == 0 || len(bg) == 0 {
		if a == b {
			return 1
		}
		return 0
	}
	inter := 0
	i, j := 0, 0
	for i < len(ag) && j < len(bg) {
		if ag[i] == bg[j] {
			inter++
			i++
			j++
			continue
		}
		if ag[i] < bg[j] {
			i++
		} else {
			j++
		}
	}
	return (2.0 * float64(inter)) / float64(len(ag)+len(bg))
}

func runeNGrams(s string, n int) []string {
	r := []rune(s)
	if len(r) < n || n <= 0 {
		return nil
	}
	out := make([]string, 0, len(r)-n+1)
	for i := 0; i <= len(r)-n; i++ {
		out = append(out, string(r[i:i+n]))
	}
	sort.Strings(out)
	return out
}

func splitSpeakerContexts(entries []session.ConversationEntry, sessionID, speaker string, limit int) ([]string, []string) {
	self := make([]string, 0, limit)
	other := make([]string, 0, limit)
	for i := len(entries) - 1; i >= 0 && (len(self) < limit || len(other) < limit); i-- {
		m := entries[i].Message
		if m.SessionID != sessionID {
			continue
		}
		text := truncate(strings.TrimSpace(m.Content), 80)
		if text == "" {
			continue
		}
		if strings.EqualFold(m.From, speaker) {
			if len(self) < limit {
				self = append(self, text)
			}
			continue
		}
		if len(other) < limit {
			other = append(other, fmt.Sprintf("%s: %s", m.From, text))
		}
	}
	if len(self) == 0 {
		self = append(self, "なし")
	}
	if len(other) == 0 {
		other = append(other, "なし")
	}
	return self, other
}

func latestOtherUtterance(entries []session.ConversationEntry, sessionID, speaker string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if m.SessionID != sessionID || strings.EqualFold(m.From, speaker) {
			continue
		}
		return strings.TrimSpace(m.Content)
	}
	return ""
}

func latestSelfUtterance(entries []session.ConversationEntry, sessionID, speaker string) string {
	for i := len(entries) - 1; i >= 0; i-- {
		m := entries[i].Message
		if m.SessionID != sessionID || !strings.EqualFold(m.From, speaker) {
			continue
		}
		return strings.TrimSpace(m.Content)
	}
	return ""
}

func violatesAttribution(response, latestOther string) bool {
	resp := normalizeLoopText(response)
	other := normalizeLoopText(latestOther)
	if resp == "" || other == "" {
		return false
	}
	if textSimilarity(resp, other) < 0.93 {
		return false
	}
	lower := strings.ToLower(response)
	if strings.Contains(lower, "あなた") || strings.Contains(lower, "君") || strings.Contains(lower, "相手") || strings.Contains(lower, "その視点") {
		return false
	}
	return true
}
