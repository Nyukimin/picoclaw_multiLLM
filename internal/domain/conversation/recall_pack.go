package conversation

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nyukimin/picoclaw_multiLLM/internal/domain/llm"
)

// PromptConstraints はプロンプト組み立ての制約
type PromptConstraints struct {
	MaxTotalTokens    int // LLM の MaxContext（デフォルト: 8192）
	MaxPromptTokens   int // プロンプトに使えるトークン（デフォルト: 4000）
	MaxResponseTokens int // 応答用トークン（デフォルト: 512）
	RecallBudgetRatio float64
}

// DefaultConstraints はデフォルトのトークン制約を返す
func DefaultConstraints() PromptConstraints {
	return PromptConstraints{
		MaxTotalTokens:    8192,
		MaxPromptTokens:   4000,
		MaxResponseTokens: 512,
		RecallBudgetRatio: 0.10,
	}
}

// RecallPack は Recall 結果を構造化した LLM プロンプト注入用フォーマット
type RecallPack struct {
	// RollingSummary: L0現在会話の古い部分を圧縮した要約
	RollingSummary string

	// ShortContext: 現在の Thread 内の直近メッセージ（最大12件）
	ShortContext []Message

	// MidSummaries: 同一セッション内の過去 Thread 要約（最大3件）
	MidSummaries []ThreadSummary

	// LongFacts: VectorDB から類似検索した過去の知識（最大3件）
	LongFacts []string

	// KBSnippets: ドメイン知識ベースからの関連情報（最大2件）
	KBSnippets []string

	// SearchCacheSnippets: 外部検索のfresh cache hitから得た参照情報
	SearchCacheSnippets []SearchCacheSnippet

	// RejectedTraceItems: role filterやbudget制御でプロンプト採用されなかった候補のtrace
	RejectedTraceItems []RecallTraceItem

	// Persona: キャラクター設定
	Persona PersonaState

	// UserProfile: ユーザーの好み・傾向
	UserProfile UserProfile

	// Constraints: トークン上限等
	Constraints PromptConstraints
}

type SearchCacheSnippet struct {
	Query       string
	Provider    string
	ResultsJSON string
	SourceURLs  []string
	RetrievedAt time.Time
	Roles       []string
}

type TokenEstimator interface {
	EstimateTokens(text string) int
}

type TokenEstimatorFunc func(text string) int

func (f TokenEstimatorFunc) EstimateTokens(text string) int {
	if f == nil {
		return estimateRecallTokens(text)
	}
	return f(text)
}

// HasContext は RecallPack に何らかの文脈があるかを返す
func (rp *RecallPack) HasContext() bool {
	return len(rp.ShortContext) > 0 ||
		strings.TrimSpace(rp.RollingSummary) != "" ||
		len(rp.MidSummaries) > 0 ||
		len(rp.LongFacts) > 0 ||
		len(rp.KBSnippets) > 0 ||
		len(rp.SearchCacheSnippets) > 0
}

