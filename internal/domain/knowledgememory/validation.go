package knowledgememory

import (
	"fmt"
	"strings"
)

func ValidatePersonalArchiveEntry(item PersonalArchiveEntry) error {
	if strings.TrimSpace(item.EntryID) == "" {
		return fmt.Errorf("entry_id is required")
	}
	if strings.TrimSpace(item.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}
	if strings.TrimSpace(item.OriginalText) == "" {
		return fmt.Errorf("original_text is required")
	}
	if !item.Protected {
		return fmt.Errorf("personal archive original must be protected")
	}
	return nil
}

func ValidateCreativeKnowledgeItem(item CreativeKnowledgeItem) error {
	if strings.TrimSpace(item.ItemID) == "" {
		return fmt.Errorf("item_id is required")
	}
	if strings.TrimSpace(item.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return fmt.Errorf("status is required")
	}
	return nil
}

func ValidateNewsKnowledgeItem(item NewsKnowledgeItem) error {
	if strings.TrimSpace(item.ItemID) == "" {
		return fmt.Errorf("item_id is required")
	}
	if strings.TrimSpace(item.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(item.Topic) == "" {
		return fmt.Errorf("topic is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return fmt.Errorf("status is required")
	}
	return nil
}

func ValidateDailyIntakeRule(item DailyIntakeRule) error {
	if strings.TrimSpace(item.RuleID) == "" {
		return fmt.Errorf("rule_id is required")
	}
	if strings.TrimSpace(item.UserID) == "" {
		return fmt.Errorf("user_id is required")
	}
	if strings.TrimSpace(item.Topic) == "" {
		return fmt.Errorf("topic is required")
	}
	if strings.TrimSpace(item.Cadence) == "" {
		return fmt.Errorf("cadence is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return fmt.Errorf("status is required")
	}
	return nil
}

func ValidateTemporalMemoryMarker(item TemporalMemoryMarker) error {
	if strings.TrimSpace(item.MarkerID) == "" {
		return fmt.Errorf("marker_id is required")
	}
	switch strings.TrimSpace(item.Layer) {
	case "thread", "today", "3days", "week", "month", "year", "long_term":
	default:
		return fmt.Errorf("unsupported temporal memory layer")
	}
	if strings.TrimSpace(item.ReferenceID) == "" {
		return fmt.Errorf("reference_id is required")
	}
	if strings.TrimSpace(item.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	return nil
}

func ValidateDreamConsolidationRun(item DreamConsolidationRun) error {
	if strings.TrimSpace(item.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(item.Status) == "" {
		return fmt.Errorf("status is required")
	}
	if strings.TrimSpace(item.ReviewStatus) == "" {
		return fmt.Errorf("review_status is required")
	}
	if item.ReviewStatus == "approved" && item.Status != "reviewed" && item.Status != "promoted" {
		return fmt.Errorf("dream consolidation cannot be auto-approved")
	}
	return nil
}
