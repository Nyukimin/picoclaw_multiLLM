package config

// VocabularyConfig holds configuration for vocabulary store
type VocabularyConfig struct {
	UpdateIntervalHours int      `yaml:"update_interval_hours"`
	MaxEntriesInContext int      `yaml:"max_entries_in_context"`
	RSSSources          []string `yaml:"rss_sources"`
}

// DefaultVocabularyConfig returns default configuration
func DefaultVocabularyConfig() VocabularyConfig {
	return VocabularyConfig{
		UpdateIntervalHours: 6,
		MaxEntriesInContext: 20,
		RSSSources: []string{
			"https://example.com/news/rss",
		},
	}
}
