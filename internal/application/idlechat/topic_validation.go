package idlechat

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	MinJudgeTotal                  = 24
	MinCategoryFit                 = 4
	MinSafety                      = 4
	RecentTopicSimilarityThreshold = 0.82
)

var CommonForbiddenMetaTerms = []string{
	"カテゴリ", "strategy", "provider", "seed", "内部",
	"生成", "プロンプト", "JSON", "候補",
}

var ExternalForbiddenTerms = []string{
	"Wikipedia", "ウィキペディア",
	"外部刺激", "ランダム記事", "偶然の記事",
	"記事", "ページ", "検索結果", "取得元",
	"provider", "RSS", "URL",
}

var movieTopicPattern = regexp.MustCompile(`^「[^」]{2,24}」ってどんな映画？$`)

func ValidateSeedForCategory(category TopicCategory, seed TopicSeed) error {
	switch category {
	case TopicCategorySingle:
		if strings.TrimSpace(seed.Genre1) == "" {
			return fmt.Errorf("%w: genre_1 is required", ErrTopicSeedUnavailable)
		}
	case TopicCategoryDouble:
		if strings.TrimSpace(seed.Genre1) == "" || strings.TrimSpace(seed.Genre2) == "" {
			return fmt.Errorf("%w: genre_1 and genre_2 are required", ErrTopicSeedUnavailable)
		}
	case TopicCategoryExternal:
		if strings.TrimSpace(seed.Genre1) == "" || seed.ExternalMaterial == nil || strings.TrimSpace(seed.ExternalMaterial.Title) == "" {
			return fmt.Errorf("%w: external material title and genre_1 are required", ErrTopicSeedUnavailable)
		}
	case TopicCategoryMovie:
		return nil
	case TopicCategoryNews:
		if seed.News == nil || strings.TrimSpace(seed.News.Title) == "" {
			return fmt.Errorf("%w: news seed is required", ErrTopicSeedUnavailable)
		}
	case TopicCategoryForecast:
		if strings.TrimSpace(seed.ForecastDomain) == "" {
			return fmt.Errorf("%w: forecast_domain is required", ErrTopicSeedUnavailable)
		}
	case TopicCategoryStory:
		if strings.TrimSpace(seed.StoryBase) == "" {
			return fmt.Errorf("%w: story_base is required", ErrTopicSeedUnavailable)
		}
	default:
		return ErrUnsupportedTopicCategory
	}
	return nil
}

func ValidateTopicCandidate(category TopicCategory, seed TopicSeed, candidate TopicCandidate) error {
	topic := strings.TrimSpace(candidate.Topic)
	if err := ValidateCommonTopic(topic); err != nil {
		return err
	}
	expectedAxis := ExpectedAxisByCategory[category]
	if expectedAxis != "" && strings.TrimSpace(candidate.InterestingnessAxis) != expectedAxis {
		return fmt.Errorf("%w: axis must be %q", ErrTopicContractViolation, expectedAxis)
	}
	switch category {
	case TopicCategorySingle:
		return nil
	case TopicCategoryDouble:
		if !containsAny(topic, strings.TrimSpace(seed.Genre1)) || !containsAny(topic, strings.TrimSpace(seed.Genre2)) {
			return fmt.Errorf("%w: double topic must contain both genres", ErrTopicContractViolation)
		}
	case TopicCategoryExternal:
		for _, term := range ExternalForbiddenTerms {
			if containsTopicTerm(topic, term) {
				return fmt.Errorf("%w: external topic leaks meta term %q", ErrTopicContractViolation, term)
			}
		}
		if seed.ExternalMaterial != nil && strings.TrimSpace(seed.ExternalMaterial.Title) != "" && !topicContainsLooseMaterial(topic, seed.ExternalMaterial.Title) {
			return fmt.Errorf("%w: external topic must preserve material", ErrTopicContractViolation)
		}
	case TopicCategoryMovie:
		if !movieTopicPattern.MatchString(topic) {
			return fmt.Errorf("%w: movie topic must match required format", ErrTopicContractViolation)
		}
		title := strings.TrimSuffix(strings.TrimPrefix(topic, "「"), "」ってどんな映画？")
		if strings.ContainsAny(title, "。！？!?") || containsAny(title, "あらすじ", "について", "映画について") {
			return fmt.Errorf("%w: movie title includes explanation", ErrTopicContractViolation)
		}
	case TopicCategoryNews:
		if seed.News == nil || strings.TrimSpace(seed.News.Title) == "" {
			return fmt.Errorf("%w: news seed is required", ErrTopicSeedUnavailable)
		}
		if containsTopicTerm(topic, "ニュースについて") || containsTopicTerm(topic, "記事") || containsTopicTerm(topic, "RSS") || containsTopicTerm(topic, "URL") || containsTopicTerm(topic, "provider") {
			return fmt.Errorf("%w: news topic leaks source or weak form", ErrTopicContractViolation)
		}
		if source := strings.TrimSpace(seed.News.Source); source != "" && containsTopicTerm(topic, source) {
			return fmt.Errorf("%w: news topic leaks source", ErrTopicContractViolation)
		}
	case TopicCategoryForecast:
		if strings.TrimSpace(seed.ForecastDomain) == "" {
			return fmt.Errorf("%w: forecast_domain is required", ErrTopicSeedUnavailable)
		}
		if topic == "AIの未来" || topic == "未来社会について" || topic == "人類はどうなるか" {
			return fmt.Errorf("%w: forecast topic is too abstract", ErrTopicContractViolation)
		}
		if !containsAny(topic, "変える", "変わる", "どう", "行方", "分岐", "影響", "再編", "変化") {
			return fmt.Errorf("%w: forecast topic must include change structure", ErrTopicContractViolation)
		}
	case TopicCategoryStory:
		if strings.TrimSpace(seed.StoryBase) == "" {
			return fmt.Errorf("%w: story_base is required", ErrTopicSeedUnavailable)
		}
		if !containsAny(topic, strings.TrimSpace(seed.StoryBase)) {
			return fmt.Errorf("%w: story topic must preserve story_base", ErrTopicContractViolation)
		}
		if !containsAny(topic, "視点", "役割", "語り", "語り直", "側", "記録係", "時代", "反転") {
			return fmt.Errorf("%w: story topic must include transform cue", ErrTopicContractViolation)
		}
	}
	return nil
}

