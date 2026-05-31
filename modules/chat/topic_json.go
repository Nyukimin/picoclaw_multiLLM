package chat

import (
	"encoding/json"
	"strings"
)

type topicCandidatesEnvelope struct {
	Candidates []TopicCandidate `json:"candidates"`
}

func ParseTopicCandidates(raw string) ([]TopicCandidate, error) {
	text := strings.TrimSpace(ExtractJSONPayload(raw))
	if text == "" {
		return nil, ErrTopicGenerationInvalidJSON
	}
	var env topicCandidatesEnvelope
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		return nil, ErrTopicGenerationInvalidJSON
	}
	if len(env.Candidates) == 0 {
		return nil, ErrTopicGenerationNoCandidates
	}
	return env.Candidates, nil
}

func ParseTopicJudgeResult(raw string) (TopicJudgeResult, error) {
	text := strings.TrimSpace(ExtractJSONPayload(raw))
	if text == "" {
		return TopicJudgeResult{}, ErrTopicJudgeInvalidJSON
	}
	var judge TopicJudgeResult
	if err := json.Unmarshal([]byte(text), &judge); err != nil {
		return TopicJudgeResult{}, ErrTopicJudgeInvalidJSON
	}
	if strings.TrimSpace(judge.WinnerTopic) == "" {
		return TopicJudgeResult{}, ErrTopicJudgeWinnerMissing
	}
	for i := range judge.Scores {
		judge.Scores[i] = NormalizeJudgeScoreTotal(judge.Scores[i])
	}
	return judge, nil
}

func ExtractJSONPayload(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) >= 3 {
			text = strings.Join(lines[1:len(lines)-1], "\n")
			text = strings.TrimSpace(text)
		}
	}
	if strings.HasPrefix(text, "{") && strings.HasSuffix(text, "}") {
		return text
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		return strings.TrimSpace(text[start : end+1])
	}
	return ""
}
