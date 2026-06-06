package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func (s *L1SQLiteStore) PromoteValidatedStagingItemToDomainGraph(ctx context.Context, id string, domain string, entityType string, entityID string, relationType string, confidence float64) (*L1DomainGraphAssertion, error) {
	id = strings.TrimSpace(id)
	domain = normalizeNewsCategory(domain)
	entityType = normalizeDomainGraphToken(entityType)
	entityID = strings.TrimSpace(entityID)
	relationType = normalizeDomainGraphToken(relationType)
	if id == "" {
		return nil, errors.New("l1 staging item id is required")
	}
	if err := validateKnowledgeDomain(domain); err != nil {
		return nil, err
	}
	if entityType == "" {
		return nil, errors.New("domain graph entity_type is required")
	}
	if confidence <= 0 {
		confidence = 0.5
	}
	if confidence > 1 {
		return nil, errors.New("domain graph confidence must be <= 1")
	}
	item, err := s.stagingItemByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.ValidationStatus != L1StagingStatusValidated {
		return nil, fmt.Errorf("l1 staging item must be validated before domain graph promotion: %s", item.ValidationStatus)
	}
	if item.Kind != L1StagingKindExternalFetch && item.Kind != L1StagingKindSearchResult {
		return nil, fmt.Errorf("l1 staging item kind cannot be promoted to domain graph: %s", item.Kind)
	}
	now := time.Now().UTC()
	summary := strings.TrimSpace(item.SummaryDraft)
	if summary == "" {
		summary = strings.TrimSpace(item.RawText)
	}
	evidence := map[string]interface{}{}
	for k, v := range item.Meta {
		evidence[k] = v
	}
	evidence["staging_id"] = item.ID
	evidence["staging_namespace"] = item.Namespace
	evidence["staging_kind"] = item.Kind
	evidence["event_id"] = item.EventID
	evidence["source_id"] = item.SourceID
	evidence["source_url"] = item.SourceURL
	evidence["raw_hash"] = item.RawHash
	evidence["license_note"] = item.LicenseNote
	evidence["validation_status"] = item.ValidationStatus
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal domain graph assertion evidence: %w", err)
	}
	assertion := &L1DomainGraphAssertion{
		ID:               fmt.Sprintf("dg:%s:%s:%s", domain, item.EventID, item.RawHash[:12]),
		StagingID:        item.ID,
		Domain:           domain,
		EntityType:       entityType,
		EntityID:         entityID,
		RelationType:     relationType,
		SourceID:         item.SourceID,
		SourceURL:        item.SourceURL,
		RawHash:          item.RawHash,
		Summary:          summary,
		Confidence:       confidence,
		ValidationStatus: item.ValidationStatus,
		Evidence:         evidence,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO domain_graph_assertion (
	assertion_id, staging_id, domain, entity_type, entity_id, relation_type,
	source_id, source_url, raw_hash, summary, confidence, validation_status,
	evidence_json, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(staging_id) DO UPDATE SET
	domain = excluded.domain,
	entity_type = excluded.entity_type,
	entity_id = excluded.entity_id,
	relation_type = excluded.relation_type,
	source_id = excluded.source_id,
	source_url = excluded.source_url,
	raw_hash = excluded.raw_hash,
	summary = excluded.summary,
	confidence = excluded.confidence,
	validation_status = excluded.validation_status,
	evidence_json = excluded.evidence_json,
	updated_at = excluded.updated_at
`, assertion.ID, assertion.StagingID, assertion.Domain, assertion.EntityType, assertion.EntityID, assertion.RelationType,
		assertion.SourceID, assertion.SourceURL, assertion.RawHash, assertion.Summary, assertion.Confidence, assertion.ValidationStatus,
		string(evidenceJSON), assertion.CreatedAt, assertion.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to promote l1 staging item to domain graph assertion: %w", err)
	}
	namespace, err := BuildL1Namespace(NamespaceKindKnowledge, "domain_graph_"+domain)
	if err != nil {
		return nil, err
	}
	if _, err := s.AppendEvent(ctx, "domain_graph.promoted_from_staging", namespace, "", 0, map[string]interface{}{
		"assertion_id":  assertion.ID,
		"staging_id":    item.ID,
		"domain":        assertion.Domain,
		"entity_type":   assertion.EntityType,
		"entity_id":     assertion.EntityID,
		"relation_type": assertion.RelationType,
		"source_id":     assertion.SourceID,
		"raw_hash":      assertion.RawHash,
	}, "promoter"); err != nil {
		return nil, fmt.Errorf("failed to append domain graph promoted event log: %w", err)
	}
	return assertion, nil
}

func normalizeDomainGraphToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "-", "_")
	value = strings.ReplaceAll(value, " ", "_")
	return value
}
