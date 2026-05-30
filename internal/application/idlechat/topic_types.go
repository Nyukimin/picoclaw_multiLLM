package idlechat

import (
	"errors"
	"strings"
)

type TopicCategory string

const (
	TopicCategorySingle   TopicCategory = "single"
	TopicCategoryDouble   TopicCategory = "double"
	TopicCategoryExternal TopicCategory = "external"
	TopicCategoryMovie    TopicCategory = "movie"
	TopicCategoryNews     TopicCategory = "news"
	TopicCategoryForecast TopicCategory = "forecast"
	TopicCategoryStory    TopicCategory = "story"
)

var (
	ErrUnsupportedTopicCategory    = errors.New("topic_category_unsupported")
	ErrTopicSeedUnavailable        = errors.New("topic_seed_unavailable")
	ErrTopicGenerationInvalidJSON  = errors.New("topic_generation_invalid_json")
	ErrTopicGenerationNoCandidates = errors.New("topic_generation_no_candidates")
	ErrTopicContractViolation      = errors.New("topic_contract_violation")
	ErrTopicJudgeInvalidJSON       = errors.New("topic_judge_invalid_json")
	ErrTopicJudgeWinnerMissing     = errors.New("topic_judge_winner_missing")
	ErrTopicJudgeLowScore          = errors.New("topic_judge_low_score")
	ErrRecentTopicExactDuplicate   = errors.New("topic_recent_exact_duplicate")
	ErrRecentTopicTooSimilar       = errors.New("topic_recent_too_similar")
	ErrTopicGenerationFailed       = errors.New("topic_generation_failed")
)

type TopicSeed struct {
	Category TopicCategory `json:"category"`

	Genre1 string `json:"genre_1,omitempty"`
	Genre2 string `json:"genre_2,omitempty"`

	ExternalMaterial *ExternalMaterialSeed `json:"external_material,omitempty"`

	News *NewsSeed `json:"news,omitempty"`

	ForecastDomain string   `json:"forecast_domain,omitempty"`
	TrendKeywords  []string `json:"trend_keywords,omitempty"`

	StoryBase      string `json:"story_base,omitempty"`
	StoryTransform string `json:"story_transform,omitempty"`

	RecentTopics []RecentTopic `json:"recent_topics,omitempty"`
}

type ExternalMaterialSeed struct {
	Title    string `json:"title"`
	Summary  string `json:"summary,omitempty"`
	Provider string `json:"provider,omitempty"`
	URL      string `json:"url,omitempty"`
	Category string `json:"category,omitempty"`
}

type RecentTopic struct {
	Topic    string        `json:"topic"`
	Category TopicCategory `json:"category,omitempty"`
	Strategy string        `json:"strategy,omitempty"`
}

type TopicCandidate struct {
	Topic               string `json:"topic"`
	InterestingnessAxis string `json:"interestingness_axis"`
	OpeningHook         string `json:"opening_hook"`
	Avoid               string `json:"avoid"`
	Rationale           string `json:"rationale,omitempty"`
}

type TopicJudgeResult struct {
	WinnerTopic         string            `json:"winner_topic"`
	Scores              []TopicJudgeScore `json:"scores"`
	RejectReasonSummary string            `json:"reject_reason_summary,omitempty"`
}

type TopicJudgeScore struct {
	Topic                 string `json:"topic"`
	CategoryFit           int    `json:"category_fit"`
	Concreteness          int    `json:"concreteness"`
	Curiosity             int    `json:"curiosity"`
	ConversationPotential int    `json:"conversation_potential"`
	AxisStrength          int    `json:"axis_strength"`
	Novelty               int    `json:"novelty"`
	Safety                int    `json:"safety"`
	Total                 int    `json:"total"`
	Reason                string `json:"reason"`
}

type TopicGenerationResult struct {
	Topic    string        `json:"topic"`
	Category TopicCategory `json:"category"`
	Strategy string        `json:"strategy"`

	InterestingnessAxis string `json:"interestingness_axis"`
	OpeningHook         string `json:"opening_hook"`
	Avoid               string `json:"avoid"`

	Seed       TopicSeed         `json:"seed"`
	Candidates []TopicCandidate  `json:"candidates,omitempty"`
	Judge      *TopicJudgeResult `json:"judge,omitempty"`
	Provider   string            `json:"provider"`
}

type TopicGenerationDiagnostic struct {
	SessionID         string                       `json:"session_id,omitempty"`
	Category          string                       `json:"category"`
	Strategy          string                       `json:"strategy"`
	Attempt           int                          `json:"attempt"`
	ErrorCode         string                       `json:"error_code,omitempty"`
	ErrorMessage      string                       `json:"error_message,omitempty"`
	SeedSummary       string                       `json:"seed_summary,omitempty"`
	CandidateCount    int                          `json:"candidate_count,omitempty"`
	InvalidCandidates []InvalidCandidateDiagnostic `json:"invalid_candidates,omitempty"`
	WinnerTopic       string                       `json:"winner_topic,omitempty"`
	JudgeTotal        int                          `json:"judge_total,omitempty"`
}

type InvalidCandidateDiagnostic struct {
	Topic string `json:"topic"`
	Error string `json:"error"`
}

var ExpectedAxisByCategory = map[TopicCategory]string{
	TopicCategorySingle:   "観察",
	TopicCategoryDouble:   "接続",
	TopicCategoryExternal: "偶然の意味化",
	TopicCategoryMovie:    "共同妄想",
	TopicCategoryNews:     "現実の影響",
	TopicCategoryForecast: "変化の分岐",
	TopicCategoryStory:    "視点反転",
}

func NormalizeTopicCategory(s string) (TopicCategory, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "single":
		return TopicCategorySingle, nil
	case "double":
		return TopicCategoryDouble, nil
	case "external":
		return TopicCategoryExternal, nil
	case "movie":
		return TopicCategoryMovie, nil
	case "news":
		return TopicCategoryNews, nil
	case "forecast":
		return TopicCategoryForecast, nil
	case "story", "story-simple":
		return TopicCategoryStory, nil
	default:
		return "", ErrUnsupportedTopicCategory
	}
}

func TopicCategoryFromStrategy(strategy TopicStrategy) (TopicCategory, error) {
	return NormalizeTopicCategory(string(strategy))
}

func StrategyFromCategory(category TopicCategory) string {
	switch category {
	case TopicCategoryStory:
		return "story-simple"
	default:
		return string(category)
	}
}

func categoryForSummaryStrategy(strategy TopicStrategy) TopicCategory {
	category, err := TopicCategoryFromStrategy(strategy)
	if err == nil {
		return category
	}
	text := strings.ToLower(strings.TrimSpace(string(strategy)))
	switch {
	case strings.HasPrefix(text, "forecast"):
		return TopicCategoryForecast
	case strings.HasPrefix(text, "story"):
		return TopicCategoryStory
	default:
		return ""
	}
}