// ToPromptMessages は RecallPack を llm.Message のスライスに変換
// userMessage は含めない（呼び出し側で追加する）
func (rp *RecallPack) ToPromptMessages() []llm.Message {
	var messages []llm.Message

	// 1. システムプロンプト（Persona + UserProfile）
	systemPrompt := rp.Persona.SystemPrompt
	if profileText := rp.UserProfile.ToPromptText(); profileText != "" {
		systemPrompt += "\n\n" + profileText
	}
	if systemPrompt != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}

	// 2. 過去文脈（L2中期記憶 + L3長期記憶 + KB）
	contextText := ""
	if strings.TrimSpace(rp.RollingSummary) != "" {
		contextText += "【L0 現在会話 / rolling summary】\n"
		contextText += "- " + strings.TrimSpace(rp.RollingSummary) + "\n"
	}
	if len(rp.MidSummaries) > 0 {
		contextText += "【L2 中期記憶 / 過去の会話から思い出したこと】\n"
		for _, s := range rp.MidSummaries {
			contextText += "- " + s.Summary + "\n"
		}
	}
	if len(rp.LongFacts) > 0 {
		contextText += "【L3 長期記憶 / 過去の会話から思い出したこと】\n"
		for _, f := range rp.LongFacts {
			contextText += "- " + f + "\n"
		}
	}
	if len(rp.KBSnippets) > 0 {
		contextText += "【Knowledge DB / 参考知識】\n"
		for _, kb := range rp.KBSnippets {
			contextText += kb + "\n"
		}
	}
	if len(rp.SearchCacheSnippets) > 0 {
		contextText += "【L1 Search Cache / 検索キャッシュ】\n"
		for _, cache := range rp.SearchCacheSnippets {
			contextText += "- " + cache.ToPromptText() + "\n"
		}
	}
	if contextText != "" {
		messages = append(messages, llm.Message{
			Role:    "system",
			Content: contextText,
		})
	}

	// 3. 直近の会話履歴（ShortContext）
	for _, msg := range rp.ShortContext {
		role := "user"
		switch msg.Speaker {
		case SpeakerMio:
			role = "assistant"
		case SpeakerUser:
			role = "user"
		default:
			role = "system"
		}
		messages = append(messages, llm.Message{
			Role:    role,
			Content: msg.Msg,
		})
	}

	return messages
}

func (rp *RecallPack) ApplyRecallBudget(maxContextTokens int, ratio float64) RecallPack {
	return rp.ApplyRecallBudgetWithEstimator(maxContextTokens, ratio, nil)
}

func (rp *RecallPack) ApplyRecallBudgetWithEstimator(maxContextTokens int, ratio float64, estimator TokenEstimator) RecallPack {
	if rp == nil {
		return RecallPack{}
	}
	if maxContextTokens <= 0 || ratio <= 0 {
		return *rp
	}
	budget := int(float64(maxContextTokens) * ratio)
	if budget <= 0 {
		return *rp
	}
	trimmed := *rp
	trimmed.MidSummaries = nil
	trimmed.LongFacts = nil
	trimmed.KBSnippets = nil
	trimmed.SearchCacheSnippets = nil
	used := 0
	canAdd := func(text string) bool {
		cost := estimateWithFallback(estimator, text)
		if cost > budget {
			return false
		}
		if used+cost > budget {
			return false
		}
		used += cost
		return true
	}
	for _, summary := range rp.MidSummaries {
		if canAdd(summary.Summary) {
			trimmed.MidSummaries = append(trimmed.MidSummaries, summary)
		}
	}
	for _, fact := range rp.LongFacts {
		if canAdd(fact) {
			trimmed.LongFacts = append(trimmed.LongFacts, fact)
		}
	}
	for _, snippet := range rp.KBSnippets {
		if canAdd(snippet) {
			trimmed.KBSnippets = append(trimmed.KBSnippets, snippet)
		}
	}
	for _, cache := range rp.SearchCacheSnippets {
		if canAdd(cache.ToPromptText()) {
			trimmed.SearchCacheSnippets = append(trimmed.SearchCacheSnippets, cache)
		}
	}
	return trimmed
}

func estimateWithFallback(estimator TokenEstimator, text string) int {
	if estimator == nil {
		return estimateRecallTokens(text)
	}
	cost := estimator.EstimateTokens(text)
	if cost <= 0 {
		return estimateRecallTokens(text)
	}
	return cost
}

