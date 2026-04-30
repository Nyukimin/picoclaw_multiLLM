package idlechat

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

const maxPromptGuides = 5

var promptFixLineRe = regexp.MustCompile(`(?i)^\s*(?:[-*・]\s*)?(?:PROMPT_FIX|prompt_fix|プロンプト補正|プロンプト修正)\s*[:：]\s*(.+)$`)

func (o *IdleChatOrchestrator) reviewSessionEnd(topic, mode string, transcript []string, summary, loopReason string) (string, string) {
	body := strings.TrimSpace(strings.Join(transcript, "\n"))
	if body == "" {
		return "", ""
	}

	fallbackReview, fallbackGuide := heuristicQualityReview(topic, mode, transcript, summary, loopReason)
	messages := []llm.Message{
		{Role: "system", Content: "あなたはIdleChatの聞き手体験を評価し、次回プロンプトを改善する編集者です。甘い褒め評価ではなく、退屈・冗長・反復・聞き手の置いてけぼりを検出してください。"},
		{Role: "user", Content: fmt.Sprintf(`次のIdleChat終了ログを評価してください。

観点:
- その話は聞き手にとって面白かったか
- 冗長ではなかったか
- 同じ型、同じ仮定表現、同じ結論を反復していないか
- 聞き手の楽しみや理解を損なっていないか
- 注記や打ち切り理由がある場合は、必ず原因を推定して再発防止プロンプトを出す

出力形式:
QUALITY: pass または fail
ISSUES:
- 問題点を1〜3個
PROMPT_FIX: 次回以降のsystem promptに追加すべき具体的な一文

モード: %s
話題: %s
打ち切り理由: %s
要約:
%s

会話ログ:
%s`, mode, topic, strings.TrimSpace(loopReason), strings.TrimSpace(summary), body)},
	}

	resp, err := o.providerForSpeaker("shiro").Generate(o.ctx, llm.GenerateRequest{
		Messages:    messages,
		MaxTokens:   500,
		Temperature: 0.2,
	})
	if err != nil || strings.TrimSpace(resp.Content) == "" {
		if err != nil {
			log.Printf("[IdleChat] quality review failed: %v", err)
		}
		return fallbackReview, fallbackGuide
	}

	review := strings.TrimSpace(resp.Content)
	guide := extractPromptGuidance(review)
	if guide == "" {
		guide = fallbackGuide
	}
	if fallbackGuide != "" && !strings.Contains(guide, fallbackGuide) {
		guide = joinPromptGuides(guide, fallbackGuide)
	}
	return review, guide
}

func heuristicQualityReview(topic, mode string, transcript []string, summary, loopReason string) (string, string) {
	var issues []string
	var fixes []string
	if note := loopReasonLabel(loopReason); note != "" {
		issues = append(issues, "打ち切り注記: "+note)
		switch loopReason {
		case "short_template_repeat", "template_repeat":
			fixes = append(fixes, "直前3発話と同じ書き出し・同じ受け方・同じ結論を使わず、返答ごとに場面・反論・具体例のどれか一つで入口を変える。")
		case "what_if_repeat":
			fixes = append(fixes, "「もし」「だったら」「なら」で仮定を重ね続けず、次の発話では必ず現実の場面、行動、結果のどれかに着地する。")
		case "short_high_similarity", "high_similarity", "exact_repeat", "pre_emit_similarity", "alternating_repeat", "short_alternating_repeat":
			fixes = append(fixes, "相手の語句を言い換えるだけで返さず、新しい情報・立場・感情のどれかを足して会話を前に進める。")
		}
	}
	if hasRedundantTranscript(transcript) {
		issues = append(issues, "直近発話の語彙や構文が近く、聞き手には停滞して聞こえる可能性がある。")
		fixes = append(fixes, "同じ抽象語を続けず、1ターンごとに具体的な出来事、短い感情、問いのいずれかへ切り替える。")
	}
	if len(issues) == 0 {
		return "QUALITY: pass\nISSUES:\n- 大きな損耗は検出されませんでした。\nPROMPT_FIX: ", ""
	}
	return fmt.Sprintf("QUALITY: fail\nISSUES:\n- %s\nPROMPT_FIX: %s", strings.Join(issues, "\n- "), strings.Join(dedupeNonEmpty(fixes), " ")), strings.Join(dedupeNonEmpty(fixes), " ")
}

func hasRedundantTranscript(transcript []string) bool {
	if len(transcript) < 4 {
		return false
	}
	start := len(transcript) - 4
	if start < 0 {
		start = 0
	}
	repeatedStarts := map[string]int{}
	for _, line := range transcript[start:] {
		text := stripSpeakerPrefix(line)
		runes := []rune(text)
		if len(runes) > 14 {
			runes = runes[:14]
		}
		key := strings.TrimSpace(string(runes))
		if key != "" {
			repeatedStarts[key]++
		}
	}
	for _, n := range repeatedStarts {
		if n >= 2 {
			return true
		}
	}
	return false
}

func stripSpeakerPrefix(line string) string {
	if i := strings.Index(line, ":"); i >= 0 {
		return strings.TrimSpace(line[i+1:])
	}
	if i := strings.Index(line, "："); i >= 0 {
		return strings.TrimSpace(line[i+len("："):])
	}
	return strings.TrimSpace(line)
}

func extractPromptGuidance(review string) string {
	var guides []string
	for _, line := range strings.Split(review, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := promptFixLineRe.FindStringSubmatch(line); len(m) == 2 {
			guides = append(guides, strings.TrimSpace(m[1]))
			continue
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "quality: pass") {
			return ""
		}
	}
	return strings.Join(dedupeNonEmpty(guides), " ")
}

func (o *IdleChatOrchestrator) addPromptGuideLocked(guide string) {
	guide = strings.TrimSpace(guide)
	if guide == "" {
		return
	}
	for _, existing := range o.promptGuides {
		if existing == guide {
			return
		}
	}
	o.promptGuides = append(o.promptGuides, guide)
	if len(o.promptGuides) > maxPromptGuides {
		o.promptGuides = o.promptGuides[len(o.promptGuides)-maxPromptGuides:]
	}
}

func promptGuidesFromHistory(history []SessionSummary, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for i := len(history) - 1; i >= 0 && len(out) < limit; i-- {
		guide := strings.TrimSpace(history[i].PromptGuidance)
		if guide == "" {
			continue
		}
		dup := false
		for _, existing := range out {
			if existing == guide {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, guide)
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

func formatPromptGuidance(guides []string) string {
	if len(guides) == 0 {
		return ""
	}
	return "\n\n【前回までの聞き手体験レビューに基づくプロンプト補正】\n- " + strings.Join(guides, "\n- ")
}

func joinPromptGuides(parts ...string) string {
	return strings.Join(dedupeNonEmpty(parts), " ")
}

func dedupeNonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		dup := false
		for _, existing := range out {
			if existing == s {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, s)
		}
	}
	return out
}
