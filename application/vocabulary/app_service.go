package vocabulary

import (
	"log"
	"strings"
	"time"

	vocabDomain "github.com/Nyukimin/picoclaw_multiLLM/domain/vocabulary"
)

// AppService implements the vocabulary service
type AppService struct {
	repo     vocabDomain.Repository
	sources  []RSSSource
	extractor *TermExtractor
}

// RSSSource configuration for fetching news
type RSSSource struct {
	URL      string
	Name     string
	Category string
}

// NewAppService creates a new application service
func NewAppService(repo vocabDomain.Repository, sources []RSSSource) *AppService {
	return &AppService{
		repo:     repo,
		sources:  sources,
		extractor: NewTermExtractor(),
	}
}

// UpdateFromSources fetches from RSS sources and stores extracted terms
func (s *AppService) UpdateFromSources() (int, error) {
	var totalAdded int
	
	for _, source := range s.sources {
		headlines, err := s.fetchHeadlines(source.URL)
		if err != nil {
			log.Printf("Failed to fetch from %s: %v", source.Name, err)
			continue
		}
		
		for _, headline := range headlines {
			terms := s.extractor.ExtractTerms(headline)
			for _, term := range terms {
				entry := &vocabDomain.Entry{
					Term:        term.Term,
					Description: term.Description,
					Source:      source.Name,
					Timestamp:   time.Now(),
					Categories:  []string{source.Category},
				}
				
				if err := s.repo.Store(entry); err != nil {
					log.Printf("Failed to store term %s: %v", term.Term, err)
				} else {
					totalAdded++
				}
			}
		}
	}
	
	return totalAdded, nil
}

// GetContext formats recent entries for LLM injection
func (s *AppService) GetContext(maxEntries int) (string, error) {
	entries, err := s.repo.FindRecent(7) // Last 7 days
	if err != nil {
		return "", err
	}
	
	if len(entries) > maxEntries {
		entries = entries[:maxEntries]
	}
	
	var builder strings.Builder
	builder.WriteString("Recent topics glossary:\n")
	for _, entry := range entries {
		builder.WriteString("- ")
		builder.WriteString(entry.Term)
		builder.WriteString(": ")
		builder.WriteString(entry.Description)
		builder.WriteString(" (")
		builder.WriteString(entry.Source)
		builder.WriteString(")\n")
	}
	
	return builder.String(), nil
}

// SearchTerms finds entries matching search terms
func (s *AppService) SearchTerms(terms []string) ([]*vocabDomain.Entry, error) {
	var results []*vocabDomain.Entry
	for _, term := range terms {
		found, err := s.repo.FindByTerm(term)
		if err != nil {
			return nil, err
		}
		results = append(results, found...)
	}
	return results, nil
}

// Helper methods
func (s *AppService) fetchHeadlines(url string) ([]string, error) {
	// TODO: Implement RSS fetching
	// For minimal implementation, return mock data
	return []string{
		"AI Summit discusses new regulations",
		"Tech company launches quantum computing service",
		"Climate conference addresses carbon emissions",
	}, nil
}