func ValidateCommonTopic(topic string) error {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return fmt.Errorf("%w: empty topic", ErrTopicContractViolation)
	}
	if strings.ContainsAny(topic, "\r\n") {
		return fmt.Errorf("%w: topic must be one line", ErrTopicContractViolation)
	}
	n := utf8.RuneCountInString(topic)
	if n < 4 || n > 90 {
		return fmt.Errorf("%w: topic length out of range", ErrTopicContractViolation)
	}
	if strings.HasPrefix(topic, "{") || strings.HasPrefix(topic, "[") || strings.Contains(topic, "\":") {
		return fmt.Errorf("%w: topic looks like json", ErrTopicContractViolation)
	}
	for _, term := range CommonForbiddenMetaTerms {
		if containsTopicTerm(topic, term) {
			return fmt.Errorf("%w: topic leaks meta term %q", ErrTopicContractViolation, term)
		}
	}
	if hasPromptLeak(topic) || hasInternalReasoningLeak(topic) {
		return fmt.Errorf("%w: topic leaks prompt or reasoning", ErrTopicContractViolation)
	}
	return nil
}

func ValidateJudgeResult(judge TopicJudgeResult, candidates []TopicCandidate) (TopicCandidate, TopicJudgeScore, error) {
	return ValidateJudgeResultWithThresholds(judge, candidates, MinJudgeTotal, MinCategoryFit, MinSafety)
}

func ValidateJudgeResultWithThresholds(judge TopicJudgeResult, candidates []TopicCandidate, minTotal, minCategoryFit, minSafety int) (TopicCandidate, TopicJudgeScore, error) {
	if minTotal <= 0 {
		minTotal = MinJudgeTotal
	}
	if minCategoryFit <= 0 {
		minCategoryFit = MinCategoryFit
	}
	if minSafety <= 0 {
		minSafety = MinSafety
	}
	winnerTopic := strings.TrimSpace(judge.WinnerTopic)
	if winnerTopic == "" {
		return TopicCandidate{}, TopicJudgeScore{}, ErrTopicJudgeWinnerMissing
	}
	candidateByTopic := make(map[string]TopicCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByTopic[strings.TrimSpace(candidate.Topic)] = candidate
	}
	winner, ok := candidateByTopic[winnerTopic]
	if !ok {
		return TopicCandidate{}, TopicJudgeScore{}, ErrTopicJudgeWinnerMissing
	}
	for _, score := range judge.Scores {
		if strings.TrimSpace(score.Topic) != winnerTopic {
			continue
		}
		score = normalizeJudgeScoreTotal(score)
		if score.Total < minTotal || score.CategoryFit < minCategoryFit || score.Safety < minSafety {
			return TopicCandidate{}, score, ErrTopicJudgeLowScore
		}
		return winner, score, nil
	}
	return TopicCandidate{}, TopicJudgeScore{}, ErrTopicJudgeWinnerMissing
}

func normalizeJudgeScoreTotal(score TopicJudgeScore) TopicJudgeScore {
	score.Total = score.CategoryFit + score.Concreteness + score.Curiosity + score.ConversationPotential + score.AxisStrength + score.Novelty + score.Safety
	return score
}

func CheckRecentTopicSimilarity(topic string, recent []RecentTopic, threshold float64) error {
	if threshold <= 0 {
		threshold = RecentTopicSimilarityThreshold
	}
	normalized := NormalizeTopicForSimilarity(topic)
	if normalized == "" {
		return nil
	}
	for _, item := range recent {
		other := NormalizeTopicForSimilarity(item.Topic)
		if other == "" {
			continue
		}
		if normalized == other {
			return ErrRecentTopicExactDuplicate
		}
		if textSimilarity(normalized, other) >= threshold {
			return ErrRecentTopicTooSimilar
		}
	}
	return nil
}

func NormalizeTopicForSimilarity(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "　", " ")
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		switch r {
		case '。', '、', ',', '.', '！', '!', '？', '?', '「', '」', '『', '』', '(', ')', '（', '）', ':', '：':
			return ' '
		default:
			return r
		}
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func topicContainsLooseMaterial(topic, material string) bool {
	topic = NormalizeTopicForSimilarity(topic)
	material = NormalizeTopicForSimilarity(material)
	if material == "" {
		return true
	}
	if strings.Contains(topic, material) {
		return true
	}
	for _, token := range strings.Fields(material) {
		if utf8.RuneCountInString(token) >= 3 && strings.Contains(topic, token) {
			return true
		}
	}
	return false
}

func containsTopicTerm(topic, term string) bool {
	topic = strings.ToLower(strings.TrimSpace(topic))
	term = strings.ToLower(strings.TrimSpace(term))
	return term != "" && strings.Contains(topic, term)
}