func (rp *RecallPack) FilterForRole(role string) RecallPack {
	if rp == nil {
		return RecallPack{}
	}
	role = normalizeRecallRole(role)
	if role == "" {
		return *rp
	}
	filtered := *rp
	filtered.MidSummaries = nil
	filtered.KBSnippets = nil
	filtered.SearchCacheSnippets = nil
	filtered.RejectedTraceItems = append([]RecallTraceItem(nil), rp.RejectedTraceItems...)
	for _, summary := range rp.MidSummaries {
		if recallRolesMatch(summary.Roles, role) {
			filtered.MidSummaries = append(filtered.MidSummaries, summary)
		} else {
			filtered.RejectedTraceItems = append(filtered.RejectedTraceItems, rejectedThreadSummaryTrace(summary, "role "+role+" does not match candidate roles"))
		}
	}
	if recallRoleAllowsKB(role) {
		filtered.KBSnippets = append([]string(nil), rp.KBSnippets...)
	} else {
		for _, snippet := range rp.KBSnippets {
			filtered.RejectedTraceItems = append(filtered.RejectedTraceItems, rejectedKnowledgeTrace(snippet, "role "+role+" does not use Knowledge DB snippets by default"))
		}
	}
	for _, snippet := range rp.SearchCacheSnippets {
		if recallRoleAllowsSearchCache(role) && recallRolesMatch(snippet.Roles, role) {
			filtered.SearchCacheSnippets = append(filtered.SearchCacheSnippets, snippet)
			continue
		}
		reason := "role " + role + " does not use L1 search cache by default"
		if recallRoleAllowsSearchCache(role) && !recallRolesMatch(snippet.Roles, role) {
			reason = "role " + role + " does not match search cache roles"
		}
		filtered.RejectedTraceItems = append(filtered.RejectedTraceItems, rejectedSearchCacheTrace(snippet, reason))
	}
	return filtered
}

func rejectedThreadSummaryTrace(summary ThreadSummary, reason string) RecallTraceItem {
	return RecallTraceItem{
		Layer:       "L2",
		Kind:        "thread_summary",
		Summary:     summary.Summary,
		Score:       summary.Score,
		Decision:    "rejected",
		Reason:      reason,
		PromptIndex: -1,
	}
}

func rejectedKnowledgeTrace(snippet string, reason string) RecallTraceItem {
	return RecallTraceItem{
		Layer:       "L3",
		Kind:        "knowledge",
		Summary:     snippet,
		Decision:    "rejected",
		Reason:      reason,
		PromptIndex: -1,
	}
}

func rejectedSearchCacheTrace(snippet SearchCacheSnippet, reason string) RecallTraceItem {
	return RecallTraceItem{
		Layer:       "L1",
		Kind:        "search_cache",
		Summary:     snippet.ResultsJSON,
		Query:       snippet.Query,
		Provider:    snippet.Provider,
		SourceURLs:  append([]string(nil), snippet.SourceURLs...),
		RetrievedAt: snippet.RetrievedAt,
		Decision:    "rejected",
		Reason:      reason,
		PromptIndex: -1,
	}
}

func recallRoleAllowsKB(role string) bool {
	switch role {
	case "worker", "wild":
		return true
	default:
		return false
	}
}

func recallRoleAllowsSearchCache(role string) bool {
	switch role {
	case "worker":
		return true
	default:
		return false
	}
}

func recallRolesMatch(roles []string, role string) bool {
	if len(roles) == 0 {
		return true
	}
	for _, candidate := range roles {
		normalized := normalizeRecallRole(candidate)
		if normalized == role || normalized == "all" {
			return true
		}
	}
	return false
}

func normalizeRecallRole(role string) string {
	return strings.ToLower(strings.TrimSpace(role))
}

func estimateRecallTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	runes := len([]rune(text))
	return runes/4 + 1
}

func (s SearchCacheSnippet) ToPromptText() string {
	var parts []string
	if s.Query != "" {
		parts = append(parts, fmt.Sprintf("query=%s", s.Query))
	}
	if s.Provider != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", s.Provider))
	}
	if !s.RetrievedAt.IsZero() {
		parts = append(parts, fmt.Sprintf("retrieved_at=%s", s.RetrievedAt.UTC().Format(time.RFC3339)))
	}
	if len(s.SourceURLs) > 0 {
		parts = append(parts, "sources="+strings.Join(s.SourceURLs, ", "))
	}
	if s.ResultsJSON != "" {
		parts = append(parts, "results_json="+s.ResultsJSON)
	}
	return strings.Join(parts, "; ")
}
